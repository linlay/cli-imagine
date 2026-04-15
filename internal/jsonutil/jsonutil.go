package jsonutil

import (
	"encoding/json"
	"fmt"
)

// DeepCopyMap performs a deep copy of a map via JSON round-trip.
func DeepCopyMap(src map[string]any) (map[string]any, error) {
	if src == nil {
		return map[string]any{}, nil
	}
	payload, err := json.Marshal(src)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	dst := map[string]any{}
	if err := json.Unmarshal(payload, &dst); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	return dst, nil
}

// NormalizeAny converts an arbitrary value to its JSON-normalized form via round-trip.
func NormalizeAny(value any) (any, error) {
	switch typed := value.(type) {
	case nil:
		return nil, nil
	case map[string]any:
		return typed, nil
	case []any:
		return typed, nil
	case string, bool,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64,
		json.Number:
		return typed, nil
	default:
		payload, err := json.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("marshal: %w", err)
		}
		var decoded any
		if err := json.Unmarshal(payload, &decoded); err != nil {
			return nil, fmt.Errorf("unmarshal: %w", err)
		}
		return decoded, nil
	}
}
