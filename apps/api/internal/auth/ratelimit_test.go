package auth

import (
	"testing"
	"time"
)

func TestLoginLimiterBlocksAfterThreshold(t *testing.T) {
	now := time.Unix(1700000000, 0)
	limiter := NewLoginLimiter(3, 15*time.Minute)
	limiter.now = func() time.Time { return now }

	for i := range 3 {
		if !limiter.Allow("ip|alice") {
			t.Fatalf("attempt %d should be allowed", i)
		}
		limiter.RecordFailure("ip|alice")
	}
	if limiter.Allow("ip|alice") {
		t.Fatal("fourth attempt must be blocked")
	}
	if !limiter.Allow("ip|bob") {
		t.Fatal("a different username must not be blocked")
	}
	if !limiter.Allow("other|alice") {
		t.Fatal("a different client IP must not be blocked")
	}
}

func TestLoginLimiterWindowSlides(t *testing.T) {
	now := time.Unix(1700000000, 0)
	limiter := NewLoginLimiter(2, 10*time.Minute)
	limiter.now = func() time.Time { return now }

	limiter.RecordFailure("k")
	limiter.RecordFailure("k")
	if limiter.Allow("k") {
		t.Fatal("should be blocked at threshold")
	}
	now = now.Add(11 * time.Minute)
	if !limiter.Allow("k") {
		t.Fatal("failures older than the window must expire")
	}
}

func TestLoginLimiterSuccessResets(t *testing.T) {
	limiter := NewLoginLimiter(2, time.Minute)
	limiter.RecordFailure("k")
	limiter.RecordFailure("k")
	if limiter.Allow("k") {
		t.Fatal("should be blocked at threshold")
	}
	limiter.Reset("k")
	if !limiter.Allow("k") {
		t.Fatal("successful login must clear the failure counter")
	}
}
