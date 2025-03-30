package logic

import (
	"testing"

	"github.com/nus25/yuge/feed/config/types"
)

func TestRemoveLogicBlockConfig_ValidateAll(t *testing.T) {
	tests := []struct {
		name    string
		config  *BaseLogicBlockConfig
		wantErr bool
	}{
		// subject: item のテストケース
		{
			name: "正常系: subject:item 時にvalueが設定されている",
			config: &BaseLogicBlockConfig{
				Options: map[string]interface{}{
					"subject": "item",
					"value":   "reply",
				},
			},
			wantErr: false,
		},
		{
			name: "正常系: subject:item 時に別の有効なvalueが設定されている",
			config: &BaseLogicBlockConfig{
				Options: map[string]interface{}{
					"subject": "item",
					"value":   "repost",
				},
			},
			wantErr: false,
		},
		{
			name: "異常系: subject:item 時に value が設定されていない",
			config: &BaseLogicBlockConfig{
				Options: map[string]interface{}{
					"subject": "item",
				},
			},
			wantErr: true,
		},
		{
			name: "異常系: subject:item 時に無効な value が設定されている",
			config: &BaseLogicBlockConfig{
				Options: map[string]interface{}{
					"subject": "item",
					"value":   "invalid",
				},
			},
			wantErr: true,
		},
		{
			name: "異常系: subject:item 時に value が文字列でない",
			config: &BaseLogicBlockConfig{
				Options: map[string]interface{}{
					"subject": "item",
					"value":   123,
				},
			},
			wantErr: true,
		},

		// subject: language のテストケース
		{
			name: "正常系: subject:language 時にlanguageとoperatorが設定されている",
			config: &BaseLogicBlockConfig{
				Options: map[string]interface{}{
					"subject":  "language",
					"language": "en",
					"operator": "==",
				},
			},
			wantErr: false,
		},
		{
			name: "正常系: subject:language 時に別の有効なoperatorが設定されている",
			config: &BaseLogicBlockConfig{
				Options: map[string]interface{}{
					"subject":  "language",
					"language": "ja",
					"operator": "!=",
				},
			},
			wantErr: false,
		},
		{
			name: "異常系: subject:language 時に language が設定されていない",
			config: &BaseLogicBlockConfig{
				Options: map[string]interface{}{
					"subject":  "language",
					"operator": "==",
				},
			},
			wantErr: true,
		},
		{
			name: "異常系: subject:language 時に operator が設定されていない",
			config: &BaseLogicBlockConfig{
				Options: map[string]interface{}{
					"subject":  "language",
					"language": "en",
				},
			},
			wantErr: true,
		},
		{
			name: "異常系: subject:language 時に language が空文字列",
			config: &BaseLogicBlockConfig{
				Options: map[string]interface{}{
					"subject":  "language",
					"language": "",
					"operator": "==",
				},
			},
			wantErr: true,
		},
		{
			name: "異常系: subject:language 時に language が文字列でない",
			config: &BaseLogicBlockConfig{
				Options: map[string]interface{}{
					"subject":  "language",
					"language": 123,
					"operator": "==",
				},
			},
			wantErr: true,
		},
		{
			name: "異常系: subject:language 時に無効な operator が設定されている",
			config: &BaseLogicBlockConfig{
				Options: map[string]interface{}{
					"subject":  "language",
					"language": "en",
					"operator": "<",
				},
			},
			wantErr: true,
		},
		{
			name: "異常系: subject:language 時に operator が文字列でない",
			config: &BaseLogicBlockConfig{
				Options: map[string]interface{}{
					"subject":  "language",
					"language": "en",
					"operator": 123,
				},
			},
			wantErr: true,
		},

		// 共通のテストケース
		{
			name: "異常系: subject が設定されていない",
			config: &BaseLogicBlockConfig{
				Options: map[string]interface{}{
					"value":    "reply",
					"language": "en",
					"operator": "==",
				},
			},
			wantErr: true,
		},
		{
			name: "異常系: 無効な subject が設定されている",
			config: &BaseLogicBlockConfig{
				Options: map[string]interface{}{
					"subject": "invalid",
					"value":   "test",
				},
			},
			wantErr: true,
		},
		{
			name: "異常系: subject が文字列でない",
			config: &BaseLogicBlockConfig{
				Options: map[string]interface{}{
					"subject": 123,
					"value":   "reply",
				},
			},
			wantErr: true,
		},
		{
			name: "異常系: 空の Options",
			config: &BaseLogicBlockConfig{
				Options: map[string]interface{}{},
			},
			wantErr: true,
		},
		{
			name: "異常系: nil Options",
			config: &BaseLogicBlockConfig{
				Options: nil,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := (&RemoveLogicBlockFactory{}).Create(*tt.config)
			if err != nil {
				if tt.wantErr {
					return
				}
				t.Fatalf("Create() error = %v, wantErr %v", err, tt.wantErr)
			}
			err = r.ValidateAll()
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateAll() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRemoveLogicBlockConfig_Validate(t *testing.T) {
	var err error
	// subject: item のテストケース用の設定
	itemCfg, err := (&RemoveLogicBlockFactory{}).Create(
		BaseLogicBlockConfig{
			Options: map[string]interface{}{
				"subject": "item",
				"value":   "reply",
			},
		})

	// subject: language のテストケース用の設定
	languageCfg, err := (&RemoveLogicBlockFactory{}).Create(
		BaseLogicBlockConfig{
			Options: map[string]interface{}{
				"subject":  "language",
				"language": "en",
				"operator": "==",
			},
		})

	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// test required subject field
	_, err = (&RemoveLogicBlockFactory{}).Create(
		BaseLogicBlockConfig{
			Options: map[string]interface{}{},
		},
	)
	if err == nil {
		t.Fatalf("Create() error = %v", err)
	}

	tests := []struct {
		name    string
		config  types.LogicBlockConfig
		key     string
		value   interface{}
		wantErr bool
	}{
		// subject のテストケース
		{
			name:    "正常系: 有効な subject (item)",
			config:  itemCfg,
			key:     "subject",
			value:   "item",
			wantErr: false,
		},
		{
			name:    "正常系: 有効な subject (language)",
			config:  languageCfg,
			key:     "subject",
			value:   "language",
			wantErr: false,
		},
		{
			name:    "異常系: 無効な subject",
			config:  itemCfg,
			key:     "subject",
			value:   "invalid",
			wantErr: true,
		},
		{
			name:    "異常系: 空の subject",
			config:  languageCfg,
			key:     "subject",
			value:   "",
			wantErr: true,
		},
		{
			name:    "異常系: subject が string 以外",
			config:  languageCfg,
			key:     "subject",
			value:   123,
			wantErr: true,
		},

		// subject: item の場合の value のテストケース
		{
			name:    "正常系: 有効な value (reply)",
			config:  itemCfg,
			key:     "value",
			value:   "reply",
			wantErr: false,
		},
		{
			name:    "正常系: 有効な value (repost)",
			config:  itemCfg,
			key:     "value",
			value:   "repost",
			wantErr: false,
		},
		{
			name:    "異常系: 無効な value",
			config:  itemCfg,
			key:     "value",
			value:   "invalid",
			wantErr: true,
		},
		{
			name:    "異常系: 空の value",
			config:  itemCfg,
			key:     "value",
			value:   "",
			wantErr: true,
		},
		{
			name:    "異常系: value が string 以外",
			config:  itemCfg,
			key:     "value",
			value:   123,
			wantErr: true,
		},

		// subject: language の場合の language のテストケース
		{
			name:    "正常系: 有効な language",
			config:  languageCfg,
			key:     "language",
			value:   "en",
			wantErr: false,
		},
		{
			name:    "正常系: 別の有効な language",
			config:  languageCfg,
			key:     "language",
			value:   "ja",
			wantErr: false,
		},
		{
			name:    "異常系: 空の language",
			config:  languageCfg,
			key:     "language",
			value:   "",
			wantErr: true,
		},
		{
			name:    "異常系: language が string 以外",
			config:  languageCfg,
			key:     "language",
			value:   123,
			wantErr: true,
		},

		// subject: language の場合の operator のテストケース
		{
			name:    "正常系: 有効な operator ==",
			config:  languageCfg,
			key:     "operator",
			value:   "==",
			wantErr: false,
		},
		{
			name:    "正常系: 有効な operator !=",
			config:  languageCfg,
			key:     "operator",
			value:   "!=",
			wantErr: false,
		},
		{
			name:    "異常系: 無効な operator",
			config:  languageCfg,
			key:     "operator",
			value:   ">",
			wantErr: true,
		},
		{
			name:    "異常系: 空の operator",
			config:  languageCfg,
			key:     "operator",
			value:   "",
			wantErr: true,
		},
		{
			name:    "異常系: operator が string 以外",
			config:  languageCfg,
			key:     "operator",
			value:   123,
			wantErr: true,
		},

		// 無関係なキーのテストケース
		{
			name:    "異常系: 無関係なキー (item)",
			config:  itemCfg,
			key:     "unknown",
			value:   "test",
			wantErr: true,
		},
		{
			name:    "異常系: 無関係なキー (language)",
			config:  languageCfg,
			key:     "unknown",
			value:   "test",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate(tt.key, tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
