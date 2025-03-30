package logic

import (
	"testing"
)

func TestDropInLogicBlockConfig_ValidateAll(t *testing.T) {
	tests := []struct {
		name    string
		config  *BaseLogicBlockConfig
		wantErr bool
	}{
		{
			name: "正常系: 全ての必須フィールドが設定されている",
			config: &BaseLogicBlockConfig{
				BlockType: "dropin",
				Options: map[string]interface{}{
					"targetWord":     []string{"hello", "world"},
					"cancelWord":     []string{"bye", "goodbye"},
					"ignoreWord":     []string{"the", "a"},
					"expireDuration": "1h",
				},
			},
			wantErr: false,
		},
		{
			name: "異常系: targetWordが設定されていない",
			config: &BaseLogicBlockConfig{
				BlockType: "dropin",
				Options: map[string]interface{}{
					"cancelWord":     []string{"bye", "goodbye"},
					"ignoreWord":     []string{"the", "a"},
					"expireDuration": "1h",
				},
			},
			wantErr: true,
		},
		{
			name: "異常系: targetWordが空配列",
			config: &BaseLogicBlockConfig{
				BlockType: "dropin",
				Options: map[string]interface{}{
					"targetWord":     []string{},
					"cancelWord":     []string{"bye", "goodbye"},
					"ignoreWord":     []string{"the", "a"},
					"expireDuration": "1h",
				},
			},
			wantErr: true,
		},
		{
			name: "正常系: cancelWordが設定されていない",
			config: &BaseLogicBlockConfig{
				BlockType: "dropin",
				Options: map[string]interface{}{
					"targetWord":     []string{"hello", "world"},
					"ignoreWord":     []string{"the", "a"},
					"expireDuration": "1h",
				},
			},
			wantErr: false,
		},
		{
			name: "正常系: ignoreWordが設定されていない",
			config: &BaseLogicBlockConfig{
				BlockType: "dropin",
				Options: map[string]interface{}{
					"targetWord":     []string{"hello", "world"},
					"cancelWord":     []string{"bye", "goodbye"},
					"expireDuration": "1h",
				},
			},
			wantErr: false,
		},
		{
			name: "正常系: expireDurationが設定されていない",
			config: &BaseLogicBlockConfig{
				BlockType: "dropin",
				Options: map[string]interface{}{
					"targetWord": []string{"hello", "world"},
					"cancelWord": []string{"bye", "goodbye"},
					"ignoreWord": []string{"the", "a"},
				},
			},
			wantErr: false,
		},
		{
			name: "異常系: expireDurationが不正な形式",
			config: &BaseLogicBlockConfig{
				BlockType: "dropin",
				Options: map[string]interface{}{
					"targetWord":     []string{"hello", "world"},
					"cancelWord":     []string{"bye", "goodbye"},
					"ignoreWord":     []string{"the", "a"},
					"expireDuration": "invalid",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := (&DropInLogicBlockFactory{}).Create(*tt.config)
			if err != nil {
				t.Fatalf("Create() error = %v", err)
			}
			err = cfg.ValidateAll()
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateAll() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
