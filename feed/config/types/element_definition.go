package types

import (
	"strconv"
	"time"

	"github.com/nus25/yuge/feed/errors"
)

// ElementType represents the type of a configuration element
type ElementType string

const (
	ElementTypeString      ElementType = "string"
	ElementTypeInt         ElementType = "int"
	ElementTypeFloat       ElementType = "float"
	ElementTypeBool        ElementType = "bool"
	ElementTypeDuration    ElementType = "duration"
	ElementTypeMap         ElementType = "map"
	ElementTypeStringArray ElementType = "array"
)

// ConfigElementDefinition represents the definition of a configuration element
type ConfigElementDefinition struct {
	// Type of the element
	Type ElementType

	// Key of the element
	Key string

	// Default value
	DefaultValue interface{}

	// Whether this element is required
	Required bool

	// Function to validate the value
	Validator func(value interface{}) error

	// Description (for documentation generation)
	Description string
}

func ConvertStringArray(value interface{}) ([]string, error) {
	if _, ok := value.(string); ok {
		return []string{value.(string)}, nil
	}
	if a, ok := value.([]string); ok {
		return a, nil
	}
	var stringArray []string
	if interfaceArray, ok := value.([]interface{}); ok {
		for _, v := range interfaceArray {
			if _, ok := v.(string); !ok {
				return nil, errors.NewValidationError("", value, "must be a string array")
			}
			stringArray = append(stringArray, v.(string))
		}
		return stringArray, nil
	}
	return nil, errors.NewValidationError("", value, "cannot convert to string array")
}

// ConvertValue converts the value to the appropriate type
// value is string or same type as ElementType
func (def *ConfigElementDefinition) ConvertValue(value interface{}) (interface{}, error) {
	switch def.Type {
	case ElementTypeString:
		if str, ok := value.(string); ok {
			return str, nil
		}
		return nil, errors.NewValidationError("", value, "cannot convert to string")

	case ElementTypeInt:
		if i, ok := value.(int); ok {
			return i, nil
		}
		if i, ok := value.(uint64); ok {
			return int(i), nil
		}
		if f, ok := value.(float64); ok {
			return int(f), nil
		}
		if str, ok := value.(string); ok {
			if i, err := strconv.Atoi(str); err == nil {
				return i, nil
			}
		}
		return nil, errors.NewValidationError("", value, "cannot convert to integer")

	case ElementTypeFloat:
		if f, ok := value.(float64); ok {
			return f, nil
		}
		if i, ok := value.(int); ok {
			return float64(i), nil
		}
		if str, ok := value.(string); ok {
			if f, err := strconv.ParseFloat(str, 64); err == nil {
				return f, nil
			}
		}
		return nil, errors.NewValidationError("", value, "cannot convert to float")

	case ElementTypeBool:
		if b, ok := value.(bool); ok {
			return b, nil
		}
		if str, ok := value.(string); ok {
			if b, err := strconv.ParseBool(str); err == nil {
				return b, nil
			}
		}
		return nil, errors.NewValidationError("", value, "cannot convert to boolean")

	case ElementTypeDuration:
		if d, ok := value.(time.Duration); ok {
			return d, nil
		}
		if str, ok := value.(string); ok {
			if d, err := time.ParseDuration(str); err == nil {
				return d, nil
			}
		}
		return nil, errors.NewValidationError("", value, "cannot convert to duration")

	case ElementTypeMap:
		if m, ok := value.(map[string]interface{}); ok {
			return m, nil
		}
		return nil, errors.NewValidationError("", value, "cannot convert to map")

	case ElementTypeStringArray:
		return ConvertStringArray(value)
	}

	return nil, errors.NewValidationError("", value, "unknown type")
}

// ValidateType validates that the value type matches the element type
func (def *ConfigElementDefinition) ValidateType(key string, value interface{}) error {
	switch def.Type {
	case ElementTypeString:
		if _, ok := value.(string); !ok {
			return errors.NewValidationError(key, value, "must be a string")
		}
	case ElementTypeInt:
		// Try to convert string to integer
		if strVal, ok := value.(string); ok {
			if _, err := strconv.Atoi(strVal); err != nil {
				return errors.NewValidationError(key, value, "must be an integer or a string that can be converted to an integer")
			}
		} else if _, ok := value.(uint64); ok {
			return nil
		} else if _, ok := value.(float64); ok {
			return nil
		} else if _, ok := value.(int); !ok {
			return errors.NewValidationError(key, value, "must be an integer")
		}
	case ElementTypeFloat:
		// Try to convert string to float
		if strVal, ok := value.(string); ok {
			if _, err := strconv.ParseFloat(strVal, 64); err != nil {
				return errors.NewValidationError(key, value, "must be a float or a string that can be converted to a float")
			}
		} else if _, ok := value.(float64); !ok {
			return errors.NewValidationError(key, value, "must be a float")
		}
	case ElementTypeBool:
		// Try to convert string to boolean
		if strVal, ok := value.(string); ok {
			if _, err := strconv.ParseBool(strVal); err != nil {
				return errors.NewValidationError(key, value, "must be a boolean or a string that can be converted to a boolean")
			}
		} else if _, ok := value.(bool); !ok {
			return errors.NewValidationError(key, value, "must be a boolean")
		}
	case ElementTypeDuration:
		if _, ok := value.(time.Duration); ok {
			return nil
		}
		// Try to convert string to duration
		if strVal, ok := value.(string); ok {
			if _, err := time.ParseDuration(strVal); err != nil {
				return errors.NewValidationError(key, value, "must be a valid duration string (e.g. '5s', '1h30m')")
			}
		}
	case ElementTypeMap:
		if _, ok := value.(map[string]interface{}); !ok {
			return errors.NewValidationError(key, value, "must be a map")
		}
	case ElementTypeStringArray:
		if _, err := ConvertStringArray(value); err != nil {
			return errors.NewValidationError(key, value, "must be a string array")
		}
	}

	return nil
}
