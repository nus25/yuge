package subscriber

import (
	"fmt"
	"strings"
)

// parseHeaderFlags parses "-H"/"--feed-editor-header" values in the fixed
// "Key: Value" format into a header map for the gyoka projection client.
func parseHeaderFlags(values []string) (map[string]string, error) {
	headers := make(map[string]string, len(values))
	for _, v := range values {
		key, value, ok := strings.Cut(v, ":")
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if !ok || key == "" {
			return nil, fmt.Errorf("invalid header %q: expected format 'Key: Value'", v)
		}
		headers[key] = value
	}
	return headers, nil
}
