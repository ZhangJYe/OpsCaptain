package tools

import (
	"testing"
	"time"
)

func TestCalculateDuration_ValidTime(t *testing.T) {
	activeAt := time.Now().Add(-2*time.Hour - 30*time.Minute - 15*time.Second).Format(time.RFC3339Nano)
	result := calculateDuration(activeAt)

	if result == "unknown" {
		t.Fatal("expected valid duration, got 'unknown'")
	}
	if len(result) == 0 {
		t.Fatal("expected non-empty duration string")
	}
	if result[0] < '0' || result[0] > '9' {
		t.Fatalf("expected duration to start with digit, got '%s'", result)
	}
}

func TestCalculateDuration_InvalidTime(t *testing.T) {
	result := calculateDuration("not-a-time")
	if result != "unknown" {
		t.Fatalf("expected 'unknown' for invalid time, got '%s'", result)
	}
}

func TestCalculateDuration_RecentTime(t *testing.T) {
	activeAt := time.Now().Add(-5 * time.Second).Format(time.RFC3339Nano)
	result := calculateDuration(activeAt)

	if result == "unknown" {
		t.Fatal("expected valid duration")
	}
	if result[len(result)-1] != 's' {
		t.Fatalf("expected duration to end with 's', got '%s'", result)
	}
}

func TestCalculateDuration_MinutesRange(t *testing.T) {
	activeAt := time.Now().Add(-15 * time.Minute).Format(time.RFC3339Nano)
	result := calculateDuration(activeAt)

	if result == "unknown" {
		t.Fatal("expected valid duration")
	}
	if result[len(result)-1] != 's' {
		t.Fatalf("expected duration to end with 's', got '%s'", result)
	}
}
