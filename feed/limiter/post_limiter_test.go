package limiter

import (
	"testing"
	"time"
)

func TestPostLimitter(t *testing.T) {
	tests := []struct {
		name     string
		limit    int
		window   time.Duration
		interval time.Duration
		did      string
		posts    int
		want     bool
	}{
		{
			name:     "単一投稿は制限に引っかからない",
			limit:    10,
			window:   10 * time.Minute,
			interval: time.Minute,
			did:      "did:plc:user1",
			posts:    1,
			want:     true,
		},
		{
			name:     "制限以下の投稿数は許可される",
			limit:    10,
			window:   10 * time.Minute,
			interval: time.Minute,
			did:      "did:plc:user1",
			posts:    3,
			want:     true,
		},
		{
			name:     "制限を超える投稿は拒否される",
			limit:    2,
			window:   10 * time.Minute,
			interval: time.Minute,
			did:      "did:plc:user1",
			posts:    3,
			want:     false,
		},
		{
			name:     "異なるユーザーの投稿は別々にカウントされる",
			limit:    2,
			window:   10 * time.Minute,
			interval: time.Minute,
			did:      "did:plc:user2",
			posts:    2,
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l, err := NewPostLimiter(tt.limit, tt.window, tt.interval)
			if err != nil {
				t.Errorf("NewPostLimiter() returned error: %v", err)
			}
			var got bool
			var count int
			for i := 0; i < tt.posts; i++ {
				got, count = l.RecordPost(tt.did)
			}
			if got != tt.want {
				t.Errorf("PostLimitter.RecordPost() = %v (count: %d), want %v", got, count, tt.want)
			}
		})
	}
}

func TestPostLimiter_NewPostLimiter(t *testing.T) {
	tests := []struct {
		name        string
		postLimit   int
		limitWindow time.Duration
		cleanupFreq time.Duration
		wantLimit   int
		wantWindow  time.Duration
		wantFreq    time.Duration
		wantErr     bool
	}{
		{
			name:        "valid values are respected",
			postLimit:   20,
			limitWindow: 20 * time.Minute,
			cleanupFreq: 2 * time.Minute,
			wantLimit:   20,
			wantWindow:  20 * time.Minute,
			wantFreq:    2 * time.Minute,
			wantErr:     false,
		},
		{
			name:        "invalid values are rejected",
			postLimit:   0,
			limitWindow: 0,
			cleanupFreq: 0,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l, err := NewPostLimiter(tt.postLimit, tt.limitWindow, tt.cleanupFreq)
			if tt.wantErr {
				if err == nil {
					t.Errorf("NewPostLimiter() returned nil, want error")
				}
				return
			}
			if err != nil {
				t.Errorf("NewPostLimiter() returned error: %v", err)
			}
			if l == nil {
				t.Error("NewPostLimiter() returned nil")
			}
			if l.postLimit != tt.wantLimit {
				t.Errorf("postLimit = %v, want %v", l.postLimit, tt.wantLimit)
			}
			if l.limitWindow != tt.wantWindow {
				t.Errorf("limitWindow = %v, want %v", l.limitWindow, tt.wantWindow)
			}
			if l.cleanupFreq != tt.wantFreq {
				t.Errorf("cleanupFreq = %v, want %v", l.cleanupFreq, tt.wantFreq)
			}
		})
	}
}

func TestPostLimitter_RecordPost_WithInterval(t *testing.T) {
	tests := []struct {
		name     string
		limit    int
		window   time.Duration
		interval time.Duration
		did      string
		want     bool
	}{
		{
			name:     "制限に達した後、intervalを待つと再度投稿可能",
			limit:    2,
			window:   time.Second,
			interval: time.Second, // テストのため短い間隔を設定
			did:      "did:plc:user1",
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l, err := NewPostLimiter(tt.limit, tt.window, tt.interval)
			if err != nil {
				t.Errorf("NewPostLimiter() returned error: %v", err)
			}

			// 制限まで投稿
			for i := 0; i < tt.limit; i++ {
				isAllowed, _ := l.RecordPost(tt.did)
				if !isAllowed {
					t.Errorf("最初の%d回の投稿は制限に達していないはずです", i+1)
				}
			}

			// 制限を超える投稿は拒否される
			isAllowed, _ := l.RecordPost(tt.did)
			if isAllowed {
				t.Error("制限を超える投稿が許可されました")
			}

			// interval待機
			time.Sleep(2 * tt.interval)

			// interval後は再度投稿可能
			isAllowed, _ = l.RecordPost(tt.did)
			if isAllowed != tt.want {
				t.Errorf("interval後のPostLimitter.RecordPost() = %v, want %v", isAllowed, tt.want)
			}
		})
	}
}

func TestPostLimiter_GetRecords(t *testing.T) {
	l, err := NewPostLimiter(10, 10*time.Minute, time.Minute)
	if err != nil {
		t.Errorf("NewPostLimiter() returned error: %v", err)
	}
	l.RecordPost("did:plc:user1")
	l.RecordPost("did:plc:user1")
	records := l.GetRecords()
	if len(records["did:plc:user1"]) != 2 {
		t.Errorf("GetRecords() = %v, want 2 records", len(records["did:plc:user1"]))
	}
}

func TestPostLimiter_SetPostLimit(t *testing.T) {
	l, err := NewPostLimiter(10, 10*time.Minute, time.Minute)
	if err != nil {
		t.Errorf("NewPostLimiter() returned error: %v", err)
	}
	l.SetPostLimit(5)
	if l.postLimit != 5 {
		t.Errorf("SetPostLimit() = %v, want 5", l.postLimit)
	}
	// if given invalid limit value, use default value
	if err := l.SetPostLimit(-1); err == nil {
		t.Errorf("SetPostLimit() returned nil, want error")
	}
}

func TestPostLimiter_Clear(t *testing.T) {
	l, err := NewPostLimiter(10, 10*time.Minute, time.Minute)
	if err != nil {
		t.Errorf("NewPostLimiter() returned error: %v", err)
	}
	l.RecordPost("did:plc:user1")
	l.Clear()
	records := l.GetRecords()
	if len(records) != 0 {
		t.Errorf("Clear() did not clear records, got %v", records)
	}
}
