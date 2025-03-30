package subscriber

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestFeedStatus_MarshalJSON(t *testing.T) {
	// テストケースの設定
	now := time.Now()
	tests := []struct {
		name     string
		status   FeedStatus
		expected map[string]interface{}
	}{
		{
			name: "正常なステータス（エラーなし）",
			status: FeedStatus{
				FeedID:      "test-feed",
				LastUpdated: now,
				LastStatus:  FeedStatusActive,
				Error:       "",
			},
			expected: map[string]interface{}{
				"feedId":      "test-feed",
				"lastUpdated": now.UTC().Format(time.RFC3339),
				"lastStatus":  "active",
			},
		},
		{
			name: "エラーを含むステータス",
			status: FeedStatus{
				FeedID:      "error-feed",
				LastUpdated: now,
				LastStatus:  FeedStatusError,
				Error:       "something went wrong",
			},
			expected: map[string]interface{}{
				"feedId":      "error-feed",
				"lastUpdated": now.UTC().Format(time.RFC3339),
				"lastStatus":  "error",
				"error":       "something went wrong",
			},
		},
	}

	// テストの実行
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// JSONにマーシャル
			data, err := tt.status.MarshalJSON()
			if err != nil {
				t.Errorf("MarshalJSON() error = %v", err)
				return
			}

			// 結果を検証
			var result map[string]interface{}
			err = json.Unmarshal(data, &result)
			if err != nil {
				t.Errorf("Failed to unmarshal JSON: %v", err)
				return
			}

			if !reflect.DeepEqual(tt.expected, result) {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestFeedStatus_SetError(t *testing.T) {
	// テストケースの設定
	tests := []struct {
		name        string
		initialErr  string
		setErr      error
		expectedErr string
	}{
		{
			name:        "エラーの設定",
			initialErr:  "",
			setErr:      errors.New("new error"),
			expectedErr: "new error",
		},
		{
			name:        "nilエラーの設定",
			initialErr:  "previous error",
			setErr:      nil,
			expectedErr: "",
		},
	}

	// テストの実行
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// テスト用のFeedStatusを作成
			fs := FeedStatus{
				FeedID:      "test-feed",
				LastStatus:  FeedStatusActive,
				LastUpdated: time.Now().Add(-1 * time.Hour), // 1時間前
				Error:       tt.initialErr,
			}

			// エラーを設定
			beforeUpdate := fs.LastUpdated
			fs.SetError(tt.setErr)

			// 検証
			if fs.LastStatus != FeedStatusError {
				t.Errorf("Expected LastStatus to be FeedStatusError, got %v", fs.LastStatus)
			}

			if fs.Error != tt.expectedErr {
				t.Errorf("Expected Error to be %q, got %q", tt.expectedErr, fs.Error)
			}

			if !fs.LastUpdated.After(beforeUpdate) {
				t.Error("LastUpdatedが更新されていません")
			}
		})
	}
}

func TestStatus_String(t *testing.T) {
	// テストケースの設定
	tests := []struct {
		status   Status
		expected string
	}{
		{FeedStatusActive, "active"},
		{FeedStatusInactive, "inactive"},
		{FeedStatusError, "error"},
		{FeedStatusUnknown, "unknown"},
		{Status(999), "unknown"}, // 未定義の値
	}

	// テストの実行
	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := tt.status.String()
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}
