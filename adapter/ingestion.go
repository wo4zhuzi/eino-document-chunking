package adapter

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/cloudwego/eino/schema"
	chunking "github.com/wo4zhuzi/eino-document-chunking"
	ingestion "github.com/wo4zhuzi/eino-document-ingestion"
)

const ingestionAdapterName = "ingestion"

// IngestionAdapter consumes the standard output contract of eino-document-ingestion.
type IngestionAdapter struct {
	delegate   chunking.FormatAdapter
	structured bool
}

// NewIngestionAdapter selects plain or structured conversion from ParserInfo.Output.
func NewIngestionAdapter(info ingestion.ParserInfo) (*IngestionAdapter, error) {
	granularity := info.Output.Granularity
	if granularity == "" {
		granularity = ingestion.GranularityDocument
	}
	switch granularity {
	case ingestion.GranularityDocument,
		ingestion.GranularityPage,
		ingestion.GranularitySection,
		ingestion.GranularityRow,
		ingestion.GranularityBlock:
	default:
		return nil, fmt.Errorf("%w: unsupported ingestion granularity %q", chunking.ErrInvalidConfig, granularity)
	}
	if info.Output.Structured && granularity != ingestion.GranularityBlock {
		return nil, fmt.Errorf("%w: structured ingestion output must use block granularity", chunking.ErrInvalidConfig)
	}

	var delegate chunking.FormatAdapter = NewDocumentAdapter()
	if info.Output.Structured {
		structured, err := NewStructuredDocumentAdapter(StructuredDocumentAdapterConfig{
			Resolver: ingestionStructureResolver{},
		})
		if err != nil {
			return nil, err
		}
		delegate = structured
	}
	return &IngestionAdapter{delegate: delegate, structured: info.Output.Structured}, nil
}

// Name implements chunking.FormatAdapter.
func (*IngestionAdapter) Name() string {
	return ingestionAdapterName
}

// Adapt implements chunking.FormatAdapter.
func (adapter *IngestionAdapter) Adapt(
	ctx context.Context,
	documents []*schema.Document,
) ([]chunking.Block, error) {
	if adapter == nil || adapter.delegate == nil {
		return nil, fmt.Errorf("%w: ingestion adapter is unavailable", chunking.ErrInvalidConfig)
	}
	blocks, err := adapter.delegate.Adapt(ctx, documents)
	if err != nil {
		return nil, err
	}
	if adapter.structured {
		normalizeIngestionStructurePaths(blocks)
	}
	return blocks, nil
}

func normalizeIngestionStructurePaths(blocks []chunking.Block) {
	labelByID := make(map[string]string, len(blocks))
	for _, block := range blocks {
		label, ok := block.Metadata[ingestion.MetadataStructureLabel].(string)
		if ok && strings.TrimSpace(label) != "" {
			labelByID[block.ID] = strings.TrimSpace(label)
		}
	}
	for index := range blocks {
		structure := blocks[index].Structure
		if structure == nil {
			continue
		}
		sectionPath := structure.Path
		if structure.Kind != chunking.BlockKindHeading &&
			len(sectionPath) > 0 && sectionPath[len(sectionPath)-1] == blocks[index].ID {
			sectionPath = sectionPath[:len(sectionPath)-1]
		}
		structure.Path = append([]string(nil), sectionPath...)
		structure.SemanticPath = make([]string, 0, len(sectionPath))
		for _, nodeID := range sectionPath {
			if label, exists := labelByID[nodeID]; exists {
				structure.SemanticPath = append(structure.SemanticPath, label)
			}
		}
	}
}

type ingestionStructureResolver struct{}

func (ingestionStructureResolver) Resolve(
	_ context.Context,
	document *schema.Document,
) (*chunking.BlockStructure, error) {
	metadata := document.MetaData
	kind, ok := metadata[ingestion.MetadataStructureKind].(string)
	if !ok || strings.TrimSpace(kind) == "" {
		return nil, fmt.Errorf("metadata %q must be a non-empty string", ingestion.MetadataStructureKind)
	}
	depth, ok := ingestionStructureDepth(metadata[ingestion.MetadataStructureDepth])
	if !ok || depth < 0 {
		return nil, fmt.Errorf("metadata %q must be a non-negative integer", ingestion.MetadataStructureDepth)
	}
	path, ok := ingestionStructurePath(metadata[ingestion.MetadataStructurePath])
	if !ok {
		return nil, fmt.Errorf("metadata %q must be []string without empty items", ingestion.MetadataStructurePath)
	}
	parentID, ok := optionalIngestionString(metadata, ingestion.MetadataStructureParentID)
	if !ok {
		return nil, fmt.Errorf("metadata %q must be a string", ingestion.MetadataStructureParentID)
	}
	boundary, ok := optionalIngestionString(metadata, ingestion.MetadataStructureBoundary)
	if !ok {
		return nil, fmt.Errorf("metadata %q must be a string", ingestion.MetadataStructureBoundary)
	}

	return &chunking.BlockStructure{
		Kind:     chunking.BlockKind(strings.TrimSpace(kind)),
		Depth:    depth,
		ParentID: strings.TrimSpace(parentID),
		Path:     path,
		Boundary: chunking.BlockBoundary(boundary),
	}, nil
}

func ingestionStructureDepth(value any) (int, bool) {
	switch number := value.(type) {
	case int:
		return number, true
	case int8:
		return int(number), true
	case int16:
		return int(number), true
	case int32:
		return int(number), true
	case int64:
		if int64(int(number)) != number {
			return 0, false
		}
		return int(number), true
	case uint:
		if uint(int(number)) != number {
			return 0, false
		}
		return int(number), true
	case uint8:
		return int(number), true
	case uint16:
		return int(number), true
	case uint32:
		if uint32(int(number)) != number {
			return 0, false
		}
		return int(number), true
	case uint64:
		if uint64(int(number)) != number || number > uint64(math.MaxInt) {
			return 0, false
		}
		return int(number), true
	case float64:
		if number != math.Trunc(number) || number < 0 || number > float64(math.MaxInt) {
			return 0, false
		}
		return int(number), true
	default:
		return 0, false
	}
}

func ingestionStructurePath(value any) ([]string, bool) {
	if value == nil {
		return nil, true
	}
	path, ok := value.([]string)
	if !ok {
		return nil, false
	}
	cloned := make([]string, len(path))
	for index, item := range path {
		item = strings.TrimSpace(item)
		if item == "" {
			return nil, false
		}
		cloned[index] = item
	}
	return cloned, true
}

func optionalIngestionString(metadata map[string]any, key string) (string, bool) {
	value, exists := metadata[key]
	if !exists {
		return "", true
	}
	text, ok := value.(string)
	return text, ok
}

var _ chunking.FormatAdapter = (*IngestionAdapter)(nil)
