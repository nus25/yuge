package limiter

import (
	"log/slog"
	"sync"
	"time"

	"github.com/nus25/yuge/feed/errors"
)

type PostLimiter struct {
	mu          sync.Mutex
	records     map[string][]time.Time // Post records per user ID
	postLimit   int                    // Post threshold
	limitWindow time.Duration          // Time window
	cleanupFreq time.Duration          // Interval for cleaning up old data
}

func (pt *PostLimiter) GetRecords() map[string][]time.Time {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	return pt.records
}

func NewPostLimiter(postLimit int, limitWindow, cleanupFreq time.Duration) (*PostLimiter, error) {
	if postLimit <= 0 {
		return nil, errors.NewConfigError("PostLimiter", "postLimit", "postLimit must be greater than 0")
	}
	if limitWindow <= 0 {
		return nil, errors.NewConfigError("PostLimiter", "limitWindow", "limitWindow must be greater than 0")
	}
	if cleanupFreq <= 0 {
		return nil, errors.NewConfigError("PostLimiter", "cleanupFreq", "cleanupFreq must be greater than 0")
	}

	pt := &PostLimiter{
		records:     make(map[string][]time.Time),
		postLimit:   postLimit,
		limitWindow: limitWindow,
		cleanupFreq: cleanupFreq,
	}
	go pt.cleanupOldRecords() // Auto cleanup of old data
	return pt, nil
}

// RecordPost records a post and determines if it exceeds the threshold
func (pt *PostLimiter) RecordPost(did string) (isAllowed bool, count int) {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	now := time.Now()
	posts, exists := pt.records[did]

	if !exists {
		posts = []time.Time{}
	}

	// Remove old posts
	cutoff := now.Add(-pt.limitWindow)
	validPosts := []time.Time{}
	for _, t := range posts {
		if t.After(cutoff) {
			validPosts = append(validPosts, t)
		}
	}

	// Add current post
	validPosts = append(validPosts, now)
	pt.records[did] = validPosts

	// Check if exceeds threshold
	if len(validPosts) > pt.postLimit {
		return false, len(validPosts)
	}
	return true, len(validPosts)
}

// SetPostLimit sets the post limit
func (pt *PostLimiter) SetPostLimit(limit int) error {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	if limit <= 0 {
		return errors.NewConfigError("PostLimiter", "postLimit", "postLimit must be greater than 0")
	}
	pt.postLimit = limit
	return nil
}

// cleanupOldRecords periodically removes old data
func (pt *PostLimiter) cleanupOldRecords() {
	for {
		time.Sleep(pt.cleanupFreq)
		slog.Info("cleaning up old records", "records_count", len(pt.records))
		pt.mu.Lock()
		now := time.Now()
		cutoff := now.Add(-pt.limitWindow)

		for did, posts := range pt.records {
			validPosts := []time.Time{}
			for _, t := range posts {
				if t.After(cutoff) {
					validPosts = append(validPosts, t)
				}
			}
			if len(validPosts) == 0 {
				delete(pt.records, did)
			} else {
				pt.records[did] = validPosts
			}
		}
		pt.mu.Unlock()
	}
}

// Clear clears all records in the limiter
func (pt *PostLimiter) Clear() {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	pt.records = make(map[string][]time.Time)
}
