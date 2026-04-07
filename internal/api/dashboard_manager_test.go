package api

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeDashboardLeaderLockStore struct {
	leader       bool
	err          error
	calls        int
	releaseErr   error
	releaseCalls int
}

func (f *fakeDashboardLeaderLockStore) TryAcquireDashboardRefreshLeaderLock(ctx context.Context) (bool, error) {
	f.calls++
	return f.leader, f.err
}

func (f *fakeDashboardLeaderLockStore) ReleaseDashboardRefreshLeaderLock(ctx context.Context) error {
	f.releaseCalls++
	return f.releaseErr
}

func TestParseDashboardLogLevel(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  dashboardLogLevel
	}{
		{name: "default empty", input: "", want: dashboardLogLevelMinimal},
		{name: "off", input: "off", want: dashboardLogLevelOff},
		{name: "errors only", input: "errors_only", want: dashboardLogLevelErrorsOnly},
		{name: "minimal", input: "minimal", want: dashboardLogLevelMinimal},
		{name: "verbose", input: "verbose", want: dashboardLogLevelVerbose},
		{name: "unknown defaults minimal", input: "unknown", want: dashboardLogLevelMinimal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseDashboardLogLevel(tt.input)
			if got != tt.want {
				t.Fatalf("parseDashboardLogLevel(%q)=%v want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestCanRunPeriodicAllOrgRefresh(t *testing.T) {
	tests := []struct {
		name    string
		leader  bool
		err     error
		wantRun bool
	}{
		{name: "leader instance", leader: true, wantRun: true},
		{name: "non leader instance", leader: false, wantRun: false},
		{name: "lock error", leader: false, err: context.DeadlineExceeded, wantRun: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &fakeDashboardLeaderLockStore{leader: tt.leader, err: tt.err}
			dm := &DashboardManager{
				lockStore: store,
				instance:  "test",
				logLevel:  dashboardLogLevelOff,
			}

			got := dm.canRunPeriodicAllOrgRefresh(context.Background(), dashboardTriggerTicker)
			if got != tt.wantRun {
				t.Fatalf("canRunPeriodicAllOrgRefresh()=%v want %v", got, tt.wantRun)
			}
			if store.calls != 1 {
				t.Fatalf("expected lock store to be called once, got %d", store.calls)
			}
		})
	}
}

func TestBeginRefreshCycle(t *testing.T) {
	dm := &DashboardManager{
		instance: "test",
		logLevel: dashboardLogLevelOff,
	}

	firstID, ok := dm.beginRefreshCycle(dashboardTriggerTicker)
	if !ok {
		t.Fatalf("expected first beginRefreshCycle to succeed")
	}
	if firstID != 1 {
		t.Fatalf("expected first cycle id=1, got %d", firstID)
	}

	if _, ok := dm.beginRefreshCycle(dashboardTriggerTicker); ok {
		t.Fatalf("expected second beginRefreshCycle to fail while in progress")
	}

	dm.endRefreshCycle()

	thirdID, ok := dm.beginRefreshCycle(dashboardTriggerTicker)
	if !ok {
		t.Fatalf("expected beginRefreshCycle to succeed after endRefreshCycle")
	}
	if thirdID != 2 {
		t.Fatalf("expected next cycle id=2, got %d", thirdID)
	}
}

func TestRunRefreshCycleReleasesLeaderLock(t *testing.T) {
	store := &fakeDashboardLeaderLockStore{}
	dm := &DashboardManager{
		lockStore: store,
		instance:  "test",
		logLevel:  dashboardLogLevelOff,
	}

	// Force beginRefreshCycle to short-circuit so this test does not need DB access.
	dm.refreshInProgress.Store(true)

	err := dm.runRefreshCycle(context.Background(), dashboardTriggerTicker)
	if err != nil {
		t.Fatalf("runRefreshCycle returned unexpected error: %v", err)
	}
	if store.releaseCalls != 1 {
		t.Fatalf("expected one lock release call, got %d", store.releaseCalls)
	}
}

func TestRunRefreshCycleReturnsReleaseError(t *testing.T) {
	store := &fakeDashboardLeaderLockStore{releaseErr: errors.New("release failed")}
	dm := &DashboardManager{
		lockStore: store,
		instance:  "test",
		logLevel:  dashboardLogLevelOff,
	}

	// Force beginRefreshCycle to short-circuit so this test does not need DB access.
	dm.refreshInProgress.Store(true)

	err := dm.runRefreshCycle(context.Background(), dashboardTriggerTicker)
	if err == nil {
		t.Fatalf("expected release error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to release dashboard leader lock") {
		t.Fatalf("expected lock release error context, got: %v", err)
	}
	if store.releaseCalls != 1 {
		t.Fatalf("expected one lock release call, got %d", store.releaseCalls)
	}
}
