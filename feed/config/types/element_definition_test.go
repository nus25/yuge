package types

import (
	"reflect"
	"testing"
	"time"
)

func TestConfigElementDefinition_ConvertValue(t *testing.T) {
	tests := []struct {
		name     string
		def      ConfigElementDefinition
		value    interface{}
		expected interface{}
		wantErr  bool
	}{
		{
			name:     "String type conversion",
			def:      ConfigElementDefinition{Type: ElementTypeString},
			value:    "test",
			expected: "test",
			wantErr:  false,
		},
		{
			name:     "String type conversion with int",
			def:      ConfigElementDefinition{Type: ElementTypeString},
			value:    123,
			expected: "123",
			wantErr:  true,
		},
		{
			name:     "String type conversion with nil",
			def:      ConfigElementDefinition{Type: ElementTypeString},
			value:    nil,
			expected: nil,
			wantErr:  true,
		},
		{
			name:     "Integer type conversion",
			def:      ConfigElementDefinition{Type: ElementTypeInt},
			value:    "123",
			expected: 123,
			wantErr:  false,
		},
		{
			name:     "Integer type conversion with int",
			def:      ConfigElementDefinition{Type: ElementTypeInt},
			value:    123,
			expected: 123,
			wantErr:  false,
		},
		{
			name:     "Integer type conversion with uint64",
			def:      ConfigElementDefinition{Type: ElementTypeInt},
			value:    uint64(123),
			expected: 123,
			wantErr:  false,
		},
		{
			name:     "Integer type conversion with float64",
			def:      ConfigElementDefinition{Type: ElementTypeInt},
			value:    3.14,
			expected: 3,
			wantErr:  false,
		},
		{
			name:     "Integer type conversion with nil",
			def:      ConfigElementDefinition{Type: ElementTypeInt},
			value:    nil,
			expected: nil,
			wantErr:  true,
		},
		{
			name:     "Float type conversion",
			def:      ConfigElementDefinition{Type: ElementTypeFloat},
			value:    "3.14",
			expected: 3.14,
			wantErr:  false,
		},
		{
			name:     "Float type conversion with float",
			def:      ConfigElementDefinition{Type: ElementTypeFloat},
			value:    3.14,
			expected: 3.14,
			wantErr:  false,
		},
		{
			name:     "Float type conversion with int",
			def:      ConfigElementDefinition{Type: ElementTypeFloat},
			value:    123,
			expected: 123.0,
			wantErr:  false,
		},
		{
			name:     "Float type conversion with nil",
			def:      ConfigElementDefinition{Type: ElementTypeFloat},
			value:    nil,
			expected: nil,
			wantErr:  true,
		},
		{
			name:     "Boolean type conversion with string",
			def:      ConfigElementDefinition{Type: ElementTypeBool},
			value:    "true",
			expected: true,
			wantErr:  false,
		},
		{
			name:     "Boolean type conversion with bool",
			def:      ConfigElementDefinition{Type: ElementTypeBool},
			value:    true,
			expected: true,
			wantErr:  false,
		},
		{
			name:     "Boolean type conversion with nil",
			def:      ConfigElementDefinition{Type: ElementTypeBool},
			value:    nil,
			expected: nil,
			wantErr:  true,
		},
		{
			name:     "Duration type conversion",
			def:      ConfigElementDefinition{Type: ElementTypeDuration},
			value:    "1h",
			expected: time.Hour,
			wantErr:  false,
		},
		{
			name:     "Duration type conversion with duration",
			def:      ConfigElementDefinition{Type: ElementTypeDuration},
			value:    time.Second,
			expected: time.Second,
			wantErr:  false,
		},
		{
			name:     "Duration type conversion with nil",
			def:      ConfigElementDefinition{Type: ElementTypeDuration},
			value:    nil,
			expected: nil,
			wantErr:  true,
		},
		{
			name:     "Map type conversion",
			def:      ConfigElementDefinition{Type: ElementTypeMap},
			value:    map[string]interface{}{"key": "value"},
			expected: map[string]interface{}{"key": "value"},
			wantErr:  false,
		},
		{
			name:     "Map type conversion with nil",
			def:      ConfigElementDefinition{Type: ElementTypeMap},
			value:    nil,
			expected: nil,
			wantErr:  true,
		},
		{
			name:     "Array type conversion",
			def:      ConfigElementDefinition{Type: ElementTypeStringArray},
			value:    []string{"1", "2", "3"},
			expected: []string{"1", "2", "3"},
			wantErr:  false,
		},
		{
			name:     "Array type conversion with nil",
			def:      ConfigElementDefinition{Type: ElementTypeStringArray},
			value:    nil,
			expected: nil,
			wantErr:  true,
		},
		{
			name:     "Invalid type conversion",
			def:      ConfigElementDefinition{Type: "invalid"},
			value:    "test",
			expected: nil,
			wantErr:  true,
		},
		{
			name:     "Invalid type conversion with nil",
			def:      ConfigElementDefinition{Type: "invalid"},
			value:    nil,
			expected: nil,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.def.ConvertValue(tt.value)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				// For map type, need to use reflect.DeepEqual for comparison
				if tt.def.Type == ElementTypeMap || tt.def.Type == ElementTypeStringArray {
					if !reflect.DeepEqual(got, tt.expected) {
						t.Errorf("expected %v but got %v", tt.expected, got)
					}
				} else if got != tt.expected {
					t.Errorf("expected %v but got %v", tt.expected, got)
				}
			}
		})
	}
}

func TestConfigElementDefinition_ValidateType(t *testing.T) {
	tests := []struct {
		name    string
		def     ConfigElementDefinition
		key     string
		value   interface{}
		wantErr bool
	}{
		{
			name:    "Valid string",
			def:     ConfigElementDefinition{Type: ElementTypeString},
			key:     "test_key",
			value:   "test",
			wantErr: false,
		},
		{
			name:    "Valid integer",
			def:     ConfigElementDefinition{Type: ElementTypeInt},
			key:     "test_key",
			value:   123,
			wantErr: false,
		},
		{
			name:    "Valid integer string",
			def:     ConfigElementDefinition{Type: ElementTypeInt},
			key:     "test_key",
			value:   "123",
			wantErr: false,
		},
		{
			name:    "Valid uint64 integer",
			def:     ConfigElementDefinition{Type: ElementTypeInt},
			key:     "test_key",
			value:   uint64(123),
			wantErr: false,
		},
		{
			name:    "Invalid integer string",
			def:     ConfigElementDefinition{Type: ElementTypeInt},
			key:     "test_key",
			value:   "not-a-number",
			wantErr: true,
		},
		{
			name:    "Invalid integer type",
			def:     ConfigElementDefinition{Type: ElementTypeInt},
			key:     "test_key",
			value:   []interface{}{1, 2, 3},
			wantErr: true,
		},
		{
			name:    "Valid float",
			def:     ConfigElementDefinition{Type: ElementTypeFloat},
			key:     "test_key",
			value:   3.14,
			wantErr: false,
		},
		{
			name:    "Valid float string",
			def:     ConfigElementDefinition{Type: ElementTypeFloat},
			key:     "test_key",
			value:   "3.14",
			wantErr: false,
		},
		{
			name:    "Invalid float string",
			def:     ConfigElementDefinition{Type: ElementTypeFloat},
			key:     "test_key",
			value:   "not-a-float",
			wantErr: true,
		},
		{
			name:    "Invalid float type",
			def:     ConfigElementDefinition{Type: ElementTypeFloat},
			key:     "test_key",
			value:   true,
			wantErr: true,
		},
		{
			name:    "Valid boolean",
			def:     ConfigElementDefinition{Type: ElementTypeBool},
			key:     "test_key",
			value:   true,
			wantErr: false,
		},
		{
			name:    "Valid boolean string true",
			def:     ConfigElementDefinition{Type: ElementTypeBool},
			key:     "test_key",
			value:   "true",
			wantErr: false,
		},
		{
			name:    "Valid boolean string false",
			def:     ConfigElementDefinition{Type: ElementTypeBool},
			key:     "test_key",
			value:   "false",
			wantErr: false,
		},
		{
			name:    "Invalid boolean string",
			def:     ConfigElementDefinition{Type: ElementTypeBool},
			key:     "test_key",
			value:   "not-a-boolean",
			wantErr: true,
		},
		{
			name:    "Invalid boolean type",
			def:     ConfigElementDefinition{Type: ElementTypeBool},
			key:     "test_key",
			value:   123,
			wantErr: true,
		},
		{
			name:    "Valid duration",
			def:     ConfigElementDefinition{Type: ElementTypeDuration},
			key:     "test_key",
			value:   time.Hour,
			wantErr: false,
		},
		{
			name:    "Valid duration string",
			def:     ConfigElementDefinition{Type: ElementTypeDuration},
			key:     "test_key",
			value:   "1h30m",
			wantErr: false,
		},
		{
			name:    "Invalid duration string",
			def:     ConfigElementDefinition{Type: ElementTypeDuration},
			key:     "test_key",
			value:   "not-a-duration",
			wantErr: true,
		},
		{
			name:    "Valid map",
			def:     ConfigElementDefinition{Type: ElementTypeMap},
			key:     "test_key",
			value:   map[string]interface{}{"key": "value"},
			wantErr: false,
		},
		{
			name:    "Invalid map type",
			def:     ConfigElementDefinition{Type: ElementTypeMap},
			key:     "test_key",
			value:   "not-a-map",
			wantErr: true,
		},
		{
			name:    "Valid array",
			def:     ConfigElementDefinition{Type: ElementTypeStringArray},
			key:     "test_key",
			value:   []string{"item1", "item2"},
			wantErr: false,
		},
		{
			name:    "valid array type(single string)",
			def:     ConfigElementDefinition{Type: ElementTypeStringArray},
			key:     "test_key",
			value:   "not-an-array",
			wantErr: false,
		},
		{
			name:    "Invalid array type(int)",
			def:     ConfigElementDefinition{Type: ElementTypeStringArray},
			key:     "test_key",
			value:   123,
			wantErr: true,
		},
		{
			name:    "Invalid type",
			def:     ConfigElementDefinition{Type: ElementTypeString},
			key:     "test_key",
			value:   123,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.def.ValidateType(tt.key, tt.value)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}
