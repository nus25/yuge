package watchlist

import (
	"testing"
	"time"
)

func TestWatchlist(t *testing.T) {
	expireDuration := 1 * time.Second
	watchlist, err := NewWatchlist(expireDuration)
	if err != nil {
		t.Fatalf("failed to create watchlist: %v", err)
	}

	t.Run("Add", func(t *testing.T) {
		did := "did:plc:test"
		rkey := "test1"
		watchlist.Add(did, rkey)

		// 追加したアイテムが存在することを確認
		item := watchlist.Contains(did)
		if item == nil {
			t.Error("expected non-nil item but got nil")
		}
		if item.RKey != rkey {
			t.Errorf("expected rkey %s, got %s", rkey, item.RKey)
		}
	})

	t.Run("Delete", func(t *testing.T) {
		did := "did:plc:test2"
		rkey := "test2"
		watchlist.Add(did, rkey)
		deleted := watchlist.Delete(did)

		// 削除したアイテムが存在しないことを確認
		item := watchlist.Contains(did)
		if item != nil {
			t.Error("expected nil item but got non-nil")
		}
		if !deleted {
			t.Error("expected true but got false")
		}
	})

	t.Run("Delete_NonExistent", func(t *testing.T) {
		did := "did:plc:nonexistent"
		deleted := watchlist.Delete(did)

		// 存在しないDIDを削除しようとした場合、falseが返されることを確認
		if deleted {
			t.Error("expected false but got true")
		}
	})

	t.Run("Contains_Expired", func(t *testing.T) {
		did := "did:plc:test3"
		rkey := "test3"
		watchlist.Add(did, rkey)

		// 有効期限が切れるまで待機
		time.Sleep(expireDuration + 100*time.Millisecond)

		// 期限切れのアイテムが存在しないことを確認
		item := watchlist.Contains(did)
		if item != nil {
			t.Error("expected nil item but got non-nil")
		}
	})

	t.Run("Refresh", func(t *testing.T) {
		// 期限切れのアイテムと有効なアイテムを追加
		expiredDid := "did:plc:expired"
		validDid := "did:plc:valid"
		watchlist.Add(expiredDid, "expired")

		// 期限切れまで待機
		time.Sleep(expireDuration + 100*time.Millisecond)

		// 有効なアイテムを追加
		watchlist.Add(validDid, "valid")

		// リフレッシュを実行
		err := watchlist.Reflesh()
		if err != nil {
			t.Errorf("failed to refresh watchlist: %v", err)
		}

		// 期限切れのアイテムが削除されていることを確認
		expiredItem := watchlist.Contains(expiredDid)
		if expiredItem != nil {
			t.Error("expected nil item but got non-nil")
		}

		// 有効なアイテムは残っていることを確認
		validItem := watchlist.Contains(validDid)
		if validItem == nil {
			t.Error("expected non-nil item but got nil")
		}
		if validItem.RKey != "valid" {
			t.Errorf("expected rkey 'valid', got %s", validItem.RKey)
		}
	})

	t.Run("Clear", func(t *testing.T) {
		// アイテムを追加
		did1 := "did:plc:test5"
		did2 := "did:plc:test6"
		watchlist.Add(did1, "test5")
		watchlist.Add(did2, "test6")

		// クリア前にアイテムが存在することを確認
		if watchlist.Contains(did1) == nil {
			t.Error("expected non-nil item but got nil for did1")
		}
		if watchlist.Contains(did2) == nil {
			t.Error("expected non-nil item but got nil for did2")
		}

		// クリアを実行
		watchlist.Clear()

		// すべてのアイテムが削除されていることを確認
		if watchlist.Contains(did1) != nil {
			t.Error("expected nil item but got non-nil for did1")
		}
		if watchlist.Contains(did2) != nil {
			t.Error("expected nil item but got non-nil for did2")
		}
		if len(watchlist.items) != 0 {
			t.Errorf("expected 0 items, got %d", len(watchlist.items))
		}
	})

	t.Run("UpdateExpireDuration", func(t *testing.T) {
		tests := []struct {
			name          string
			initialDur    time.Duration
			newDur        time.Duration
			expectedDelta time.Duration
		}{
			{
				name:          "有効期限を延長",
				initialDur:    1 * time.Minute,
				newDur:        5 * time.Hour,
				expectedDelta: 5*time.Hour - 1*time.Minute,
			},
			{
				name:          "有効期限を短縮",
				initialDur:    5 * time.Hour,
				newDur:        1 * time.Second,
				expectedDelta: 1*time.Second - 5*time.Hour,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				// 初期の有効期限を設定
				w, err := NewWatchlist(tt.initialDur)
				if err != nil {
					t.Fatalf("failed to create watchlist: %v", err)
				}
				defer w.Stop()

				// アイテムを追加
				did := "did:plc:test7"
				w.Add(did, "test7")

				// アイテムの有効期限を直接確認
				item := w.Contains(did)
				if item == nil {
					t.Fatal("expected non-nil item but got nil")
				}
				initialExpiry := item.ExpireAt

				// 有効期限を変更
				w.UpdatExpireDuration(tt.newDur)

				// 更新後のアイテムを取得
				updatedItem := w.Contains(did)
				if updatedItem == nil {
					t.Fatal("expected non-nil item but got nil")
				}

				// 実際の変化時間を計算
				actualDelta := updatedItem.ExpireAt.Sub(initialExpiry)

				// 期待値と実際の値を比較（1秒の誤差を許容）
				if diff := actualDelta.Seconds() - tt.expectedDelta.Seconds(); diff < -1 || diff > 1 {
					t.Errorf("expected delta %v, got %v", tt.expectedDelta, actualDelta)
				}
			})
		}
	})
}
