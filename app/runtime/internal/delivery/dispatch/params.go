package dispatch

import (
	"encoding/json"
	"fmt"
)

func unmarshal(raw json.RawMessage, dst any) error {
	if len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, dst)
}

// decodeStringParam pulls a single named string field out of a method's
// params object. key parameterises which JSON field to read so methods
// that name their id field differently share one decoder.
func decodeStringParam(raw json.RawMessage, key string) (string, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err == nil {
		if v, ok := obj[key]; ok {
			var s string
			if json.Unmarshal(v, &s) == nil && s != "" {
				return s, nil
			}
		}
	}
	return "", fmt.Errorf("%s is required", key)
}
