package metadatautil

// Clone copies the common map and slice values used by Eino metadata.
func Clone(metadata map[string]any) map[string]any {
	if metadata == nil {
		return make(map[string]any)
	}
	cloned := make(map[string]any, len(metadata))
	for key, value := range metadata {
		cloned[key] = cloneValue(value)
	}
	return cloned
}

// MergePreserving adds missing keys without replacing values from base.
func MergePreserving(base, additions map[string]any) map[string]any {
	merged := Clone(base)
	for key, value := range additions {
		if _, exists := merged[key]; exists {
			continue
		}
		merged[key] = cloneValue(value)
	}
	return merged
}

func cloneValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return Clone(typed)
	case map[string]string:
		cloned := make(map[string]string, len(typed))
		for key, item := range typed {
			cloned[key] = item
		}
		return cloned
	case map[int]float64:
		cloned := make(map[int]float64, len(typed))
		for key, item := range typed {
			cloned[key] = item
		}
		return cloned
	case []any:
		cloned := make([]any, len(typed))
		for i, item := range typed {
			cloned[i] = cloneValue(item)
		}
		return cloned
	case []string:
		return append([]string(nil), typed...)
	case []int:
		return append([]int(nil), typed...)
	case []float64:
		return append([]float64(nil), typed...)
	case []byte:
		return append([]byte(nil), typed...)
	default:
		return value
	}
}
