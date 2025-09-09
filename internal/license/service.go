package license

import (
	"context"
	"database/sql"
	"net/http"
	"sync"
	"time"
)

type Service struct {
	cfg    *Config
	store  *Storage
	http   *http.Client
	pkMu   sync.RWMutex
	pubKey *ParsedPublicKey
}

func NewService(cfg *Config, db *sql.DB) *Service {
	return &Service{cfg: cfg, store: NewStorage(db), http: &http.Client{Timeout: cfg.EffectiveTimeout()}}
}

// LoadOrInit returns existing state or creates a missing placeholder.
func (s *Service) LoadOrInit(ctx context.Context) (*LicenseState, error) {
	st, err := s.store.GetLicenseState(ctx)
	if err != nil {
		return nil, err
	}
	if st != nil {
		return st, nil
	}
	st = &LicenseState{Status: StatusMissing}
	if err := s.store.UpsertLicenseState(ctx, st); err != nil {
		return nil, err
	}
	return st, nil
}

// EnterLicense stores a token after offline validation.
func (s *Service) EnterLicense(ctx context.Context, token string) (*LicenseState, error) {
	pk, err := s.ensurePublicKey(ctx)
	if err != nil {
		return nil, err
	}
	claims, err := ValidateOfflineJWT(token, pk)
	if err != nil {
		return nil, err
	}
	st := &LicenseState{Status: StatusActive}
	if subj, ok := claims["email"].(string); ok {
		st.Subject = &subj
	}
	if app, ok := claims["appName"].(string); ok {
		st.AppName = &app
	}
	if seat, ok := claims["seatCount"].(float64); ok {
		sc := int(seat)
		st.SeatCount = &sc
	}
	if unlim, ok := claims["unlimited"].(bool); ok {
		st.Unlimited = unlim
	}
	if exp, ok := claims["exp"].(float64); ok {
		t := time.Unix(int64(exp), 0)
		st.ExpiresAt = &t
	}
	if token != "" {
		st.Token = &token
	}
	// kid from cached key
	if pk != nil {
		st.Kid = &pk.Kid
	}
	if err := s.store.UpsertLicenseState(ctx, st); err != nil {
		return nil, err
	}
	return st, nil
}

// PerformOnlineValidation refreshes status; force triggers even if recently validated.
func (s *Service) PerformOnlineValidation(ctx context.Context, force bool) (*LicenseState, error) {
	st, err := s.LoadOrInit(ctx)
	if err != nil {
		return nil, err
	}
	if st.Token == nil || *st.Token == "" {
		return st, ErrLicenseMissing
	}

	pk, err := s.ensurePublicKey(ctx)
	if err != nil {
		return nil, err
	}
	// offline quick check first
	if _, err := ValidateOfflineJWT(*st.Token, pk); err != nil {
		switch err {
		case ErrLicenseExpired:
			_ = s.store.UpdateValidationResult(ctx, false, nil, StatusExpired, nil, st.ValidationFailures, &sql.NullTime{Valid: false})
			return s.store.GetLicenseState(ctx)
		case ErrLicenseInvalid:
			_ = s.store.UpdateValidationResult(ctx, false, nil, StatusInvalid, nil, st.ValidationFailures, &sql.NullTime{Valid: false})
			return s.store.GetLicenseState(ctx)
		}
	}
	// online
	data, _, err := ValidateOnline(ctx, s.cfg, s.http, *st.Token, false)
	if err != nil {
		if _, isNet := err.(NetworkError); isNet {
			// network failure path
			fails := st.ValidationFailures + 1
			newStatus := st.Status
			var graceStart sql.NullTime
			switch {
			case st.Status == StatusMissing:
				newStatus = StatusMissing
			case fails >= 2 && st.Status == StatusWarning:
				newStatus = StatusGrace
				graceStart = sql.NullTime{Time: time.Now(), Valid: true}
			case fails >= 1 && (st.Status == StatusActive):
				newStatus = StatusWarning
			}
			_ = s.store.UpdateValidationResult(ctx, false, nil, newStatus, nil, fails, &graceStart)
			return s.store.GetLicenseState(ctx)
		}
		// licence semantic error already handled by offline part typically
		return s.store.GetLicenseState(ctx)
	}
	// success path resets failures
	_ = data // placeholder; can map updated seat counts etc.
	_ = s.store.UpdateValidationResult(ctx, true, nil, StatusActive, nil, 0, &sql.NullTime{Valid: false})
	return s.store.GetLicenseState(ctx)
}

// ensurePublicKey caches the public key.
func (s *Service) ensurePublicKey(ctx context.Context) (*ParsedPublicKey, error) {
	s.pkMu.RLock()
	pk := s.pubKey
	s.pkMu.RUnlock()
	if pk != nil {
		return pk, nil
	}
	s.pkMu.Lock()
	defer s.pkMu.Unlock()
	if s.pubKey != nil {
		return s.pubKey, nil
	}
	fetched, err := FetchPublicKey(ctx, s.cfg, s.http)
	if err != nil {
		return nil, err
	}
	s.pubKey = fetched
	return fetched, nil
}
