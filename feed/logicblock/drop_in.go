package logicblock

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	apibsky "github.com/bluesky-social/indigo/api/bsky"
	config "github.com/nus25/yuge/feed/config/logic"
	"github.com/nus25/yuge/feed/config/types"
	"github.com/nus25/yuge/feed/errors"
	"github.com/nus25/yuge/feed/metrics"
	"github.com/nus25/yuge/feed/watchlist"
)

// type check
var _ LogicBlock = (*DropInLogicblock)(nil)
var _ CommandProcessor = (*DropInLogicblock)(nil)
var _ MetricProvider = (*DropInLogicblock)(nil)
var _ StatefulLogicBlock = (*DropInLogicblock)(nil)

const (
	BlockTypeDropIn                      = config.DropInBlockType
	DropInLogicMetricDropinListUserCount = "dropin_list_user_count"
	DropInCommandReset                   = "reset"
	DropInCommandAdd                     = "add"
	DropInCommandDelete                  = "delete"
	DropinCommandList                    = "list"
)

func init() {
	FactoryInstance().RegisterCreator(BlockTypeDropIn, NewDropInLogicBlock)
}

type DropInLogicblock struct {
	*BaseLogicblock
	expireDuration time.Duration
	targetWord     []string
	cancelWord     []string
	ignoreWord     []string
	watchlist      *watchlist.Watchlist
	stateFeedID    string
	stateBlockKey  string
	stateStore     StateRepository
}

func NewDropInLogicBlock(cfg types.LogicBlockConfig, logger *slog.Logger) (LogicBlock, error) {
	if cfg.GetBlockType() != BlockTypeDropIn {
		logger.Error("invalid block type", "type", cfg.GetBlockType())
		return nil, errors.NewConfigError("block type", cfg.GetBlockType(), "invalid block type")
	}

	dcfg, ok := cfg.(*config.DropInLogicBlockConfig)
	if !ok {
		logger.Error("invalid config type", "type", fmt.Sprintf("%T", cfg))
		return nil, errors.NewConfigError("config type", fmt.Sprintf("%T", cfg), "invalid config type")
	}

	// targetWord
	tw, ok := dcfg.GetStringArrayOption(config.DropInOptionTargetWord)
	if !ok {
		logger.Error("targetWord option not found")
		return nil, errors.NewConfigError(config.DropInOptionTargetWord, "", "targetWord option not found")
	}
	if len(tw) == 0 {
		logger.Error("targetWord must not be empty")
		return nil, errors.NewConfigError(config.DropInOptionTargetWord, fmt.Sprintf("%v", tw), "targetWord must not be empty")
	}
	// convert to lower case
	for i := range tw {
		tw[i] = strings.ToLower(tw[i])
	}

	// cancelWord (optional)
	cw, ok := dcfg.GetStringArrayOption(config.DropInOptionCancelWord)
	if !ok {
		cw = []string{}
	}
	// convert to lower case
	for i := range cw {
		cw[i] = strings.ToLower(cw[i])
	}

	// ignoreWord (optional)
	iw, ok := dcfg.GetStringArrayOption(config.DropInOptionIgnoreWord)
	if !ok {
		iw = []string{}
	}
	// convert to lower case
	for i := range iw {
		iw[i] = strings.ToLower(iw[i])
	}

	// expireDuration (optional)
	ed, ok := dcfg.GetDurationOption(config.DropInOptionExpireDuration)
	if !ok {
		ed = time.Duration(0)
	}

	// watchlist
	wl, err := watchlist.NewWatchlist(ed)
	if err != nil {
		logger.Error("failed to create watchlist", "error", err)
		return nil, errors.NewConfigError("drop in logic block", "", "failed to create watchlist")
	}

	return &DropInLogicblock{
		BaseLogicblock: &BaseLogicblock{
			blockType: BlockTypeDropIn,
			config:    cfg,
			logger:    logger,
		},
		expireDuration: ed,
		targetWord:     tw,
		cancelWord:     cw,
		ignoreWord:     iw,
		watchlist:      wl,
	}, nil
}

func (d *DropInLogicblock) Reset() error {
	d.logger.Info("resetting drop-in block")
	d.watchlist.Clear()
	d.saveState()
	return nil
}

func (d *DropInLogicblock) Shutdown(ctx context.Context) error {
	d.watchlist.Stop()
	return nil
}

func (d *DropInLogicblock) Test(did string, rkey string, post *apibsky.FeedPost) bool {
	txt := strings.ToLower(post.Text)
	// cancelWord
	for _, w := range d.cancelWord {
		if strings.Contains(txt, w) {
			d.watchlist.Delete(did)
			d.saveState()
			return false
		}
	}

	// ignoreWord
	for _, w := range d.ignoreWord {
		if strings.Contains(txt, w) {
			return false
		}
	}

	// check did is in watchlist
	if d.watchlist.Contains(did) != nil {
		return true
	}

	// if targetWord is in post.Text, add to watchlist
	for _, w := range d.targetWord {
		if strings.Contains(txt, w) {
			d.watchlist.Add(did, rkey)
			d.saveState()
			return true
		}
	}

	return false
}

func (d *DropInLogicblock) HandlePreDelete(_ PostStore, did string, rkey string) ([]PostMutation, error) {
	return nil, nil
}

func (d *DropInLogicblock) HandlePostDelete(did string, rkey string) {
	item := d.watchlist.Contains(did)
	if item == nil {
		return
	}
	// if trigger post is deleted, delete from watchlist
	if item.RKey == rkey {
		d.watchlist.Delete(did)
		d.saveState()
	}
}

func (d *DropInLogicblock) RestoreState(ctx context.Context, feedID string, blockKey string, repository StateRepository) error {
	d.stateFeedID = feedID
	d.stateBlockKey = blockKey
	d.stateStore = repository

	state, found, err := repository.LoadLogicBlockState(ctx, feedID, blockKey)
	if err != nil {
		return fmt.Errorf("load drop-in state: %w", err)
	}
	if !found {
		return nil
	}
	items := make(map[string]watchlist.WatchItem)
	if err := json.Unmarshal(state, &items); err != nil {
		return fmt.Errorf("unmarshal drop-in state: %w", err)
	}
	d.watchlist.Restore(items)
	return nil
}

func (d *DropInLogicblock) saveState() {
	if d.stateStore == nil {
		return
	}
	state, err := json.Marshal(d.watchlist.List())
	if err != nil {
		d.logger.Error("failed to marshal drop-in state", "error", err)
		return
	}
	if err := d.stateStore.SaveLogicBlockState(context.Background(), d.stateFeedID, d.stateBlockKey, 1, state); err != nil {
		d.logger.Error("failed to save drop-in state", "error", err)
	}
}

func (d *DropInLogicblock) GetMetrics() []metrics.Metric {
	ms := []metrics.Metric{}
	ms = append(ms, metrics.NewMetric(DropInLogicMetricDropinListUserCount, "dropin list user count", d.BlockName(), metrics.MetricTypeInt, int64(len(d.watchlist.List()))))
	return ms
}

func (d *DropInLogicblock) ProcessCommand(command string, args map[string]string) (message string, err error) {
	switch strings.ToLower(command) {
	case DropInCommandReset:
		err = d.Reset()
		if err != nil {
			return "", err
		}
		return "reset success", nil
	case DropInCommandAdd:
		did := args["did"]
		rkey := args["rkey"]
		if did == "" || rkey == "" {
			return "", fmt.Errorf("invalid command parameters: %s did: %s rkey: %s", command, did, rkey)
		}
		d.watchlist.Add(did, rkey)
		d.saveState()
		return "add success", nil
	case DropInCommandDelete:
		did := args["did"]
		if did == "" {
			return "", fmt.Errorf("invalid command parameters: %s did: %s", command, did)
		}
		d.watchlist.Delete(did)
		d.saveState()
		return "delete success", nil
	case DropinCommandList:
		list := d.watchlist.List()
		listStr := ""
		for did, item := range list {
			if listStr != "" {
				listStr += ", "
			}
			listStr += fmt.Sprintf("{did: %s, rkey: %s, expireAt: %s}", did, item.RKey, item.ExpireAt.UTC().Format(time.RFC3339))
		}
		return fmt.Sprintf("list success: [%s]", listStr), nil
	default:
		return "", fmt.Errorf("invalid command: %s", command)
	}
}
