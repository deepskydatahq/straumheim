package record

import (
	"fmt"
)

// Flatten converts a nested map into a flat map with underscore-separated keys.
// Nested maps and slices are recursively flattened.
func Flatten(input map[string]any) map[string]any {
	result := make(map[string]any)
	flatten("", input, result)
	return result
}

func flatten(prefix string, input map[string]any, result map[string]any) {
	for k, v := range input {
		key := k
		if prefix != "" {
			key = prefix + "_" + k
		}
		flattenValue(key, v, result)
	}
}

func flattenValue(key string, v any, result map[string]any) {
	switch val := v.(type) {
	case map[string]any:
		flatten(key, val, result)
	case []any:
		for i, item := range val {
			indexedKey := fmt.Sprintf("%s_%d", key, i)
			flattenValue(indexedKey, item, result)
		}
	default:
		result[key] = v
	}
}

// EnsureFlattened populates the Flattened field from Payload if it is nil.
func (r *Record) EnsureFlattened() {
	if r.Flattened != nil {
		return
	}
	if r.Payload == nil {
		return
	}
	r.Flattened = Flatten(r.Payload)
}
