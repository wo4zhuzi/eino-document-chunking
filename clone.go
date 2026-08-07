package chunking

import "github.com/cloudwego/eino/schema"

func cloneDocuments(documents []*schema.Document) []*schema.Document {
	cloned := make([]*schema.Document, len(documents))
	for i, document := range documents {
		cloned[i] = cloneDocument(document)
	}
	return cloned
}

func cloneDocument(document *schema.Document) *schema.Document {
	if document == nil {
		return nil
	}
	return &schema.Document{
		ID:       document.ID,
		Content:  document.Content,
		MetaData: cloneMetadata(document.MetaData),
	}
}

func cloneBlocks(blocks []Block) []Block {
	cloned := make([]Block, len(blocks))
	for i := range blocks {
		cloned[i] = blocks[i]
		cloned[i].SourceUnitIDs = append([]string(nil), blocks[i].SourceUnitIDs...)
		cloned[i].Metadata = cloneMetadata(blocks[i].Metadata)
	}
	return cloned
}

func cloneMetadata(metadata map[string]any) map[string]any {
	if metadata == nil {
		return make(map[string]any)
	}
	cloned := make(map[string]any, len(metadata))
	for key, value := range metadata {
		cloned[key] = cloneMetadataValue(value)
	}
	return cloned
}

func cloneMetadataValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneMetadata(typed)
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
			cloned[i] = cloneMetadataValue(item)
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

func mergeMetadataPreserving(base, additions map[string]any) map[string]any {
	merged := cloneMetadata(base)
	for key, value := range additions {
		if _, exists := merged[key]; exists {
			continue
		}
		merged[key] = cloneMetadataValue(value)
	}
	return merged
}
