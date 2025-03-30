package errors

import "fmt"

// ValidationError represents a configuration validation error
type ValidationError struct {
	Field   string // Field name that failed validation
	Value   any    // Value that was invalid
	Message string // Description of the validation error
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation failed for %s: %s (got: %v)", e.Field, e.Message, e.Value)
}

// NewValidationError creates a new ValidationError
func NewValidationError(field string, value any, message string) *ValidationError {
	return &ValidationError{
		Field:   field,
		Value:   value,
		Message: message,
	}
}

// ConfigError represents an error in the configuration structure or content
type ConfigError struct {
	Component string // Component name (e.g., "LogicBlock", "Feed", "Store")
	Key       string // Configuration key
	Message   string // Error description
}

func (e *ConfigError) Error() string {
	return fmt.Sprintf("configuration error in %s: %s (key: %s)", e.Component, e.Message, e.Key)
}

// NewConfigError creates a new ConfigError
func NewConfigError(component string, key string, message string) *ConfigError {
	return &ConfigError{
		Component: component,
		Key:       key,
		Message:   message,
	}
}

// DependencyError represents an error related to missing or invalid dependencies
type DependencyError struct {
	Component  string // Component that requires the dependency
	Dependency string // Name of the missing/invalid dependency
	Message    string // Error description
}

func (e *DependencyError) Error() string {
	return fmt.Sprintf("dependency error in %s: %s (dependency: %s)", e.Component, e.Message, e.Dependency)
}

// NewDependencyError creates a new DependencyError
func NewDependencyError(component string, dependency string, message string) *DependencyError {
	return &DependencyError{
		Component:  component,
		Dependency: dependency,
		Message:    message,
	}
}
