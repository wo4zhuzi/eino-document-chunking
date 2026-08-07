package structureaware_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/cloudwego/eino/schema"
	chunking "github.com/wo4zhuzi/eino-document-chunking"
	"github.com/wo4zhuzi/eino-document-chunking/adapter"
	"github.com/wo4zhuzi/eino-document-chunking/einoadapter"
	"github.com/wo4zhuzi/eino-document-chunking/strategy/structureaware"
)

func TestStructureAwareAssemblyRelationsAndStability(t *testing.T) {
	engine := newStructureEngine(t, structureaware.StructureAwareConfig{
		MaxRunes: 80,
		MinRunes: 30,
	}, metadataStructureResolver{})
	documents := structuredDocuments()
	before, err := json.Marshal(documents)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	first, err := engine.Chunk(context.Background(), documents)
	if err != nil {
		t.Fatalf("Chunk() error = %v", err)
	}
	second, err := engine.Chunk(context.Background(), documents)
	if err != nil {
		t.Fatalf("second Chunk() error = %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("results are not stable\nfirst=%#v\nsecond=%#v", first, second)
	}
	after, err := json.Marshal(documents)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if string(before) != string(after) {
		t.Fatalf("input changed\nbefore=%s\nafter=%s", before, after)
	}

	if first.AdapterName != "structured_document" || first.StrategyName != structureaware.StructureAwareStrategyName {
		t.Fatalf("adapter=%q strategy=%q", first.AdapterName, first.StrategyName)
	}
	if first.Statistics.InputDocumentCount != len(documents) || first.Statistics.BlockCount != len(documents) {
		t.Fatalf("statistics = %#v", first.Statistics)
	}
	if len(first.Chunks) != 4 {
		t.Fatalf("len(chunks) = %d, want 4: %#v", len(first.Chunks), first.Chunks)
	}

	for index, chunk := range first.Chunks {
		if chunk.Sequence != index+1 || chunk.Kind != structureaware.ChunkKindStructure || chunk.ParentID != "" {
			t.Fatalf("chunk[%d] = %#v", index, chunk)
		}
		if utf8.RuneCountInString(chunk.Content) > 80 {
			t.Fatalf("chunk[%d] exceeds MaxRunes: %q", index, chunk.Content)
		}
		if chunk.Metadata[chunking.MetadataStrategyName] != structureaware.StructureAwareStrategyName {
			t.Fatalf("chunk[%d] strategy metadata = %#v", index, chunk.Metadata)
		}
	}
	if first.Chunks[0].Content != "Guide\n\nInstall the service before starting it." {
		t.Fatalf("first content = %q", first.Chunks[0].Content)
	}
	if first.Chunks[1].Content != "Guide\n\nRun the service and verify its health endpoint." {
		t.Fatalf("second content = %q", first.Chunks[1].Content)
	}
	if !reflect.DeepEqual(first.Chunks[1].Metadata[structureaware.MetadataStructurePath], []string{"Guide"}) {
		t.Fatalf("second path metadata = %#v", first.Chunks[1].Metadata[structureaware.MetadataStructurePath])
	}
	if !reflect.DeepEqual(
		first.Chunks[0].Metadata[structureaware.MetadataStructureBlockKinds],
		[]string{"heading", "paragraph"},
	) {
		t.Fatalf("first block kinds = %#v", first.Chunks[0].Metadata[structureaware.MetadataStructureBlockKinds])
	}
	if first.Chunks[0].NextID != first.Chunks[1].ID || first.Chunks[1].PreviousID != first.Chunks[0].ID {
		t.Fatalf("guide adjacency is invalid: %#v %#v", first.Chunks[0], first.Chunks[1])
	}
	if first.Chunks[2].NextID != "" || first.Chunks[3].PreviousID != "" {
		t.Fatalf("adjacency crossed documents: %#v %#v", first.Chunks[2], first.Chunks[3])
	}
	assertRelations(t, first)
}

func TestStructureAwareAtomicOversizeSplitter(t *testing.T) {
	document := &schema.Document{
		ID:       "code-1",
		Content:  "0123456789ABCDEFGHIJ",
		MetaData: structureMetadata(chunking.BlockKindCode, 1, "", []string{"Guide"}, chunking.BlockBoundaryNone),
	}

	withoutSplitter := newStructureEngine(t, structureaware.StructureAwareConfig{
		MaxRunes:       10,
		MinRunes:       5,
		HeadingContext: structureaware.HeadingContextMetadataOnly,
	}, metadataStructureResolver{})
	if _, err := withoutSplitter.Chunk(context.Background(), []*schema.Document{document}); !errors.Is(err, chunking.ErrOversizeBlock) {
		t.Fatalf("oversize atomic block error = %v", err)
	}

	withSplitter := newStructureEngine(t, structureaware.StructureAwareConfig{
		MaxRunes:       10,
		MinRunes:       5,
		HeadingContext: structureaware.HeadingContextMetadataOnly,
		OversizeSplitter: structureaware.OversizeSplitterFunc(func(
			_ context.Context,
			block chunking.Block,
			maxRunes int,
		) ([]string, error) {
			if block.ID != "code-1" || maxRunes != 10 {
				t.Fatalf("splitter input block=%#v maxRunes=%d", block, maxRunes)
			}
			return []string{"0123456789", "ABCDEFGHIJ"}, nil
		}),
	}, metadataStructureResolver{})
	result, err := withSplitter.Chunk(context.Background(), []*schema.Document{document})
	if err != nil {
		t.Fatalf("Chunk() error = %v", err)
	}
	if len(result.Chunks) != 2 || result.Chunks[0].Content != "0123456789" || result.Chunks[1].Content != "ABCDEFGHIJ" {
		t.Fatalf("chunks = %#v", result.Chunks)
	}
	if result.Chunks[0].SourceUnitIDs[0] != "code-1" || result.Chunks[1].SourceUnitIDs[0] != "code-1" {
		t.Fatalf("source units = %#v %#v", result.Chunks[0].SourceUnitIDs, result.Chunks[1].SourceUnitIDs)
	}
}

func TestStructureAwareLongTextUsesBoundedSplitting(t *testing.T) {
	engine := newStructureEngine(t, structureaware.StructureAwareConfig{
		MaxRunes:       20,
		MinRunes:       10,
		HeadingContext: structureaware.HeadingContextMetadataOnly,
	}, metadataStructureResolver{})
	document := &schema.Document{
		ID:       "long-text",
		Content:  strings.Repeat("a", 45),
		MetaData: structureMetadata(chunking.BlockKindParagraph, 0, "", nil, chunking.BlockBoundaryNone),
	}
	result, err := engine.Chunk(context.Background(), []*schema.Document{document})
	if err != nil {
		t.Fatalf("Chunk() error = %v", err)
	}
	if len(result.Chunks) != 3 {
		t.Fatalf("len(chunks) = %d, want 3", len(result.Chunks))
	}
	for _, chunk := range result.Chunks {
		if utf8.RuneCountInString(chunk.Content) > 20 || !reflect.DeepEqual(chunk.SourceUnitIDs, []string{"long-text"}) {
			t.Fatalf("chunk = %#v", chunk)
		}
	}
}

func TestStructureAwareErrors(t *testing.T) {
	invalidConfigs := []structureaware.StructureAwareConfig{
		{MaxRunes: -1},
		{MaxRunes: 10, MinRunes: 11},
		{HeadingContext: "unsupported"},
		{OversizeSplitter: structureaware.OversizeSplitterFunc(nil)},
	}
	for index, config := range invalidConfigs {
		if _, err := structureaware.NewStructureAwareStrategy(config); !errors.Is(err, chunking.ErrInvalidConfig) {
			t.Fatalf("config[%d] error = %v", index, err)
		}
	}

	strategy, err := structureaware.NewStructureAwareStrategy(structureaware.StructureAwareConfig{})
	if err != nil {
		t.Fatalf("NewStructureAwareStrategy() error = %v", err)
	}
	plainEngine, err := chunking.NewEngine(chunking.EngineConfig{
		Profile:  chunking.Profile{Name: "plain", Version: "v1"},
		Adapter:  adapter.NewDocumentAdapter(),
		Strategy: strategy,
	})
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}
	if _, err := plainEngine.Chunk(context.Background(), []*schema.Document{{ID: "plain", Content: "content"}}); !errors.Is(err, chunking.ErrStructureRequired) {
		t.Fatalf("missing structure error = %v", err)
	}

	missingParent := newStructureEngine(t, structureaware.StructureAwareConfig{}, metadataStructureResolver{})
	if _, err := missingParent.Chunk(context.Background(), []*schema.Document{{
		ID:       "paragraph",
		Content:  "content",
		MetaData: structureMetadata(chunking.BlockKindParagraph, 1, "missing", []string{"Guide"}, chunking.BlockBoundaryNone),
	}}); !errors.Is(err, chunking.ErrInvalidStructure) {
		t.Fatalf("missing parent error = %v", err)
	}

	cycle := []*schema.Document{
		{ID: "one", Content: "one", MetaData: structureMetadata(chunking.BlockKindParagraph, 1, "two", nil, chunking.BlockBoundaryNone)},
		{ID: "two", Content: "two", MetaData: structureMetadata(chunking.BlockKindParagraph, 1, "one", nil, chunking.BlockBoundaryNone)},
	}
	if _, err := missingParent.Chunk(context.Background(), cycle); !errors.Is(err, chunking.ErrInvalidStructure) {
		t.Fatalf("cycle error = %v", err)
	}

	conflictDocument := &schema.Document{
		ID:      "conflict",
		Content: "content",
		MetaData: map[string]any{
			"structure_kind":                      string(chunking.BlockKindText),
			"structure_depth":                     0,
			structureaware.MetadataStructureDepth: 99,
		},
	}
	if _, err := missingParent.Chunk(context.Background(), []*schema.Document{conflictDocument}); !errors.Is(err, chunking.ErrMetadataConflict) {
		t.Fatalf("metadata conflict error = %v", err)
	}

	sentinel := errors.New("split failed")
	failingSplitter := newStructureEngine(t, structureaware.StructureAwareConfig{
		MaxRunes:       5,
		MinRunes:       2,
		HeadingContext: structureaware.HeadingContextMetadataOnly,
		OversizeSplitter: structureaware.OversizeSplitterFunc(func(context.Context, chunking.Block, int) ([]string, error) {
			return nil, sentinel
		}),
	}, metadataStructureResolver{})
	if _, err := failingSplitter.Chunk(context.Background(), []*schema.Document{{
		ID:       "table",
		Content:  "too large",
		MetaData: structureMetadata(chunking.BlockKindTable, 0, "", nil, chunking.BlockBoundaryNone),
	}}); !errors.Is(err, chunking.ErrOversizeBlock) || !errors.Is(err, sentinel) {
		t.Fatalf("splitter error = %v", err)
	}

	for name, parts := range map[string][]string{
		"empty":    nil,
		"oversize": {"still too large"},
	} {
		t.Run("splitter_"+name, func(t *testing.T) {
			engine := newStructureEngine(t, structureaware.StructureAwareConfig{
				MaxRunes:       5,
				MinRunes:       2,
				HeadingContext: structureaware.HeadingContextMetadataOnly,
				OversizeSplitter: structureaware.OversizeSplitterFunc(func(context.Context, chunking.Block, int) ([]string, error) {
					return parts, nil
				}),
			}, metadataStructureResolver{})
			if _, err := engine.Chunk(context.Background(), []*schema.Document{{
				ID:       "code",
				Content:  "too large",
				MetaData: structureMetadata(chunking.BlockKindCode, 0, "", nil, chunking.BlockBoundaryNone),
			}}); !errors.Is(err, chunking.ErrOversizeBlock) {
				t.Fatalf("splitter output error = %v", err)
			}
		})
	}

	formatAdapter, err := adapter.NewStructuredDocumentAdapter(adapter.StructuredDocumentAdapterConfig{
		Resolver: metadataStructureResolver{},
	})
	if err != nil {
		t.Fatalf("NewStructuredDocumentAdapter() error = %v", err)
	}
	duplicateStrategy, err := structureaware.NewStructureAwareStrategy(structureaware.StructureAwareConfig{
		MaxRunes: 20,
		MinRunes: 10,
	})
	if err != nil {
		t.Fatalf("NewStructureAwareStrategy() error = %v", err)
	}
	duplicateEngine, err := chunking.NewEngine(chunking.EngineConfig{
		Profile:     chunking.Profile{Name: "duplicate", Version: "v1"},
		Adapter:     formatAdapter,
		Strategy:    duplicateStrategy,
		IDGenerator: constantIDGenerator("duplicate"),
	})
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}
	if _, err := duplicateEngine.Chunk(context.Background(), []*schema.Document{
		{ID: "one", Content: "one", MetaData: structureMetadata(chunking.BlockKindHeading, 0, "", []string{"one"}, chunking.BlockBoundaryHard)},
		{ID: "two", Content: "two", MetaData: structureMetadata(chunking.BlockKindHeading, 0, "", []string{"two"}, chunking.BlockBoundaryHard)},
	}); !errors.Is(err, chunking.ErrDuplicateID) {
		t.Fatalf("duplicate id error = %v", err)
	}
}

func TestStructureAwareContextConcurrencyAndEinoProjection(t *testing.T) {
	engine := newStructureEngine(t, structureaware.StructureAwareConfig{
		MaxRunes: 80,
		MinRunes: 30,
	}, metadataStructureResolver{})
	documents := structuredDocuments()

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := engine.Chunk(canceled, documents); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled error = %v", err)
	}
	deadline, stop := context.WithTimeout(context.Background(), time.Nanosecond)
	defer stop()
	time.Sleep(time.Millisecond)
	if _, err := engine.Chunk(deadline, documents); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline error = %v", err)
	}

	want, err := engine.Chunk(context.Background(), documents)
	if err != nil {
		t.Fatalf("Chunk() error = %v", err)
	}
	const calls = 32
	errorsChannel := make(chan error, calls)
	var waitGroup sync.WaitGroup
	for index := 0; index < calls; index++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			got, err := engine.Chunk(context.Background(), documents)
			if err != nil {
				errorsChannel <- err
				return
			}
			if !reflect.DeepEqual(got, want) {
				errorsChannel <- errors.New("concurrent result is not stable")
			}
		}()
	}
	waitGroup.Wait()
	close(errorsChannel)
	for err := range errorsChannel {
		t.Error(err)
	}

	transformer, err := einoadapter.NewEinoTransformer(engine, einoadapter.EinoTransformerConfig{
		Output: einoadapter.TransformerOutputAll,
	})
	if err != nil {
		t.Fatalf("NewEinoTransformer() error = %v", err)
	}
	transformed, err := transformer.Transform(context.Background(), documents)
	if err != nil {
		t.Fatalf("Transform() error = %v", err)
	}
	if len(transformed) != len(want.Chunks) {
		t.Fatalf("len(transformed) = %d, want %d", len(transformed), len(want.Chunks))
	}
	for _, document := range transformed {
		if document.MetaData[chunking.MetadataChunkKind] != string(structureaware.ChunkKindStructure) {
			t.Fatalf("transformed kind = %#v", document.MetaData[chunking.MetadataChunkKind])
		}
	}
}

func newStructureEngine(
	t *testing.T,
	config structureaware.StructureAwareConfig,
	resolver adapter.StructureResolver,
) *chunking.Engine {
	t.Helper()
	formatAdapter, err := adapter.NewStructuredDocumentAdapter(adapter.StructuredDocumentAdapterConfig{
		Resolver: resolver,
	})
	if err != nil {
		t.Fatalf("NewStructuredDocumentAdapter() error = %v", err)
	}
	strategy, err := structureaware.NewStructureAwareStrategy(config)
	if err != nil {
		t.Fatalf("NewStructureAwareStrategy() error = %v", err)
	}
	engine, err := chunking.NewEngine(chunking.EngineConfig{
		Profile:  chunking.Profile{Name: "structure-test", Version: "v1"},
		Adapter:  formatAdapter,
		Strategy: strategy,
	})
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}
	return engine
}

type metadataStructureResolver struct{}

type constantIDGenerator string

func (generator constantIDGenerator) Generate(context.Context, chunking.IDInput) (string, error) {
	return string(generator), nil
}

func (metadataStructureResolver) Resolve(
	_ context.Context,
	document *schema.Document,
) (*chunking.BlockStructure, error) {
	metadata := document.MetaData
	kind, _ := metadata["structure_kind"].(string)
	depth, _ := metadata["structure_depth"].(int)
	parentID, _ := metadata["structure_parent_id"].(string)
	path, _ := metadata["structure_path"].([]string)
	boundary, _ := metadata["structure_boundary"].(string)
	return &chunking.BlockStructure{
		Kind:     chunking.BlockKind(kind),
		Depth:    depth,
		ParentID: parentID,
		Path:     append([]string(nil), path...),
		Boundary: chunking.BlockBoundary(boundary),
	}, nil
}

func structuredDocuments() []*schema.Document {
	return []*schema.Document{
		{
			ID:       "guide-heading",
			Content:  "Guide",
			MetaData: structureMetadata(chunking.BlockKindHeading, 0, "", []string{"Guide"}, chunking.BlockBoundaryHard),
		},
		{
			ID:       "guide-install",
			Content:  "Install the service before starting it.",
			MetaData: structureMetadata(chunking.BlockKindParagraph, 1, "guide-heading", []string{"Guide"}, chunking.BlockBoundaryNone),
		},
		{
			ID:       "guide-run",
			Content:  "Run the service and verify its health endpoint.",
			MetaData: structureMetadata(chunking.BlockKindParagraph, 1, "guide-heading", []string{"Guide"}, chunking.BlockBoundarySoft),
		},
		{
			ID:       "config-heading",
			Content:  "Configuration",
			MetaData: structureMetadata(chunking.BlockKindHeading, 0, "", []string{"Configuration"}, chunking.BlockBoundaryHard),
		},
		{
			ID:       "config-body",
			Content:  "Set the port and storage directory.",
			MetaData: structureMetadata(chunking.BlockKindParagraph, 1, "config-heading", []string{"Configuration"}, chunking.BlockBoundaryNone),
		},
		{
			ID:       "other-heading",
			Content:  "Other",
			MetaData: withDocumentID(structureMetadata(chunking.BlockKindHeading, 0, "", []string{"Other"}, chunking.BlockBoundaryHard), "other"),
		},
	}
}

func structureMetadata(
	kind chunking.BlockKind,
	depth int,
	parentID string,
	path []string,
	boundary chunking.BlockBoundary,
) map[string]any {
	return map[string]any{
		"document_id":         "guide",
		"structure_kind":      string(kind),
		"structure_depth":     depth,
		"structure_parent_id": parentID,
		"structure_path":      append([]string(nil), path...),
		"structure_boundary":  string(boundary),
		"source":              "test",
	}
}

func withDocumentID(metadata map[string]any, documentID string) map[string]any {
	metadata["document_id"] = documentID
	return metadata
}

func assertRelations(t *testing.T, result *chunking.Result) {
	t.Helper()
	want := make(map[string]struct{})
	for _, chunk := range result.Chunks {
		if chunk.NextID != "" {
			want[relationKey(chunking.RelationTypePreviousNext, chunk.ID, chunk.NextID)] = struct{}{}
		}
		for _, sourceUnitID := range chunk.SourceUnitIDs {
			want[relationKey(chunking.RelationTypeSource, chunk.ID, sourceUnitID)] = struct{}{}
		}
	}
	if len(result.Relations) != len(want) {
		t.Fatalf("len(relations) = %d, want %d", len(result.Relations), len(want))
	}
	for _, relation := range result.Relations {
		if _, exists := want[relationKey(relation.Type, relation.FromID, relation.ToID)]; !exists {
			t.Fatalf("unexpected relation = %#v", relation)
		}
	}
}

func relationKey(relationType chunking.RelationType, fromID, toID string) string {
	return strings.Join([]string{string(relationType), fromID, toID}, "\x00")
}
