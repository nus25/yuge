package logic

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/nus25/yuge/feed/config/types"
)

func TestBaseLogicBlockConfig_GetBlockType(t *testing.T) {
	config := &BaseLogicBlockConfig{
		BlockType: "test",
	}
	if got := config.GetBlockType(); got != "test" {
		t.Errorf("GetBlockType() = %v, want %v", got, "test")
	}
}

func TestBaseLogicBlockConfig_GetBlockName(t *testing.T) {
	tests := []struct {
		name     string
		config   *BaseLogicBlockConfig
		expected string
	}{
		{
			name: "BlockName specified",
			config: &BaseLogicBlockConfig{
				BlockName: "testBlock",
			},
			expected: "testBlock",
		},
		{
			name: "BlockName omitted",
			config: &BaseLogicBlockConfig{
				BlockName: "",
			},
			expected: "",
		},
		{
			name:     "BlockName omitted",
			config:   &BaseLogicBlockConfig{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.config.GetBlockName(); got != tt.expected {
				t.Errorf("GetBlockName() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestBaseLogicBlockConfig_GetOption(t *testing.T) {
	config := &BaseLogicBlockConfig{
		Options: map[string]interface{}{
			"key1": "value1",
			"key2": 123,
			"key3": true,
		},
	}

	tests := []struct {
		name     string
		key      string
		expected interface{}
	}{
		{
			name:     "存在するキー",
			key:      "key1",
			expected: "value1",
		},
		{
			name:     "存在しないキー",
			key:      "nonexistent",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := config.GetOption(tt.key)
			if result != tt.expected {
				t.Errorf("GetOption() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestBaseLogicBlockConfig_GetStringOption(t *testing.T) {
	config := &BaseLogicBlockConfig{
		Options: map[string]interface{}{
			"string": "value",
			"int":    123,
		},
	}

	tests := []struct {
		name          string
		blockName     string
		key           string
		expectedVal   string
		expectedFound bool
	}{
		{
			name:          "文字列の値が取得できる",
			blockName:     "testBlock",
			key:           "string",
			expectedVal:   "value",
			expectedFound: true,
		},
		{
			name:          "文字列以外の型は取得できない",
			blockName:     "testBlock",
			key:           "int",
			expectedVal:   "",
			expectedFound: false,
		},
		{
			name:          "存在しないキー",
			blockName:     "testBlock",
			key:           "nonexistent",
			expectedVal:   "",
			expectedFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, found := config.GetStringOption(tt.key)
			if val != tt.expectedVal {
				t.Errorf("GetStringOption() value = %v, want %v", val, tt.expectedVal)
			}
			if found != tt.expectedFound {
				t.Errorf("GetStringOption() found = %v, want %v", found, tt.expectedFound)
			}
		})
	}
}

func TestBaseLogicBlockConfig_GetStringArrayOption(t *testing.T) {
	config := &BaseLogicBlockConfig{
		Options: map[string]interface{}{
			"stringArray": []string{"item1", "item2"},
		},
	}

	tests := []struct {
		name          string
		key           string
		expectedVal   []string
		expectedFound bool
	}{
		{
			name:          "文字列配列の値が取得できる",
			key:           "stringArray",
			expectedVal:   []string{"item1", "item2"},
			expectedFound: true,
		},
		{
			name:          "存在しないキー",
			key:           "nonexistent",
			expectedVal:   nil,
			expectedFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, found := config.GetStringArrayOption(tt.key)
			if !reflect.DeepEqual(val, tt.expectedVal) {
				t.Errorf("GetStringArrayOption() value = %v, want %v", val, tt.expectedVal)
			}
			if found != tt.expectedFound {
				t.Errorf("GetStringArrayOption() found = %v, want %v", found, tt.expectedFound)
			}
		})
	}
}

func TestBaseLogicBlockConfig_GetIntOption(t *testing.T) {
	config := &BaseLogicBlockConfig{
		Options: map[string]interface{}{
			"int":     123,
			"uint64":  uint64(456),
			"float64": float64(789),
			"string":  "not a number",
		},
	}

	tests := []struct {
		name          string
		key           string
		expectedVal   int
		expectedFound bool
	}{
		{
			name:          "int型の値が取得できる",
			key:           "int",
			expectedVal:   123,
			expectedFound: true,
		},
		{
			name:          "uint64型の値が取得できる",
			key:           "uint64",
			expectedVal:   456,
			expectedFound: true,
		},
		{
			name:          "float64型の値が取得できる",
			key:           "float64",
			expectedVal:   789,
			expectedFound: true,
		},
		{
			name:          "数値以外の型は取得できない",
			key:           "string",
			expectedVal:   0,
			expectedFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, found := config.GetIntOption(tt.key)
			if val != tt.expectedVal {
				t.Errorf("GetIntOption() value = %v, want %v", val, tt.expectedVal)
			}
			if found != tt.expectedFound {
				t.Errorf("GetIntOption() found = %v, want %v", found, tt.expectedFound)
			}
		})
	}
}

func TestBaseLogicBlockConfig_GetBoolOption(t *testing.T) {
	config := &BaseLogicBlockConfig{
		Options: map[string]interface{}{
			"bool":   true,
			"string": "not a bool",
		},
	}

	tests := []struct {
		name          string
		key           string
		expectedVal   bool
		expectedFound bool
	}{
		{
			name:          "bool型の値が取得できる",
			key:           "bool",
			expectedVal:   true,
			expectedFound: true,
		},
		{
			name:          "bool以外の型は取得できない",
			key:           "string",
			expectedVal:   false,
			expectedFound: false,
		},
		{
			name:          "存在しないキー",
			key:           "nonexistent",
			expectedVal:   false,
			expectedFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, found := config.GetBoolOption(tt.key)
			if val != tt.expectedVal {
				t.Errorf("GetBoolOption() value = %v, want %v", val, tt.expectedVal)
			}
			if found != tt.expectedFound {
				t.Errorf("GetBoolOption() found = %v, want %v", found, tt.expectedFound)
			}
		})
	}
}

func TestBaseLogicBlockConfig_GetDurationOption(t *testing.T) {
	config := &BaseLogicBlockConfig{
		Options: map[string]interface{}{
			"string":     "5s",
			"duration":   5 * time.Second,
			"int64":      int64(5000000000),
			"float64":    float64(5000000000),
			"invalidStr": "invalid",
			"otherType":  true,
		},
	}

	tests := []struct {
		name          string
		key           string
		expectedVal   time.Duration
		expectedFound bool
	}{
		{
			name:          "文字列型の時間が取得できる",
			key:           "string",
			expectedVal:   5 * time.Second,
			expectedFound: true,
		},
		{
			name:          "Duration型の値が取得できる",
			key:           "duration",
			expectedVal:   5 * time.Second,
			expectedFound: true,
		},
		{
			name:          "int64型の値が取得できる",
			key:           "int64",
			expectedVal:   5 * time.Second,
			expectedFound: true,
		},
		{
			name:          "float64型の値が取得できる",
			key:           "float64",
			expectedVal:   5 * time.Second,
			expectedFound: true,
		},
		{
			name:          "無効な文字列は取得できない",
			key:           "invalidStr",
			expectedVal:   0,
			expectedFound: false,
		},
		{
			name:          "その他の型は取得できない",
			key:           "otherType",
			expectedVal:   0,
			expectedFound: false,
		},
		{
			name:          "存在しないキー",
			key:           "nonexistent",
			expectedVal:   0,
			expectedFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, found := config.GetDurationOption(tt.key)
			if val != tt.expectedVal {
				t.Errorf("GetDurationOption() value = %v, want %v", val, tt.expectedVal)
			}
			if found != tt.expectedFound {
				t.Errorf("GetDurationOption() found = %v, want %v", found, tt.expectedFound)
			}
		})
	}
}

func TestBaseLogicBlockConfig_Create(t *testing.T) {
	base := BaseLogicBlockConfig{
		BlockType: "testType",
		Options: map[string]interface{}{
			"key1": "value1",
			"key2": 123,
		},
	}

	created := base.Create(base)

	if created.GetBlockType() != base.BlockType {
		t.Errorf("Create() BlockType = %v, want %v", created.GetBlockType(), base.BlockType)
	}

	// Check options
	for key, expected := range base.Options {
		if val := created.GetOption(key); val != expected {
			t.Errorf("Create() option[%s] = %v, want %v", key, val, expected)
		}
	}
}

func TestBaseLogicBlockConfig_Update(t *testing.T) {
	// Setup test definitions
	config := &BaseLogicBlockConfig{
		BlockType: "test",
		Options:   make(map[string]interface{}),
		definitions: map[string]types.ConfigElementDefinition{
			"validKey": {
				Type:     types.ElementTypeString,
				Key:      "validKey",
				Required: false,
				Validator: func(value interface{}) error {
					return nil
				},
			},
			"intKey": {
				Type:     types.ElementTypeInt,
				Key:      "intKey",
				Required: false,
				Validator: func(value interface{}) error {
					return nil
				},
			},
			"validationFailKey": {
				Type:     types.ElementTypeString,
				Key:      "validationFailKey",
				Required: false,
				Validator: func(value interface{}) error {
					return errors.New("validation error")
				},
			},
		},
	}

	// Test valid update
	err := config.Update("validKey", "newValue")
	if err != nil {
		t.Errorf("Update() error = %v, want nil", err)
	}
	if val, _ := config.GetStringOption("validKey"); val != "newValue" {
		t.Errorf("Update() didn't set value correctly, got %v", val)
	}

	// Test invalid key
	err = config.Update("invalidKey", "value")
	if err == nil {
		t.Errorf("Update() error = nil, want error for invalid key")
	}

	// Test validation failure
	err = config.Update("validationFailKey", "value")
	if err == nil {
		t.Errorf("Update() error = nil, want validation error")
	}

	// Test conversion failure
	err = config.Update("intKey", "not an int")
	if err == nil {
		t.Errorf("Update() error = nil, want conversion error")
	}
}

func TestBaseLogicBlockConfig_DeepCopy(t *testing.T) {
	original := &BaseLogicBlockConfig{
		BlockType: "testType",
		Options: map[string]interface{}{
			"key1": "value1",
			"key2": 123,
			"key3": true,
		},
	}

	copy := original.DeepCopy()

	// Check that the copy has the same values
	if copy.GetBlockType() != original.BlockType {
		t.Errorf("DeepCopy() BlockType = %v, want %v", copy.GetBlockType(), original.BlockType)
	}

	// Check all options are copied
	for key, expected := range original.Options {
		if val := copy.GetOption(key); val != expected {
			t.Errorf("DeepCopy() option[%s] = %v, want %v", key, val, expected)
		}
	}

	// Verify it's a deep copy by modifying the original
	original.Options["key1"] = "modified"
	if val := copy.GetOption("key1"); val == "modified" {
		t.Errorf("DeepCopy() didn't create a deep copy, changes to original affected the copy")
	}
}
