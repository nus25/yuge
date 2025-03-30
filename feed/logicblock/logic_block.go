package logicblock

import (
	"context"
	"log/slog"

	apibsky "github.com/bluesky-social/indigo/api/bsky"
	"github.com/nus25/yuge/feed/config/types"
	"github.com/nus25/yuge/feed/metrics"
)

// PreDeleteHandler is an interface for logic blocks that handle pre-delete events
type PreDeleteHandler interface {
	HandlePreDelete(did string, rkey string) error
}

type MetricProvider interface {
	GetMetrics() []metrics.Metric
}

type CommandProcessor interface {
	ProcessCommand(command string, args map[string]string) (message string, err error)
}

// LogicBlock represents a unit of logic that can be applied to posts
// for filtering and processing in the feed generation pipeline.
type LogicBlock interface {
	// BlockType returns the unique identifier of the logic block
	BlockType() string

	// BlockName returns the name of the logic block
	BlockName() string

	// Config returns the configuration for this logic block
	Config() types.LogicBlockConfig

	// Logger returns the logger instance for this logic block
	Logger() *slog.Logger

	// Test evaluates a post against the logic block's rules
	Test(did string, rkey string, post *apibsky.FeedPost) (result bool)

	// Reset resets any internal state of the logic block
	Reset() error

	// Shutdown performs cleanup when the logic block is being shut down
	Shutdown(ctx context.Context) error
}

// LogicBlockCreator is a function type that creates new logic block instances
type LogicBlockCreator func(types.LogicBlockConfig, *slog.Logger) (LogicBlock, error)
