// Package maputil provides shared utilities for map and slice deep-copying
// used throughout the transformation and RGD assembly pipeline.
package maputil

// DeepCopyMap performs a deep copy of a map[string]interface{}.
func DeepCopyMap(src map[string]interface{}) map[string]interface{} {
	if src == nil {
		return nil
	}

	dst := make(map[string]interface{}, len(src))

	for k, v := range src {
		switch val := v.(type) {
		case map[string]interface{}:
			dst[k] = DeepCopyMap(val)
		case []interface{}:
			dst[k] = DeepCopySlice(val)
		default:
			dst[k] = v
		}
	}

	return dst
}

// DeepCopySlice performs a deep copy of a []interface{}.
func DeepCopySlice(src []interface{}) []interface{} {
	if src == nil {
		return nil
	}

	dst := make([]interface{}, len(src))

	for i, v := range src {
		switch val := v.(type) {
		case map[string]interface{}:
			dst[i] = DeepCopyMap(val)
		case []interface{}:
			dst[i] = DeepCopySlice(val)
		default:
			dst[i] = v
		}
	}

	return dst
}
