package logic

// CustomLogicBlockConfig don't have validation funcs
type CustomLogicBlockConfig struct {
	BaseLogicBlockConfig
}

func (l *CustomLogicBlockConfig) ValidateAll() error {
	// custom logic block don't have validation funcs
	return nil
}

func (l *CustomLogicBlockConfig) Validate(key string, value interface{}) error {
	// custom logic block don't have validation funcs
	return nil
}

func (c *CustomLogicBlockConfig) Update(key string, value interface{}) error {
	c.Options[key] = value
	return nil
}
