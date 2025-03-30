package watchlist

import (
	"log/slog"
	"time"
)

// Watchlist は監視対象のDIDとその有効期限を管理する
type Watchlist struct {
	logger         *slog.Logger
	items          map[string]WatchItem
	expireDuration time.Duration
	stopChan       chan struct{}
}

type WatchItem struct {
	ExpireAt time.Time `json:"expireAt"`
	RKey     string    `json:"rkey"`
}

func NewWatchlist(expireDuration time.Duration) (*Watchlist, error) {
	logger := slog.Default().With("component", "watchlist")
	items := make(map[string]WatchItem)

	w := &Watchlist{
		logger:         logger,
		items:          items,
		expireDuration: expireDuration,
		stopChan:       make(chan struct{}),
	}
	// 定期実行用goroutineを開始
	go w.startPeriodicRefresh()
	return w, nil
}

// Add は監視対象のDIDを追加・更新する
func (w *Watchlist) Add(did string, rkey string) {
	expireAt := time.Now().Add(w.expireDuration)
	w.items[did] = WatchItem{
		ExpireAt: expireAt,
		RKey:     rkey,
	}
	w.logger.Info("added did to watchlist", "did", did, "rkey", rkey, "expireAt", expireAt)
}

// Delete は指定されたDIDを監視対象から削除する
func (w *Watchlist) Delete(did string) bool {
	if _, exists := w.items[did]; !exists {
		w.logger.Info("attempted to remove non-existent did from watchlist", "did", did)
		return false
	}

	delete(w.items, did)
	w.logger.Info("removed did from watchlist", "did", did)
	return true
}

func (w *Watchlist) Clear() {
	w.items = make(map[string]WatchItem)
	w.logger.Info("cleared watchlist")
}

// Contains は指定されたDIDが監視対象に含まれているかを確認する
// 有効期限内のitemが存在する場合はそのアイテムを返し、ない場合はnilを返す
func (w *Watchlist) Contains(did string) *WatchItem {
	item, ok := w.items[did]
	if !ok {
		return nil
	}

	if time.Now().Before(item.ExpireAt) {
		return &item
	}

	delete(w.items, did)
	w.logger.Info("removed expired did from watchlist", "did", did)
	return nil
}

func (w *Watchlist) Save() error {
	return nil
}

func (w *Watchlist) UpdatExpireDuration(d time.Duration) error {
	w.logger.Info("updating expire duration")
	//期限切れのwatchitemは事前に削除
	w.Reflesh()
	// 既存のアイテムの有効期限を更新
	diff := d - w.expireDuration
	for did, item := range w.items {
		item.ExpireAt = item.ExpireAt.Add(diff)
		w.items[did] = item
	}

	w.expireDuration = d
	w.logger.Info("updated expire duration", "duration", d)
	return nil

}

func (w *Watchlist) Reflesh() error {
	w.logger.Info("refreshing watchlist")
	now := time.Now()
	for did, item := range w.items {
		if now.After(item.ExpireAt) {
			delete(w.items, did)
			w.logger.Info("removed expired did from watchlist", "did", did)
		}
	}
	return nil
}

// 1時間毎にリフレッシュする。goルーチンで呼び出すこと
func (w *Watchlist) startPeriodicRefresh() {
	//start ticker
	w.logger.Info("starting reflesh ticker")
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := w.Reflesh(); err != nil {
				w.logger.Error("failed to refresh watchlist", "error", err)
			}
		case <-w.stopChan:
			return
		}
	}
}

// List はwatchlistの全アイテムを返す
func (w *Watchlist) List() map[string]WatchItem {
	return w.items
}

func (w *Watchlist) Stop() {
	close(w.stopChan)
}
