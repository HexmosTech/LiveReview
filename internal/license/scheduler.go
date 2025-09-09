package license

import (
	"context"
	"fmt"
	"time"
)

// Scheduler triggers periodic online validation and enforces grace expiry.
type Scheduler struct {
	svc      *Service
	interval time.Duration
	stopCh   chan struct{}
	doneCh   chan struct{}
	started  bool
}

func NewScheduler(svc *Service) *Scheduler {
	iv := svc.cfg.ValidationInterval
	if iv < time.Minute { // defensive floor
		iv = time.Minute
	}
	return &Scheduler{svc: svc, interval: iv, stopCh: make(chan struct{}), doneCh: make(chan struct{})}
}

func (s *Scheduler) Start() {
	if s.started {
		return
	}
	s.started = true
	go s.loop()
}

func (s *Scheduler) Stop() {
	if !s.started {
		return
	}
	close(s.stopCh)
	<-s.doneCh
}

func (s *Scheduler) loop() {
	initial := time.NewTimer(5 * time.Second)
	ticker := time.NewTicker(s.interval)
	defer func() { ticker.Stop(); close(s.doneCh) }()
	for {
		select {
		case <-s.stopCh:
			return
		case <-initial.C:
			s.runOnce()
		case <-ticker.C:
			s.runOnce()
		}
	}
}

func (s *Scheduler) runOnce() {
	ctx, cancel := context.WithTimeout(context.Background(), s.svc.cfg.EffectiveTimeout())
	defer cancel()
	if _, err := s.svc.PerformOnlineValidation(ctx, false); err != nil && err != ErrLicenseMissing {
		fmt.Printf("[license] validation cycle error: %v\n", err)
	}
	_ = s.svc.expireIfGraceExceeded(ctx)
}
