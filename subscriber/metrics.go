package subscriber

import (
	"github.com/nus25/yuge/feed"
	"github.com/nus25/yuge/feed/logicblock"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// 投稿の処理数
	postsProcessed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "subscriber_posts_processed_total",
		Help: "The total number of processed posts",
	})

	jetstreamErrorCount = promauto.NewCounter(prometheus.CounterOpts{
		Name: "jetstream_error_total",
		Help: "The total number of jetstream errors",
	})
	// フィードに追加された投稿数
	postsAdded = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "feed_posts_added_total",
		Help: "The total number of posts added to feed",
	}, []string{"feed_id"})

	// 削除された投稿数
	postsDeleted = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "feed_posts_deleted_total",
		Help: "The total number of deleted posts",
	}, []string{"feed_id"})

	// フィード内の投稿数
	feedPosts = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "feed_posts",
		Help: "The current number of posts in feed",
	}, []string{"feed_id"})
	// フィード判定速度
	feedLogicLatency = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "feed_logic_latency_seconds",
			Help:    "Feed logic processing latency",
			Buckets: prometheus.ExponentialBuckets(0.000001, 2, 10),
		},
		[]string{"feed_id"},
	)
	dropinListUserCount = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "feed_logic_dropin_list_user_count",
			Help: "The current number of users in dropin list",
		},
		[]string{"feed_id", "block_name"},
	)
)

func updateMetrics(f feed.Feed) {
	ms := f.Metrics()
	for _, m := range ms.GetMetrics() {
		switch m.MetricName {
		case feed.FeedMetricNamePostCount:
			feedPosts.WithLabelValues(f.FeedId()).Set(float64(m.IntValue))
		case logicblock.DropInLogicMetricDropinListUserCount:
			dropinListUserCount.WithLabelValues(f.FeedId(), m.MetricLabel).Set(float64(m.IntValue))
		}
	}
}
