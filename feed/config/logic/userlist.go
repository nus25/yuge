package logic

import (
	"github.com/bluesky-social/indigo/util"
	"github.com/nus25/yuge/feed/config/types"
	"github.com/nus25/yuge/feed/errors"
)

func init() {
	RegisterFactory(UserListBlockType, &UserListLogicBlockFactory{})
}

// listUri: string uri of the user list
// example: at://did:plc:xxx/app.bsky.graph.list/xxx
// allow: bool if true, only DIDs in the list will pass. if false, DIDs in the list will be blocked
// apiBaseURL: string base url of the user list api
type UserListLogicBlockConfig struct {
	BaseLogicBlockConfig
}

const (
	UserListBlockType        = "userlist"
	UserListOptionUri        = "listUri"    //required
	UserListOptionAllow      = "allow"      //required
	UserListOptionApiBaseURL = "apiBaseURL" //optional
)

// UserListLogicBlockFactory is a factory for creating UserListLogicBlockConfig
type UserListLogicBlockFactory struct{}

func (f *UserListLogicBlockFactory) Create(base BaseLogicBlockConfig) (types.LogicBlockConfig, error) {
	cfg := UserListLogicBlockConfig{BaseLogicBlockConfig: base}
	cfg.definitions = UserListConfigElements
	return &cfg, nil
}

var UserListConfigElements = map[string]types.ConfigElementDefinition{
	UserListOptionUri: {
		Type:         types.ElementTypeString,
		Key:          UserListOptionUri,
		DefaultValue: "",
		Required:     true,
		Validator: func(value interface{}) error {
			if _, ok := value.(string); !ok {
				return errors.NewValidationError(UserListOptionUri, value, "must be a string")
			}
			parsedUri, err := util.ParseAtUri(value.(string))
			if err != nil {
				return errors.NewValidationError(UserListOptionUri, value, "must be a valid uri")
			}
			if parsedUri.Collection != "app.bsky.graph.list" {
				return errors.NewValidationError(UserListOptionUri, value, "must be a valid user list uri")
			}
			return nil
		},
	},
	UserListOptionAllow: {
		Type:         types.ElementTypeBool,
		Key:          UserListOptionAllow,
		DefaultValue: false,
		Required:     true,
		Validator: func(value interface{}) error {
			if _, ok := value.(bool); !ok {
				return errors.NewValidationError(UserListOptionAllow, value, "must be a boolean")
			}
			return nil
		},
	},
	UserListOptionApiBaseURL: {
		Type:         types.ElementTypeString,
		Key:          UserListOptionApiBaseURL,
		DefaultValue: "https://public.api.bsky.app",
		Required:     false,
		Validator: func(value interface{}) error {
			if _, ok := value.(string); !ok {
				return errors.NewValidationError(UserListOptionApiBaseURL, value, "must be a string")
			}
			if value == "" {
				return errors.NewValidationError(UserListOptionApiBaseURL, value, "must not be empty")
			}
			return nil
		},
	},
}
