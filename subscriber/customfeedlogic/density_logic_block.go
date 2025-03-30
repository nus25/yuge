package customfeedlogic

import (
	"context"
	"log/slog"
	"reflect"
	"regexp"
	"strconv"
	"unicode/utf8"

	apibsky "github.com/bluesky-social/indigo/api/bsky"
	"github.com/nus25/yuge/feed/config/types"
	"github.com/nus25/yuge/feed/logicblock"
)

// custom logic block must implement logicblock.LogicBlock interface
var _ logicblock.LogicBlock = &DensityLogicblock{} //type check

const BlockTypeDensity = "density"

// Register custom logic block
func init() {
	logicblock.FactoryInstance().RegisterCreator(BlockTypeDensity, NewDensityLogicBlock)
}

type DensityLogicblock struct {
	logicblock.BaseLogicblock
	threshold int
}

// creator function for custom logic block
func NewDensityLogicBlock(cfg types.LogicBlockConfig, logger *slog.Logger) (logicblock.LogicBlock, error) {
	thresholdVal := cfg.GetOption("threshold")
	logger.Info("new density logic block", "threshold", cfg.GetOption("threshold"))
	threshold := 10
	if thresholdVal != nil {
		switch v := thresholdVal.(type) {
		case float64: // JSON numbers are decoded as float64
			threshold = int(v)
		case int:
			threshold = v
		case string:
			var err error
			threshold, err = strconv.Atoi(v)
			if err != nil {
				logger.Warn("invalid threshold value, using default", "error", err, "value", v)
				threshold = 10
			}
		default:
			logger.Warn("unsupported threshold type, using default", "type", reflect.TypeOf(v), "value", v)
			threshold = 10
		}
	}

	return &DensityLogicblock{
		BaseLogicblock: *logicblock.NewBaseLogicblock(BlockTypeDensity, cfg, logger),
		threshold:      threshold,
	}, nil
}

var emojiRegex = regexp.MustCompile(`[\x{2700}-\x{27BF}]|[\x{E000}-\x{F8FF}]|\x{FE0F}|[\x{1F300}-\x{1F64F}]|[\x{1F680}-\x{1F6FF}]|[\x{2600}-\x{26FF}]|[\x{2011}-\x{26FF}]|[\x{1F900}-\x{1F9FF}]`)

// test function for custom logic block
// return true if the post is accepted, false otherwise
// post is accepted if the number of unique characters is greater than or equal to the threshold
func (l *DensityLogicblock) Test(did string, rkey string, post *apibsky.FeedPost) bool {
	if post.Text == "" {
		return false
	}

	charCount := make(map[rune]bool, l.threshold+1)
	for _, r := range post.Text {
		// Skip emoji characters
		if emojiRegex.MatchString(string(r)) {
			continue
		}
		// Count unique characters
		if utf8.ValidRune(r) {
			charCount[r] = true
		}
		if len(charCount) >= l.threshold {
			return true
		}
	}

	return false
}

// Reset is not implemented for this logic block
func (l *DensityLogicblock) Reset() error {
	return nil
}

// Shutdown is not implemented for this logic block
func (l *DensityLogicblock) Shutdown(ctx context.Context) error {
	return nil
}
