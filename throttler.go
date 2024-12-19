// throttler.go
package main

import (
	"notidock/config"
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
	buckets        []eventBucket
	suspended      bool
	suspendedAt    time.Time
	bucketDuration time.Duration
}

type NotificationThrottler struct {
	mu              sync.RWMutex
	state           map[containerKey]*throttleState
	windowDuration  time.Duration
	bucketDuration  time.Duration // Fixed at 5 seconds
	threshold       int
	cooldownPeriod  time.Duration
	cleanupInterval time.Duration
}

func NewNotificationThrottler(c config.AppConfig) *NotificationThrottler {
	return &NotificationThrottler{
		state:           make(map[containerKey]*throttleState),
		windowDuration:  c.WindowDuration,
		bucketDuration:  5 * time.Second, // Fixed bucket duration
		threshold:       c.EventThreshold,
		cooldownPeriod:  c.NotificationCooldown,
		cleanupInterval: 1 * time.Hour,
	}
}

func (nt *NotificationThrottler) ShouldNotify(containerName, imageTag string) bool {
	// If threshold is 0 or negative, throttling is disabled
	if nt.threshold <= 0 {
		return true
	}

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
