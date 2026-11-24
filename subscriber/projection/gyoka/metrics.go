package gyoka

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var gyokaProjectionRequestDuration = promauto.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "gyoka_projection_request_duration_seconds",
		Help:    "Gyoka projection API request duration",
		Buckets: prometheus.ExponentialBuckets(0.001, 2, 16),
	},
	[]string{"operation", "status"},
)
