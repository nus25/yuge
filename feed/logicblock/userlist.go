package logicblock

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	apibsky "github.com/bluesky-social/indigo/api/bsky"
	config "github.com/nus25/yuge/feed/config/logic"
	"github.com/nus25/yuge/feed/config/types"
	"github.com/nus25/yuge/feed/errors"
	"github.com/nus25/yuge/feed/userlist"
)

var _ LogicBlock = (*UserListLogicblock)(nil) //type check
var _ CommandProcessor = (*UserListLogicblock)(nil)

const (
	BlockTypeUserList     = config.UserListBlockType
	UserListCommandList   = "list"
	UserListCommandReload = "reload"
)

func init() {
	FactoryInstance().RegisterCreator(BlockTypeUserList, NewUserListLogicBlock)
}

type UserListLogicblock struct {
	*BaseLogicblock
	listUri string
	allow   bool
	list    *userlist.UserList
}

func NewUserListLogicBlock(cfg types.LogicBlockConfig, logger *slog.Logger) (LogicBlock, error) {
	if cfg.GetBlockType() != BlockTypeUserList {
		logger.Error("invalid block type", "type", cfg.GetBlockType())
		return nil, errors.NewConfigError("block type", cfg.GetBlockType(), "invalid block type")
	}

	lcfg, ok := cfg.(*config.UserListLogicBlockConfig)
	if !ok {
		logger.Error("invalid config type", "type", fmt.Sprintf("%T", cfg))
		return nil, errors.NewConfigError("config type", fmt.Sprintf("%T", cfg), "invalid config type")
	}

	listUri, ok := lcfg.GetStringOption(config.UserListOptionUri)
	if !ok {
		logger.Error("listUri option not found")
		return nil, errors.NewConfigError(config.UserListOptionUri, "", "listUri option not found")
	}
	allow, ok := lcfg.GetBoolOption(config.UserListOptionAllow)
	if !ok {
		logger.Error("invalid allow option value")
		return nil, errors.NewConfigError(config.UserListOptionAllow, "", "invalid allow option value")
	}
	apiBaseURL, ok := lcfg.GetStringOption(config.UserListOptionApiBaseURL)
	var list *userlist.UserList
	var err error
	if ok {
		logger.Debug("apiBaseURL specified", "apiBaseURL", apiBaseURL)
		list, err = userlist.NewUserListWithHost(listUri, apiBaseURL, logger)
	} else {
		list, err = userlist.NewUserList(listUri, logger)
	}
	if err != nil {
		logger.Error("failed to create list", "error", err)
		return nil, fmt.Errorf("failed to create list: %w", err)
	}

	return &UserListLogicblock{
		BaseLogicblock: &BaseLogicblock{
			blockType: BlockTypeUserList,
			config:    cfg,
			logger:    logger,
		},
		listUri: listUri,
		allow:   allow,
		list:    list,
	}, nil
}

func (l *UserListLogicblock) Test(did string, rkey string, post *apibsky.FeedPost) bool {
	// リストに含まれているかチェック
	inList := l.list.Contain(did)

	// allowがtrueの場合、リストに含まれていればtrue
	// allowがfalseの場合、リストに含まれていればfalse
	return l.allow == inList
}

func (l *UserListLogicblock) Reset() error {
	l.list.Load()
	return nil
}

func (l *UserListLogicblock) Shutdown(ctx context.Context) error {
	return nil
}

func (l *UserListLogicblock) ProcessCommand(command string, args map[string]string) (message string, err error) {
	switch strings.ToLower(command) {
	case UserListCommandList:
		return fmt.Sprintf("%v", l.list.List()), nil
	case UserListCommandReload:
		err = l.Reset()
		if err != nil {
			return "", err
		}
		return "reload success", nil
	default:
		return "", fmt.Errorf("invalid command: %s", command)
	}
}
