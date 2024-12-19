package main

import (
	"notidock/config"
	"os"
	"testing"
	"time"
)

func TestNotificationThrottler(t *testing.T) {
	t.Run("test throttling disabled with zero threshold", func(t *testing.T) {
		os.Clearenv()

		cfg := config.AppConfig{
			WindowDuration: 60 * time.Second,
			EventThreshold: 0,
		}

		throttler := NewNotificationThrottler(cfg)

		// Should always allow notifications when threshold is 0
		for i := 0; i < 5; i++ {
			if !throttler.ShouldNotify("container1", "image:1.0") {
				t.Error("Expected notification to be allowed when throttling is disabled")
			}
		}
	})

	t.Run("test basic rate limiting", func(t *testing.T) {
		os.Clearenv()

		cfg := config.AppConfig{
			WindowDuration:       10 * time.Second,
			EventThreshold:       3,
			NotificationCooldown: 2 * time.Second,
		}

		throttler := NewNotificationThrottler(cfg)

		// First three notifications should go through
		for i := 0; i < 3; i++ {
			if !throttler.ShouldNotify("container1", "image:1.0") {
				t.Errorf("Notification %d should be allowed", i+1)
			}
		}

		// Fourth notification should be blocked
		if throttler.ShouldNotify("container1", "image:1.0") {
			t.Error("Fourth notification should be blocked")
		}

		// Different container/image combination should be allowed
		if !throttler.ShouldNotify("container2", "image:2.0") {
			t.Error("Different container/image combination should be allowed")
		}
	})

	t.Run("test bucket cleanup", func(t *testing.T) {
		os.Clearenv()
		os.Setenv("NOTIDOCK_WINDOW_DURATION", "5")       // 5 second window
		os.Setenv("NOTIDOCK_EVENT_THRESHOLD", "3")       // Max 3 events
		os.Setenv("NOTIDOCK_NOTIFICATION_COOLDOWN", "2") // 2 second cooldown

		cfg := config.AppConfig{
			WindowDuration:       5 * time.Second,
			EventThreshold:       3,
			NotificationCooldown: 2 * time.Second,
		}

		throttler := NewNotificationThrottler(cfg)

		// Send 2 events
		for i := 0; i < 2; i++ {
			if !throttler.ShouldNotify("container1", "image:1.0") {
				t.Errorf("Notification %d should be allowed", i+1)
			}
		}

		// Wait for window to pass
		time.Sleep(6 * time.Second)

		// Should be allowed to send 3 more events as old ones expired
		for i := 0; i < 3; i++ {
			if !throttler.ShouldNotify("container1", "image:1.0") {
				t.Errorf("Notification %d should be allowed after window reset", i+1)
			}
		}

		// Fourth should be blocked
		if throttler.ShouldNotify("container1", "image:1.0") {
			t.Error("Fourth notification should be blocked")
		}
	})

	t.Run("test cooldown period", func(t *testing.T) {
		os.Clearenv()

		cfg := config.AppConfig{
			WindowDuration:       5 * time.Second,
			EventThreshold:       2,
			NotificationCooldown: 2 * time.Second,
		}

		throttler := NewNotificationThrottler(cfg)

		// Send events until throttled
		for i := 0; i < 3; i++ {
			throttler.ShouldNotify("container1", "image:1.0")
		}

		// Should be blocked during cooldown
		if throttler.ShouldNotify("container1", "image:1.0") {
			t.Error("Should be blocked during cooldown")
		}

		// Wait for cooldown
		time.Sleep(2100 * time.Millisecond)

		// Should be allowed again
		if !throttler.ShouldNotify("container1", "image:1.0") {
			t.Error("Should be allowed after cooldown")
		}
	})

	t.Run("test multiple buckets", func(t *testing.T) {
		os.Clearenv()

		cfg := config.AppConfig{
			WindowDuration:       10 * time.Second,
			EventThreshold:       3,
			NotificationCooldown: 2 * time.Second,
		}

		throttler := NewNotificationThrottler(cfg)

		// Send 2 events
		for i := 0; i < 2; i++ {
			if !throttler.ShouldNotify("container1", "image:1.0") {
				t.Error("Initial notifications should be allowed")
			}
		}

		// Wait for next bucket
		time.Sleep(5100 * time.Millisecond)

		// Send 1 more event (should still be within threshold)
		if !throttler.ShouldNotify("container1", "image:1.0") {
			t.Error("Should be allowed as within total threshold")
		}

		// Send 1 more event (should be blocked as it exceeds threshold)
		if throttler.ShouldNotify("container1", "image:1.0") {
			t.Error("Should be blocked as it exceeds threshold")
		}
	})

	t.Run("test cleanup of old state", func(t *testing.T) {
		os.Clearenv()

		cfg := config.AppConfig{
			WindowDuration:       5 * time.Second,
			EventThreshold:       2,
			NotificationCooldown: 1 * time.Second,
		}

		throttler := NewNotificationThrottler(cfg)

		// Add some entries
		throttler.ShouldNotify("container1", "image:1.0")
		throttler.ShouldNotify("container2", "image:2.0")

		// Wait for more than window duration + 2*cooldown
		time.Sleep(7 * time.Second)

		// Manually trigger cleanup
		throttler.cleanup()

		// Check internal state
		throttler.mu.RLock()
		stateSize := len(throttler.state)
		throttler.mu.RUnlock()

		if stateSize != 0 {
			t.Errorf("Expected cleanup to remove old entries, got %d entries", stateSize)
		}
	})
}
