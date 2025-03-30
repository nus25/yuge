package logic

import (
	"testing"
)

func TestCustomLogicBlockConfig_Update(t *testing.T) {
	tests := []struct {
		name    string
		config  *CustomLogicBlockConfig
		key     string
		value   interface{}
		wantErr bool
	}{
		{
			name: "正常系: 新規キーの追加",
			config: &CustomLogicBlockConfig{
				BaseLogicBlockConfig: BaseLogicBlockConfig{
					Options: map[string]interface{}{},
				},
			},
			key:     "newKey",
			value:   "value",
			wantErr: false,
		},
		{
			name: "正常系: 既存キーの更新",
			config: &CustomLogicBlockConfig{
				BaseLogicBlockConfig: BaseLogicBlockConfig{
					Options: map[string]interface{}{
						"existingKey": "oldValue",
					},
				},
			},
			key:     "existingKey",
			value:   "newValue",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Update(tt.key, tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("Update() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got := tt.config.Options[tt.key]; got != tt.value {
				t.Errorf("Update() got = %v, want %v", got, tt.value)
			}
		})
	}
}

func TestCustomLogicBlockConfig_ValidateAll(t *testing.T) {
	config := &CustomLogicBlockConfig{
		BaseLogicBlockConfig: BaseLogicBlockConfig{
			Options: map[string]interface{}{
				"key": "value",
			},
		},
	}

	// カスタムロジックブロックはバリデーションを行わないため、常にnilを返す
	if err := config.ValidateAll(); err != nil {
		t.Errorf("ValidateAll() error = %v, want nil", err)
	}
}

func TestCustomLogicBlockConfig_Validate(t *testing.T) {
	config := &CustomLogicBlockConfig{
		BaseLogicBlockConfig: BaseLogicBlockConfig{
			Options: map[string]interface{}{
				"key": "value",
			},
		},
	}

	// カスタムロジックブロックはバリデーションを行わないため、常にnilを返す
	if err := config.Validate("key", "value"); err != nil {
		t.Errorf("Validate() error = %v, want nil", err)
	}

	// 存在しないキーでも常にnilを返す
	if err := config.Validate("nonexistent", "value"); err != nil {
		t.Errorf("Validate() error = %v, want nil", err)
	}
}
