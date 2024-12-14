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

type eventBucket struct {
	timestamp time.Time
	count     int
}

type throttleState struct {
	buckets        []eventBucket // Stores event counts in time buckets
	suspended      bool
	suspendedAt    time.Time
	bucketDuration time.Duration
}

type NotificationThrottler struct {
	mu              sync.RWMutex
	state           map[containerKey]*throttleState
	windowDuration  time.Duration // Total duration of the sliding window
	bucketDuration  time.Duration // Duration of each bucket (e.g., 5 seconds)
	threshold       int           // Max events allowed in window
	cooldownPeriod  time.Duration
	cleanupInterval time.Duration
}

func NewNotificationThrottler() (*NotificationThrottler, error) {
	windowSeconds, err := getEnvDuration("NOTIDOCK_WINDOW_DURATION")
	if err != nil {
		return nil, fmt.Errorf("invalid window duration: %w", err)
	}
	if windowSeconds == 0 {
		windowSeconds = 60 * time.Second // Default 1 minute window
	}

	bucketSeconds := 5 * time.Second // Fixed 5-second buckets

	threshold, err := getEnvInt("NOTIDOCK_EVENT_THRESHOLD")
	if err != nil {
		return nil, fmt.Errorf("invalid event threshold: %w", err)
	}
	if threshold == 0 {
		threshold = 20 // Default threshold
	}

	cooldown, err := getEnvDuration("NOTIDOCK_NOTIFICATION_COOLDOWN")
	if err != nil {
		return nil, fmt.Errorf("invalid notification cooldown: %w", err)
	}

	nt := &NotificationThrottler{
		state:           make(map[containerKey]*throttleState),
		windowDuration:  windowSeconds,
		bucketDuration:  bucketSeconds,
		threshold:       threshold,
		cooldownPeriod:  cooldown,
		cleanupInterval: 1 * time.Hour,
	}

	go nt.periodicCleanup()

	return nt, nil
}

func getEnvInt(name string) (int, error) {
	val := os.Getenv(name)
	if val == "" {
		return 0, nil
	}
	return strconv.Atoi(val)
}

func getEnvDuration(name string) (time.Duration, error) {
	val := os.Getenv(name)
	if val == "" {
		return 0, nil
	}

	seconds, err := strconv.Atoi(val)
	if err != nil {
		return 0, err
	}

	return time.Duration(seconds) * time.Second, nil
}

func (nt *NotificationThrottler) ShouldNotify(containerName, imageTag string) bool {
	key := containerKey{name: containerName, imageTag: imageTag}
	now := time.Now()

	nt.mu.Lock()
	defer nt.mu.Unlock()

	state, exists := nt.state[key]
	if !exists {
		// Initialize new state with empty buckets
		state = &throttleState{
			buckets:        make([]eventBucket, 0),
			bucketDuration: nt.bucketDuration,
		}
		nt.state[key] = state
	}

	// If we're in suspended state, check cooldown
	if state.suspended {
		if now.Sub(state.suspendedAt) >= nt.cooldownPeriod {
			state.suspended = false
			state.buckets = make([]eventBucket, 0) // Reset buckets after cooldown
		} else {
			return false
		}
	}

	// Clean old buckets
	cutoff := now.Add(-nt.windowDuration)
	newBuckets := make([]eventBucket, 0)
	totalEvents := 0

	for _, bucket := range state.buckets {
		if bucket.timestamp.After(cutoff) {
			newBuckets = append(newBuckets, bucket)
			totalEvents += bucket.count
		}
	}
	state.buckets = newBuckets

	// Find or create current bucket
	currentBucketTime := now.Truncate(nt.bucketDuration)
	var currentBucket *eventBucket

	for i := range state.buckets {
		if state.buckets[i].timestamp.Equal(currentBucketTime) {
			currentBucket = &state.buckets[i]
			break
		}
	}

	if currentBucket == nil {
		state.buckets = append(state.buckets, eventBucket{
			timestamp: currentBucketTime,
			count:     0,
		})
		currentBucket = &state.buckets[len(state.buckets)-1]
	}

	// Increment current bucket
	currentBucket.count++
	totalEvents++

	// Check if we've exceeded the threshold
	if totalEvents > nt.threshold {
		state.suspended = true
		state.suspendedAt = now
		return false
	}

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
	cutoff := now.Add(-nt.windowDuration)

	for key, state := range nt.state {
		// Remove entries where:
		// 1. All buckets are old (outside window duration)
		// 2. Not in suspended state OR suspended state has expired
		allBucketsOld := true
		for _, bucket := range state.buckets {
			if bucket.timestamp.After(cutoff) {
				allBucketsOld = false
				break
			}
		}

		if allBucketsOld && (!state.suspended || now.Sub(state.suspendedAt) > nt.cooldownPeriod) {
			delete(nt.state, key)
		}
	}
}
