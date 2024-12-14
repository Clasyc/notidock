// throttler.go
package main

import (
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"
)

type containerKey struct {
	name     string
	imageTag string
}

type throttleState struct {
	lastNotification time.Time
	suspended        bool
	suspendedAt      time.Time
}

type NotificationThrottler struct {
	mu              sync.RWMutex
	state           map[containerKey]*throttleState
	timeout         time.Duration
	cooldownPeriod  time.Duration
	cleanupInterval time.Duration
}

func NewNotificationThrottler() (*NotificationThrottler, error) {
	timeout, err := getEnvDuration("NOTIDOCK_NOTIFICATION_TIMEOUT")
	if err != nil {
		return nil, fmt.Errorf("invalid notification timeout: %w", err)
	}

	cooldown, err := getEnvDuration("NOTIDOCK_NOTIFICATION_COOLDOWN")
	if err != nil {
		return nil, fmt.Errorf("invalid notification cooldown: %w", err)
	}

	nt := &NotificationThrottler{
		state:           make(map[containerKey]*throttleState),
		timeout:         timeout,
		cooldownPeriod:  cooldown,
		cleanupInterval: 1 * time.Hour, // Cleanup old entries every hour
	}

	// Start cleanup goroutine
	go nt.periodicCleanup()

	return nt, nil
}

func getEnvDuration(name string) (time.Duration, error) {
	val := os.Getenv(name)
	if val == "" {
		return 0, nil // Default to no throttling
	}

	seconds, err := strconv.Atoi(val)
	if err != nil {
		return 0, err
	}

	return time.Duration(seconds) * time.Second, nil
}

func (nt *NotificationThrottler) ShouldNotify(containerName, imageTag string) bool {
	if nt.timeout == 0 {
		return true // Throttling disabled
	}

	key := containerKey{name: containerName, imageTag: imageTag}
	now := time.Now()

	nt.mu.Lock()
	defer nt.mu.Unlock()

	state, exists := nt.state[key]
	if !exists {
		// First notification for this container/image
		nt.state[key] = &throttleState{
			lastNotification: now,
			suspended:        false,
		}
		return true
	}

	// If we're in suspended state
	if state.suspended {
		// Check if cooldown period has passed
		if now.Sub(state.suspendedAt) >= nt.cooldownPeriod {
			// Reset suspension and allow notification
			state.suspended = false
			state.lastNotification = now
			return true
		}
		return false // Still in cooldown period
	}

	timeSinceLastNotification := now.Sub(state.lastNotification)

	// Enter suspension if:
	// 1. We get notifications too quickly (within timeout)
	// 2. We hit the timeout period
	if timeSinceLastNotification <= nt.timeout {
		// Too frequent, enter suspension
		state.suspended = true
		state.suspendedAt = now
		return false
	}

	// Time since last notification exceeds timeout, enter suspension
	if timeSinceLastNotification >= nt.timeout {
		state.suspended = true
		state.suspendedAt = now
		return false
	}

	// This should never be reached, but just in case
	state.lastNotification = now
	return true
}

func (nt *NotificationThrottler) periodicCleanup() {
	ticker := time.NewTicker(nt.cleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		nt.cleanup()
	}
}

func (nt *NotificationThrottler) cleanup() {
	nt.mu.Lock()
	defer nt.mu.Unlock()

	now := time.Now()
	for key, state := range nt.state {
		// Remove entries that haven't been updated in twice the cooldown period
		if now.Sub(state.lastNotification) > (nt.cooldownPeriod * 2) {
			delete(nt.state, key)
		}
	}
}
