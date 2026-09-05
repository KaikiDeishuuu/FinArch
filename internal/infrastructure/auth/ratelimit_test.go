package auth

import (
	"testing"
	"time"
)

func TestLoginAttemptTrackerCanonicalizesEmail(t *testing.T) {
	tracker := NewLoginAttemptTracker(2, time.Hour)

	tracker.RecordFailure(" User@Example.COM ")
	tracker.RecordFailure("user@example.com")
	if !tracker.IsLocked("USER@example.com") {
		t.Fatal("email casing must not create a separate lockout bucket")
	}

	tracker.RecordSuccess(" user@EXAMPLE.com ")
	if tracker.IsLocked("user@example.com") {
		t.Fatal("successful login must clear the canonical lockout bucket")
	}
}

func TestLoginAttemptTrackerKeepsRecentBelowThresholdFailures(t *testing.T) {
	tracker := NewLoginAttemptTracker(3, 15*time.Minute)
	tracker.RecordFailure("user@example.com")

	tracker.mu.Lock()
	failureAt := tracker.entries["user@example.com"].lastFailure
	tracker.mu.Unlock()
	tracker.cleanupExpired(failureAt.Add(30 * time.Minute))

	tracker.mu.Lock()
	_, retained := tracker.entries["user@example.com"]
	tracker.mu.Unlock()
	if !retained {
		t.Fatal("recent below-threshold failure was removed before its retention window")
	}

	tracker.cleanupExpired(failureAt.Add(2 * time.Hour))
	tracker.mu.Lock()
	_, retained = tracker.entries["user@example.com"]
	tracker.mu.Unlock()
	if retained {
		t.Fatal("stale failure counter was not removed")
	}
}
