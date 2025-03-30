package logic

import (
	"testing"
)

func TestRegexLogicBlockConfig_ValidateAll(t *testing.T) {
	tests := []struct {
		name    string
		config  *BaseLogicBlockConfig
		wantErr bool
	}{
		{
			name: "Success: All required fields are set",
			config: &BaseLogicBlockConfig{
				Options: map[string]interface{}{
					"value":         "test",
					"invert":        false,
					"caseSensitive": true,
				},
			},
			wantErr: false,
		},
		{
			name: "Error: value is not set",
			config: &BaseLogicBlockConfig{
				Options: map[string]interface{}{
					"invert":        false,
					"caseSensitive": true,
				},
			},

			wantErr: true,
		},
		{
			name: "Error: caseSensitive is not set",
			config: &BaseLogicBlockConfig{
				Options: map[string]interface{}{
					"value":  "test",
					"invert": false,
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := (&RegexLogicBlockFactory{}).Create(*tt.config)
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

func TestRegexLogicBlockConfig_Validate(t *testing.T) {
	config, err := (&RegexLogicBlockFactory{}).Create(BaseLogicBlockConfig{
		Options: map[string]interface{}{
			"value":         "test",
			"invert":        false,
			"caseSensitive": true,
		},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	tests := []struct {
		name    string
		key     string
		value   interface{}
		wantErr bool
	}{
		{
			name:    "Success: valid value",
			key:     "value",
			value:   "test",
			wantErr: false,
		},
		{
			name:    "Error: empty value",
			key:     "value",
			value:   "",
			wantErr: true,
		},
		{
			name:    "Error: value is not string",
			key:     "value",
			value:   123,
			wantErr: true,
		},
		{
			name:    "Error: invalid regex pattern",
			key:     "value",
			value:   "[^test",
			wantErr: true,
		},
		{
			name:    "Success: valid invert",
			key:     "invert",
			value:   true,
			wantErr: false,
		},
		{
			name:    "Error: invert is not bool",
			key:     "invert",
			value:   "yes",
			wantErr: true,
		},
		{
			name:    "Success: valid caseSensitive",
			key:     "caseSensitive",
			value:   false,
			wantErr: false,
		},
		{
			name:    "Error: caseSensitive is not bool",
			key:     "caseSensitive",
			value:   "none",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := config.Validate(tt.key, tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRegexConfigElements_CaseSensitiveValidator(t *testing.T) {
	validator := RegexConfigElements[RegexOptionCaseSensitive].Validator

	tests := []struct {
		name    string
		value   interface{}
		wantErr bool
	}{
		{
			name:    "正常系: booleanのtrue",
			value:   true,
			wantErr: false,
		},
		{
			name:    "正常系: booleanのfalse",
			value:   false,
			wantErr: false,
		},
		{
			name:    "異常系: 文字列のfalse",
			value:   "false",
			wantErr: true,
		},
		{
			name:    "異常系: 数値の0",
			value:   0,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator(tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("CaseSensitiveValidator() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
