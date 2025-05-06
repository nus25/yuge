package feed

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	apibsky "github.com/bluesky-social/indigo/api/bsky"
	"github.com/bluesky-social/indigo/util"
	cfgTypes "github.com/nus25/yuge/feed/config/types"
	"github.com/nus25/yuge/feed/errors"
	"github.com/nus25/yuge/feed/logicblock"
	"github.com/nus25/yuge/feed/metrics"
	"github.com/nus25/yuge/feed/store"
	"github.com/nus25/yuge/feed/store/editor"
	"github.com/nus25/yuge/types"
)

var _ Feed = (*feedImpl)(nil) //type check

const (
	FeedMetricNamePostCount = "feed_post_count"
)

type Feed interface {
	FeedId() string
	FeedUri() string
	AddPost(did string, rkey string, cid string, t time.Time, langs []string) error
	DeletePost(did string, rkey string) error
	DeletePostByDid(did string) (deleted []types.Post, err error)
	GetPost(did string, rkey string) (post types.Post, exists bool)
	ListPost(did string) []types.Post
	Test(did string, rkey string, post *apibsky.FeedPost) bool
	PostCount() int
	Shutdown(ctx context.Context) error
	Clear() error
	Config() cfgTypes.FeedConfig
	Metrics() *metrics.Metrics
	ProcessCommand(logicBlockName string, command string, args map[string]string) (message string, err error)
}

type feedImpl struct {
	id          string
	uri         types.FeedUri
	config      cfgTypes.FeedConfig
	store       store.Store
	logicblocks []logicblock.LogicBlock
	logger      *slog.Logger
}

type FeedOptions struct {
	// feed configuration.
	Config cfgTypes.FeedConfig

	// StoreEditor is the interface for storing and retrieving feed data.
	StoreEditor editor.StoreEditor

	// Logger is an optional logger for feed operations.
	// If not specified, slog.Default() will be used.
	Logger *slog.Logger
}

func NewFeedWithOptions(ctx context.Context, feedId string, feedUri string, opts FeedOptions) (Feed, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// id check
	if feedId == "" {
		return nil, errors.NewDependencyError("Feed", "feedId", "feedId is required")
	}

	// feedUri validation
	_, err := util.ParseAtUri(feedUri)
	if err != nil {
		return nil, errors.NewDependencyError("Feed", "feedUri", fmt.Sprintf("invalid feedUri format: %v", err))
	}

	// logger
	var lg *slog.Logger
	if opts.Logger == nil {
		lg = slog.Default()
	} else {
		lg = opts.Logger
	}
	lg = lg.With("feed", feedId)

	lg.Info("initializing feed")
	cfg := opts.Config

	// store
	storeOpts := store.StoreOptions{
		FeedId:  feedId,
		FeedUri: types.FeedUri(feedUri),
		Config:  cfg.Store(),
		Editor:  opts.StoreEditor,
		Logger:  lg,
	}
	s, err := store.NewStore(ctx, storeOpts)
	if err != nil {
		return nil, errors.NewDependencyError("Feed", "store", fmt.Sprintf("failed to create store: %v", err))
	}
	if err := s.Load(ctx); err != nil {
		return nil, errors.NewDependencyError("Feed", "store", fmt.Sprintf("failed to load store: %v", err))
	}

	// 長い初期化処理の前に再度コンテキストをチェック
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// logicblock
	var logicblocks []logicblock.LogicBlock

	for _, blockCfg := range cfg.FeedLogic().GetLogicBlockConfigs() {
		// 各ブロックの作成時にもコンテキストをチェック
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		lg.Info("creating logic block", "block", blockCfg.GetBlockType())
		block, err := logicblock.FactoryInstance().Create(blockCfg, lg)
		if err != nil {
			lg.Error("failed to create logic block", "error", err)
			return nil, errors.NewDependencyError("Feed", "logicBlock", fmt.Sprintf("failed to create logic block: %v", err))
		}
		logicblocks = append(logicblocks, block)
	}

	// feed
	feed := &feedImpl{
		id:          feedId,
		uri:         types.FeedUri(feedUri),
		config:      opts.Config,
		store:       s,
		logicblocks: logicblocks,
		logger:      lg,
	}

	return feed, nil
}

func (f *feedImpl) FeedId() string {
	return f.id
}

func (f *feedImpl) FeedUri() string {
	return string(f.uri)
}

func (f *feedImpl) Shutdown(ctx context.Context) error {
	f.logger.Info("shutting down feed")

	if err := f.store.Shutdown(ctx); err != nil {
		f.logger.Error("failed to shutdown store", "error", err)
		return err
	}
	for _, b := range f.logicblocks {
		if err := b.Shutdown(ctx); err != nil {
			return err
		}
	}
	f.logger.Info("feed shutdown completed")
	return nil
}

func (f *feedImpl) Clear() error {
	f.logger.Info("resetting feed")
	//clear posts
	if err := f.store.Trim(0); err != nil {
		return err
	}
	//clear logicblocks
	for _, b := range f.logicblocks {
		if err := b.Reset(); err != nil {
			return err
		}
	}
	return nil
}

func (f *feedImpl) AddPost(did string, rkey string, cid string, t time.Time, langs []string) error {
	return f.store.Add(did, rkey, cid, t, langs)
}

func (f *feedImpl) DeletePost(did string, rkey string) error {
	for _, b := range f.logicblocks {
		if handler, ok := b.(logicblock.PreDeleteHandler); ok {
			if err := handler.HandlePreDelete(did, rkey); err != nil {
				return err
			}
		}
	}
	return f.store.Delete(did, rkey)
}
func (f *feedImpl) DeletePostByDid(did string) (deleted []types.Post, err error) {
	return f.store.DeleteByDid(did)
}

func (f *feedImpl) GetPost(did string, rkey string) (post types.Post, exists bool) {
	if p, exists := f.store.GetPost(did, rkey); exists {
		return *p, true
	}
	return types.Post{}, false
}

func (f *feedImpl) ListPost(did string) []types.Post {
	posts := f.store.List(did)
	result := make([]types.Post, len(posts))
	copy(result, posts)
	return result
}

// test if given post passes all logicblocks
func (f *feedImpl) Test(did string, rkey string, post *apibsky.FeedPost) bool {
	cfg := f.config
	if len(cfg.FeedLogic().GetLogicBlockConfigs()) == 0 {
		return false
	}

	for i, block := range f.logicblocks {
		var start time.Time
		if cfg.DetailedLog() {
			start = time.Now()
		}
		r := block.Test(did, rkey, post)
		if cfg.DetailedLog() {
			elapsed := time.Since(start)
			f.logger.Info("test",
				"block_index", i,
				"block", block.BlockType(),
				"result", r,
				"latency(ns)", elapsed)
		}
		if !r {
			return false
		}
	}
	//全てのテストをパスした場合はフィードに追加するポストとみなす
	return true
}

func (f *feedImpl) PostCount() int {
	return f.store.PostCount()
}

func (f *feedImpl) Config() cfgTypes.FeedConfig {
	cfg := f.config
	return cfg.DeepCopy()
}

func (f *feedImpl) Metrics() *metrics.Metrics {
	response := metrics.NewMetrics()
	//feed metrics
	response.AddMetric(metrics.NewMetric(FeedMetricNamePostCount, "post count of the feed", "", metrics.MetricTypeInt, int64(f.PostCount())))

	//logic block metrics
	for _, block := range f.logicblocks {
		if provider, ok := block.(logicblock.MetricProvider); ok {
			ms := provider.GetMetrics()
			for _, m := range ms {
				response.AddMetric(m)
			}
		}
	}
	return response
}

func (f *feedImpl) ProcessCommand(logicBlockName string, command string, args map[string]string) (message string, err error) {
	for _, block := range f.logicblocks {
		if block.BlockName() == logicBlockName {
			if processor, ok := block.(logicblock.CommandProcessor); ok {
				msg, err := processor.ProcessCommand(command, args)
				if err != nil {
					return "", err
				}
				return msg, nil
			}
		}
	}
	return "", fmt.Errorf("logic block not found: %s", logicBlockName)
}
