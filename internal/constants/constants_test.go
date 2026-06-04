package constants

import "testing"

func TestRotationConstants(t *testing.T) {
	if FightDurationSecs != 600 {
		t.Fatalf("FightDurationSecs = %v, want 600", FightDurationSecs)
	}
	if CACastTimeSecs != 0.5 {
		t.Fatalf("CACastTimeSecs = %v, want 0.5", CACastTimeSecs)
	}
}
