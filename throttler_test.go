package main

import (
	"os"
	"testing"
	"time"
)

func TestNotificationThrottler(t *testing.T) {
	t.Run("test throttling disabled", func(t *testing.T) {
		os.Clearenv() // Clear environment variables
		throttler, err := NewNotificationThrottler()
		if err != nil {
			t.Fatalf("Failed to create throttler: %v", err)
		}

		// Should always allow notifications when throttling is disabled
		for i := 0; i < 5; i++ {
			if !throttler.ShouldNotify("container1", "image:1.0") {
				t.Error("Expected notification to be allowed when throttling is disabled")
			}
		}
	})

	t.Run("test basic throttling", func(t *testing.T) {
		os.Clearenv()
		os.Setenv("NOTIDOCK_NOTIFICATION_TIMEOUT", "1")  // 1 second timeout
		os.Setenv("NOTIDOCK_NOTIFICATION_COOLDOWN", "2") // 2 seconds cooldown

		throttler, err := NewNotificationThrottler()
		if err != nil {
			t.Fatalf("Failed to create throttler: %v", err)
		}

		// First notification should go through
		if !throttler.ShouldNotify("container1", "image:1.0") {
			t.Error("First notification should be allowed")
		}

		// Second immediate notification should be blocked
		if throttler.ShouldNotify("container1", "image:1.0") {
			t.Error("Second immediate notification should be blocked")
		}

		// Different container/image combination should be allowed
		if !throttler.ShouldNotify("container2", "image:2.0") {
			t.Error("Different container/image combination should be allowed")
		}
	})

	t.Run("test cooldown period", func(t *testing.T) {
		os.Clearenv()
		os.Setenv("NOTIDOCK_NOTIFICATION_TIMEOUT", "1")  // 1 second timeout
		os.Setenv("NOTIDOCK_NOTIFICATION_COOLDOWN", "2") // 2 seconds cooldown

		throttler, err := NewNotificationThrottler()
		if err != nil {
			t.Fatalf("Failed to create throttler: %v", err)
		}

		// First notification should go through
		if !throttler.ShouldNotify("container1", "image:1.0") {
			t.Error("First notification should be allowed")
		}

		// Wait for timeout to trigger suspension
		time.Sleep(1100 * time.Millisecond)

		// Should be blocked (in suspension period)
		if throttler.ShouldNotify("container1", "image:1.0") {
			t.Error("Notification should be blocked during suspension")
		}

		// Wait for cooldown period
		time.Sleep(2100 * time.Millisecond)

		// Should be allowed again after cooldown
		if !throttler.ShouldNotify("container1", "image:1.0") {
			t.Error("Notification should be allowed after cooldown period")
		}
	})

	t.Run("test rapid notifications", func(t *testing.T) {
		os.Clearenv()
		os.Setenv("NOTIDOCK_NOTIFICATION_TIMEOUT", "1")
		os.Setenv("NOTIDOCK_NOTIFICATION_COOLDOWN", "2")

		throttler, err := NewNotificationThrottler()
		if err != nil {
			t.Fatalf("Failed to create throttler: %v", err)
		}

		// First notification
		if !throttler.ShouldNotify("container1", "image:1.0") {
			t.Error("First notification should be allowed")
		}

		// Rapid subsequent notifications should all be blocked
		for i := 0; i < 3; i++ {
			if throttler.ShouldNotify("container1", "image:1.0") {
				t.Error("Notification should be blocked during rapid notifications")
			}
		}
	})

	t.Run("test multiple containers", func(t *testing.T) {
		os.Clearenv()
		os.Setenv("NOTIDOCK_NOTIFICATION_TIMEOUT", "1")
		os.Setenv("NOTIDOCK_NOTIFICATION_COOLDOWN", "1")

		throttler, err := NewNotificationThrottler()
		if err != nil {
			t.Fatalf("Failed to create throttler: %v", err)
		}

		containers := []struct {
			name     string
			imageTag string
		}{
			{"container1", "image:1.0"},
			{"container1", "image:2.0"}, // Same container, different image
			{"container2", "image:1.0"}, // Different container, same image
		}

		// All first notifications should go through
		for _, c := range containers {
			if !throttler.ShouldNotify(c.name, c.imageTag) {
				t.Errorf("First notification should be allowed for %s:%s", c.name, c.imageTag)
			}
		}

		// Immediate second notifications should be blocked
		for _, c := range containers {
			if throttler.ShouldNotify(c.name, c.imageTag) {
				t.Errorf("Second immediate notification should be blocked for %s:%s", c.name, c.imageTag)
			}
		}
	})

	t.Run("test cleanup", func(t *testing.T) {
		os.Clearenv()
		os.Setenv("NOTIDOCK_NOTIFICATION_TIMEOUT", "1")
		os.Setenv("NOTIDOCK_NOTIFICATION_COOLDOWN", "1")

		throttler, err := NewNotificationThrottler()
		if err != nil {
			t.Fatalf("Failed to create throttler: %v", err)
		}

		// Add some entries
		throttler.ShouldNotify("container1", "image:1.0")
		throttler.ShouldNotify("container2", "image:2.0")

		// Manually trigger cleanup
		throttler.cleanup()

		// Wait for more than twice the cooldown period
		time.Sleep(2500 * time.Millisecond)

		// Manually trigger cleanup
		throttler.cleanup()

		// Check internal state (accessing private field for testing)
		throttler.mu.RLock()
		stateSize := len(throttler.state)
		throttler.mu.RUnlock()

		if stateSize != 0 {
			t.Errorf("Expected cleanup to remove old entries, got %d entries", stateSize)
		}
	})

	t.Run("test invalid environment variables", func(t *testing.T) {
		os.Clearenv()
		os.Setenv("NOTIDOCK_NOTIFICATION_TIMEOUT", "invalid")

		_, err := NewNotificationThrottler()
		if err == nil {
			t.Error("Expected error with invalid timeout value")
		}

		os.Clearenv()
		os.Setenv("NOTIDOCK_NOTIFICATION_COOLDOWN", "invalid")

		_, err = NewNotificationThrottler()
		if err == nil {
			t.Error("Expected error with invalid cooldown value")
		}
	})
}

// Helper function to test duration parsing
func TestGetEnvDuration(t *testing.T) {
	tests := []struct {
		name        string
		envValue    string
		want        time.Duration
		expectError bool
	}{
		{
			name:        "valid duration",
			envValue:    "60",
			want:        60 * time.Second,
			expectError: false,
		},
		{
			name:        "empty value",
			envValue:    "",
			want:        0,
			expectError: false,
		},
		{
			name:        "invalid value",
			envValue:    "invalid",
			want:        0,
			expectError: true,
		},
		{
			name:        "negative value",
			envValue:    "-60",
			want:        -60 * time.Second,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Clearenv()
			os.Setenv("TEST_DURATION", tt.envValue)

			got, err := getEnvDuration("TEST_DURATION")
			if (err != nil) != tt.expectError {
				t.Errorf("getEnvDuration() error = %v, expectError %v", err, tt.expectError)
				return
			}
			if got != tt.want {
				t.Errorf("getEnvDuration() = %v, want %v", got, tt.want)
			}
		})
	}
}
