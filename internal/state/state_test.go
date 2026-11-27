package state

import (
	"path/filepath"
	"testing"
	"time"
)

func TestStoreRoundTrip(t *testing.T) {
	s := Store{Path: filepath.Join(t.TempDir(), "nested", "state.json")}

	empty, err := s.Load()
	if err != nil || empty.Current != nil {
		t.Fatalf("Load on missing file = %+v, %v", empty, err)
	}

	now := time.Date(2026, 11, 27, 8, 0, 0, 0, time.UTC)
	want := State{Current: &Claim{Repo: "acme/app", Number: 42, ItemID: "PVTI_x", ClaimedAt: now}}
	if err := s.Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Current == nil || got.Current.Number != 42 || !got.Current.ClaimedAt.Equal(now) {
		t.Fatalf("Load = %+v", got.Current)
	}
	if !got.Current.Open() {
		t.Error("claim without actuals should be open")
	}
	got.Current.ActualsAt = &now
	if got.Current.Open() {
		t.Error("claim with actuals should not be open")
	}
	var none *Claim
	if none.Open() {
		t.Error("nil claim should not be open")
	}
}
