package chunking

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/cloudwego/eino/schema"
)

func TestParentChildSingleAndMultipleDocuments(t *testing.T) {
	engine := newTestEngine(t, 18, 7)
	documents := []*schema.Document{
		{
			ID:      "page-1",
			Content: "alpha beta gamma delta epsilon",
			MetaData: map[string]any{
				"_source":     "guide.pdf",
				"page_number": 1,
			},
		},
		{
			ID:      "page-2",
			Content: "zeta eta theta",
			MetaData: map[string]any{
				"_source":     "guide.pdf",
				"page_number": 2,
			},
		},
		{
			ID:      "other-1",
			Content: "standalone document",
			MetaData: map[string]any{
				"_source": "other.md",
			},
		},
	}

	result, err := engine.Chunk(context.Background(), documents)
	if err != nil {
		t.Fatalf("Chunk() error = %v", err)
	}
	if result.Profile != (Profile{Name: "knowledge", Version: "v1"}) {
		t.Fatalf("Profile = %#v", result.Profile)
	}
	if result.AdapterName != documentAdapterName || result.StrategyName != ParentChildStrategyName {
		t.Fatalf("adapter=%q strategy=%q", result.AdapterName, result.StrategyName)
	}
	if result.Statistics.InputDocumentCount != 3 || result.Statistics.BlockCount != 3 {
		t.Fatalf("Statistics = %#v", result.Statistics)
	}
	if result.Statistics.ParentCount < 4 || result.Statistics.ChildCount < result.Statistics.ParentCount {
		t.Fatalf("Statistics = %#v", result.Statistics)
	}

	chunkByID := make(map[string]Chunk, len(result.Chunks))
	parentsByDocument := make(map[string][]Chunk)
	childrenByParent := make(map[string][]Chunk)
	for index, chunk := range result.Chunks {
		if chunk.Sequence != index+1 {
			t.Fatalf("chunk[%d].Sequence = %d", index, chunk.Sequence)
		}
		if chunk.CharacterCount != utf8.RuneCountInString(chunk.Content) {
			t.Fatalf("chunk %q character count = %d", chunk.ID, chunk.CharacterCount)
		}
		if chunk.Metadata[MetadataProfileName] != "knowledge" ||
			chunk.Metadata[MetadataProfileVersion] != "v1" ||
			chunk.Metadata[MetadataStrategyName] != ParentChildStrategyName ||
			chunk.Metadata[MetadataAdapterName] != documentAdapterName {
			t.Fatalf("chunk %q metadata = %#v", chunk.ID, chunk.Metadata)
		}
		chunkByID[chunk.ID] = chunk
		switch chunk.Kind {
		case ChunkKindParent:
			if chunk.Level != 0 || chunk.ParentID != "" {
				t.Fatalf("parent = %#v", chunk)
			}
			parentsByDocument[chunk.DocumentID] = append(parentsByDocument[chunk.DocumentID], chunk)
		case ChunkKindChild:
			if chunk.Level != 1 || chunk.ParentID == "" {
				t.Fatalf("child = %#v", chunk)
			}
			childrenByParent[chunk.ParentID] = append(childrenByParent[chunk.ParentID], chunk)
		default:
			t.Fatalf("unexpected chunk kind %q", chunk.Kind)
		}
	}

	for _, chunk := range result.Chunks {
		if chunk.Kind != ChunkKindChild {
			continue
		}
		parent, exists := chunkByID[chunk.ParentID]
		if !exists || parent.Kind != ChunkKindParent || parent.DocumentID != chunk.DocumentID {
			t.Fatalf("child %q parent = %#v exists=%v", chunk.ID, parent, exists)
		}
		if chunk.Metadata[MetadataParentID] != chunk.ParentID {
			t.Fatalf("child %q parent metadata = %#v", chunk.ID, chunk.Metadata[MetadataParentID])
		}
	}
	for documentID, parents := range parentsByDocument {
		assertAdjacentChain(t, "parents "+documentID, parents)
	}
	for parentID, children := range childrenByParent {
		if _, exists := chunkByID[parentID]; !exists {
			t.Fatalf("children reference missing parent %q", parentID)
		}
		assertAdjacentChain(t, "children "+parentID, children)
	}
	assertRelationsMatchChunks(t, result)
}

func TestDefaultParentBuilderBoundsLargeDocuments(t *testing.T) {
	strategy, err := NewParentChildStrategy(ParentChildConfig{})
	if err != nil {
		t.Fatalf("NewParentChildStrategy() error = %v", err)
	}
	engine, err := NewEngine(EngineConfig{
		Profile:  Profile{Name: "bounded", Version: "v1"},
		Adapter:  NewDocumentAdapter(),
		Strategy: strategy,
	})
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}
	content := strings.Repeat("a", DefaultMaxParentRunes*2+37)
	result, err := engine.Chunk(context.Background(), []*schema.Document{{ID: "large", Content: content}})
	if err != nil {
		t.Fatalf("Chunk() error = %v", err)
	}
	parentCount := 0
	for _, chunk := range result.Chunks {
		if chunk.Kind != ChunkKindParent {
			continue
		}
		parentCount++
		if utf8.RuneCountInString(chunk.Content) > DefaultMaxParentRunes {
			t.Fatalf("parent length = %d", utf8.RuneCountInString(chunk.Content))
		}
	}
	if parentCount != 3 {
		t.Fatalf("parentCount = %d, want 3", parentCount)
	}
}

func TestStableIDsMetadataAndInputRemainUnchanged(t *testing.T) {
	engine := newTestEngine(t, 24, 8)
	nested := map[string]any{"owner": "ingestion"}
	tags := []string{"a", "b"}
	document := &schema.Document{
		ID:      "unit-1",
		Content: "first paragraph second paragraph",
		MetaData: map[string]any{
			"source_uri": "memory://doc",
			"nested":     nested,
			"tags":       tags,
		},
	}
	documents := []*schema.Document{document}
	before, err := json.Marshal(documents)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	first, err := engine.Chunk(context.Background(), documents)
	if err != nil {
		t.Fatalf("first Chunk() error = %v", err)
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
	for _, chunk := range first.Chunks {
		if chunk.Metadata["source_uri"] != "memory://doc" {
			t.Fatalf("chunk metadata lost source_uri: %#v", chunk.Metadata)
		}
	}

	first.Chunks[0].Metadata["nested"].(map[string]any)["owner"] = "changed"
	first.Chunks[0].Metadata["tags"].([]string)[0] = "changed"
	if nested["owner"] != "ingestion" || tags[0] != "a" {
		t.Fatalf("result metadata aliases input metadata: nested=%#v tags=%#v", nested, tags)
	}
}

func newTestEngine(t *testing.T, parentRunes, childRunes int) *Engine {
	t.Helper()
	parentBuilder, err := NewBoundedParentBuilder(BoundedParentBuilderConfig{MaxRunes: parentRunes})
	if err != nil {
		t.Fatalf("NewBoundedParentBuilder() error = %v", err)
	}
	childSplitter, err := NewBoundedTextSplitter(BoundedTextSplitterConfig{MaxRunes: childRunes})
	if err != nil {
		t.Fatalf("NewBoundedTextSplitter() error = %v", err)
	}
	strategy, err := NewParentChildStrategy(ParentChildConfig{
		ParentBuilder: parentBuilder,
		ChildSplitter: childSplitter,
	})
	if err != nil {
		t.Fatalf("NewParentChildStrategy() error = %v", err)
	}
	engine, err := NewEngine(EngineConfig{
		Profile:  Profile{Name: "knowledge", Version: "v1"},
		Adapter:  NewDocumentAdapter(),
		Strategy: strategy,
	})
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}
	return engine
}

func assertAdjacentChain(t *testing.T, name string, chunks []Chunk) {
	t.Helper()
	for index, chunk := range chunks {
		wantPrevious := ""
		wantNext := ""
		if index > 0 {
			wantPrevious = chunks[index-1].ID
		}
		if index+1 < len(chunks) {
			wantNext = chunks[index+1].ID
		}
		if chunk.PreviousID != wantPrevious || chunk.NextID != wantNext {
			t.Fatalf("%s[%d] previous=%q next=%q want previous=%q next=%q", name, index, chunk.PreviousID, chunk.NextID, wantPrevious, wantNext)
		}
	}
}

func assertRelationsMatchChunks(t *testing.T, result *Result) {
	t.Helper()
	want := make(map[string]struct{})
	for _, chunk := range result.Chunks {
		if chunk.ParentID != "" {
			want[relationKey(RelationTypeParentChild, chunk.ParentID, chunk.ID)] = struct{}{}
		}
		if chunk.NextID != "" {
			want[relationKey(RelationTypePreviousNext, chunk.ID, chunk.NextID)] = struct{}{}
		}
		for _, sourceUnitID := range chunk.SourceUnitIDs {
			want[relationKey(RelationTypeSource, chunk.ID, sourceUnitID)] = struct{}{}
		}
	}
	if len(result.Relations) != len(want) {
		t.Fatalf("relations=%d want=%d", len(result.Relations), len(want))
	}
	for _, relation := range result.Relations {
		if _, exists := want[relationKey(relation.Type, relation.FromID, relation.ToID)]; !exists {
			t.Fatalf("unexpected relation %#v", relation)
		}
	}
}
