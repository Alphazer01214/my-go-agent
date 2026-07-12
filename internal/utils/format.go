package utils

import (
	"encoding/json"
	"strings"
)

func ParseJSON(content string, v interface{}) error {
	text := strings.TrimSpace(content)

	if err := json.Unmarshal([]byte(text), v); err != nil {
		return err
	}

	// todo:
	// 1. ```json ```
	// 2. text+json+text
	// 3. many json
	
	return nil
}
