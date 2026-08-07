package chunking

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/cloudwego/eino/components/document"
	"github.com/cloudwego/eino/schema"
)

func TestNilBlankAndMixedInputs(t *testing.T) {
	engine := newTestEngine(t, 32, 12)
	if _, err := engine.Chunk(nil, nil); !errors.Is(err, ErrNilContext) {
		t.Fatalf("Chunk(nil context) error = %v", err)
	}
	if _, err := engine.Chunk(context.Background(), nil); !errors.Is(err, ErrNoValidBlocks) {
		t.Fatalf("Chunk(nil documents) error = %v", err)
	}
	if _, err := engine.Chunk(context.Background(), []*schema.Document{
		nil,
		{ID: "blank", Content: " \n\t "},
	}); !errors.Is(err, ErrNoValidBlocks) {
		t.Fatalf("Chunk(blank documents) error = %v", err)
	}
	result, err := engine.Chunk(context.Background(), []*schema.Document{
		nil,
		{ID: "blank", Content: "  "},
		{ID: "valid", Content: "valid content", MetaData: map[string]any{"kept": true}},
	})
	if err != nil {
		t.Fatalf("Chunk(mixed documents) error = %v", err)
	}
	if result.Statistics.InputDocumentCount != 3 || result.Statistics.BlockCount != 1 {
		t.Fatalf("Statistics = %#v", result.Statistics)
	}
	for _, chunk := range result.Chunks {
		if len(chunk.SourceUnitIDs) != 1 || chunk.SourceUnitIDs[0] != "valid" {
			t.Fatalf("chunk source units = %#v", chunk.SourceUnitIDs)
		}
	}
}

func TestInvalidConfigurationAndMetadataConflict(t *testing.T) {
	strategy, err := NewParentChildStrategy(ParentChildConfig{})
	if err != nil {
		t.Fatalf("NewParentChildStrategy() error = %v", err)
	}
	tests := []EngineConfig{
		{},
		{Profile: Profile{Name: "name", Version: "v1"}, Strategy: strategy},
		{Profile: Profile{Name: "name", Version: "v1"}, Adapter: NewDocumentAdapter()},
		{Profile: Profile{Name: "name"}, Adapter: NewDocumentAdapter(), Strategy: strategy},
	}
	for index, config := range tests {
		if _, err := NewEngine(config); err == nil {
			t.Fatalf("NewEngine(config %d) error = nil", index)
		}
	}
	if _, err := NewBoundedParentBuilder(BoundedParentBuilderConfig{MaxRunes: -1}); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("NewBoundedParentBuilder() error = %v", err)
	}
	if _, err := NewBoundedTextSplitter(BoundedTextSplitterConfig{MaxRunes: -1}); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("NewBoundedTextSplitter() error = %v", err)
	}
	if _, err := NewParentChildStrategy(ParentChildConfig{
		ChildSplitter:    &emptySplitter{},
		ChildTransformer: &identityTransformer{},
	}); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("NewParentChildStrategy(conflict) error = %v", err)
	}

	engine := newTestEngine(t, 32, 12)
	_, err = engine.Chunk(context.Background(), []*schema.Document{{
		ID:       "doc",
		Content:  "content",
		MetaData: map[string]any{MetadataParentID: "caller-value"},
	}})
	if !errors.Is(err, ErrMetadataConflict) {
		t.Fatalf("Chunk(metadata conflict) error = %v", err)
	}
}

func TestDependencyErrorsAreWrapped(t *testing.T) {
	sentinel := errors.New("sentinel")
	document := []*schema.Document{{ID: "doc", Content: "content"}}

	adapterEngine := newEngineForTest(t, &errorAdapter{err: sentinel}, &errorStrategy{}, nil)
	if _, err := adapterEngine.Chunk(context.Background(), document); !errors.Is(err, ErrAdapterFailed) || !errors.Is(err, sentinel) {
		t.Fatalf("adapter error = %v", err)
	}

	strategyEngine := newEngineForTest(t, NewDocumentAdapter(), &errorStrategy{err: sentinel}, nil)
	if _, err := strategyEngine.Chunk(context.Background(), document); !errors.Is(err, ErrStrategyFailed) || !errors.Is(err, sentinel) {
		t.Fatalf("strategy error = %v", err)
	}

	strategy, err := NewParentChildStrategy(ParentChildConfig{ParentBuilder: &errorParentBuilder{err: sentinel}})
	if err != nil {
		t.Fatalf("NewParentChildStrategy() error = %v", err)
	}
	parentEngine := newEngineForTest(t, NewDocumentAdapter(), strategy, nil)
	if _, err := parentEngine.Chunk(context.Background(), document); !errors.Is(err, ErrParentBuilderFailed) || !errors.Is(err, sentinel) {
		t.Fatalf("parent builder error = %v", err)
	}

	strategy, err = NewParentChildStrategy(ParentChildConfig{ChildSplitter: &errorSplitter{err: sentinel}})
	if err != nil {
		t.Fatalf("NewParentChildStrategy() error = %v", err)
	}
	splitterEngine := newEngineForTest(t, NewDocumentAdapter(), strategy, nil)
	if _, err := splitterEngine.Chunk(context.Background(), document); !errors.Is(err, ErrSplitterFailed) || !errors.Is(err, sentinel) {
		t.Fatalf("splitter error = %v", err)
	}

	strategy, err = NewParentChildStrategy(ParentChildConfig{})
	if err != nil {
		t.Fatalf("NewParentChildStrategy() error = %v", err)
	}
	idEngine := newEngineForTest(t, NewDocumentAdapter(), strategy, &errorIDGenerator{err: sentinel})
	if _, err := idEngine.Chunk(context.Background(), document); !errors.Is(err, ErrIDGenerationFailed) || !errors.Is(err, sentinel) {
		t.Fatalf("id generator error = %v", err)
	}
}

func TestDuplicateIDsInvalidRelationsAndNoChildren(t *testing.T) {
	strategy, err := NewParentChildStrategy(ParentChildConfig{})
	if err != nil {
		t.Fatalf("NewParentChildStrategy() error = %v", err)
	}
	duplicateEngine := newEngineForTest(t, NewDocumentAdapter(), strategy, constantIDGenerator("duplicate"))
	if _, err := duplicateEngine.Chunk(context.Background(), []*schema.Document{{ID: "doc", Content: "content"}}); !errors.Is(err, ErrDuplicateID) {
		t.Fatalf("duplicate id error = %v", err)
	}

	invalidEngine := newEngineForTest(t, NewDocumentAdapter(), &invalidRelationStrategy{}, nil)
	if _, err := invalidEngine.Chunk(context.Background(), []*schema.Document{{ID: "doc", Content: "content"}}); !errors.Is(err, ErrInvalidRelation) {
		t.Fatalf("invalid relation error = %v", err)
	}

	strategy, err = NewParentChildStrategy(ParentChildConfig{ChildSplitter: &emptySplitter{}})
	if err != nil {
		t.Fatalf("NewParentChildStrategy() error = %v", err)
	}
	emptyEngine := newEngineForTest(t, NewDocumentAdapter(), strategy, nil)
	if _, err := emptyEngine.Chunk(context.Background(), []*schema.Document{{ID: "doc", Content: "content"}}); !errors.Is(err, ErrNoValidChunks) {
		t.Fatalf("empty splitter error = %v", err)
	}

	duplicateBlockEngine := newEngineForTest(t, &duplicateBlockAdapter{}, &errorStrategy{}, nil)
	if _, err := duplicateBlockEngine.Chunk(context.Background(), []*schema.Document{{ID: "doc", Content: "content"}}); !errors.Is(err, ErrDuplicateID) {
		t.Fatalf("duplicate block error = %v", err)
	}
}

func TestContextCancellationAndTimeout(t *testing.T) {
	engine := newTestEngine(t, 32, 12)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := engine.Chunk(canceled, []*schema.Document{{ID: "doc", Content: "content"}}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled error = %v", err)
	}

	timeoutEngine := newEngineForTest(t, &waitingAdapter{}, &errorStrategy{}, nil)
	ctx, stop := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer stop()
	if _, err := timeoutEngine.Chunk(ctx, []*schema.Document{{ID: "doc", Content: "content"}}); !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, ErrAdapterFailed) {
		t.Fatalf("timeout error = %v", err)
	}
}

func TestConcurrentCallsAreStable(t *testing.T) {
	engine := newTestEngine(t, 40, 10)
	documents := []*schema.Document{{
		ID:       "doc",
		Content:  "alpha beta gamma delta epsilon zeta eta theta",
		MetaData: map[string]any{"source": "concurrent"},
	}}
	want, err := engine.Chunk(context.Background(), documents)
	if err != nil {
		t.Fatalf("Chunk() error = %v", err)
	}

	const calls = 64
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
				errorsChannel <- fmt.Errorf("result is not stable")
			}
		}()
	}
	waitGroup.Wait()
	close(errorsChannel)
	for err := range errorsChannel {
		t.Error(err)
	}
}

func TestCustomStrategyRequiresNoEngineChanges(t *testing.T) {
	engine := newEngineForTest(t, NewDocumentAdapter(), &customStrategy{}, nil)
	result, err := engine.Chunk(context.Background(), []*schema.Document{
		{ID: "one", Content: "first"},
		{ID: "two", Content: "second"},
	})
	if err != nil {
		t.Fatalf("Chunk() error = %v", err)
	}
	if result.StrategyName != "test_strategy" || len(result.Chunks) != 2 {
		t.Fatalf("result = %#v", result)
	}
	for index, chunk := range result.Chunks {
		if chunk.Kind != "test" || chunk.Level != 0 || chunk.Sequence != index+1 {
			t.Fatalf("chunk[%d] = %#v", index, chunk)
		}
	}
}

func TestEngineProtectsInputFromMutatingDependencies(t *testing.T) {
	nested := map[string]any{"value": "original"}
	document := &schema.Document{
		ID:       "doc",
		Content:  "original content",
		MetaData: map[string]any{"nested": nested},
	}
	adapterEngine := newEngineForTest(t, &mutatingAdapter{}, &customStrategy{}, nil)
	if _, err := adapterEngine.Chunk(context.Background(), []*schema.Document{document}); err != nil {
		t.Fatalf("Chunk(mutating adapter) error = %v", err)
	}
	if document.Content != "original content" || nested["value"] != "original" {
		t.Fatalf("adapter mutated caller input: %#v", document)
	}

	strategy, err := NewParentChildStrategy(ParentChildConfig{ChildSplitter: &mutatingSplitter{}})
	if err != nil {
		t.Fatalf("NewParentChildStrategy() error = %v", err)
	}
	splitterEngine := newEngineForTest(t, NewDocumentAdapter(), strategy, nil)
	if _, err := splitterEngine.Chunk(context.Background(), []*schema.Document{document}); err != nil {
		t.Fatalf("Chunk(mutating splitter) error = %v", err)
	}
	if document.Content != "original content" || nested["value"] != "original" {
		t.Fatalf("splitter mutated caller input: %#v", document)
	}
}

func newEngineForTest(t *testing.T, adapter FormatAdapter, strategy Strategy, generator IDGenerator) *Engine {
	t.Helper()
	engine, err := NewEngine(EngineConfig{
		Profile:     Profile{Name: "test", Version: "v1"},
		Adapter:     adapter,
		Strategy:    strategy,
		IDGenerator: generator,
	})
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}
	return engine
}

type errorAdapter struct {
	err error
}

func (*errorAdapter) Name() string { return "error_adapter" }
func (adapter *errorAdapter) Adapt(context.Context, []*schema.Document) ([]Block, error) {
	return nil, adapter.err
}

type waitingAdapter struct{}

func (*waitingAdapter) Name() string { return "waiting_adapter" }
func (*waitingAdapter) Adapt(ctx context.Context, _ []*schema.Document) ([]Block, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

type duplicateBlockAdapter struct{}

func (*duplicateBlockAdapter) Name() string { return "duplicate_block_adapter" }
func (*duplicateBlockAdapter) Adapt(context.Context, []*schema.Document) ([]Block, error) {
	return []Block{
		{ID: "same", DocumentID: "doc", Content: "one", SourceUnitIDs: []string{"same"}},
		{ID: "same", DocumentID: "doc", Content: "two", SourceUnitIDs: []string{"same"}},
	}, nil
}

type mutatingAdapter struct{}

func (*mutatingAdapter) Name() string { return "mutating_adapter" }
func (*mutatingAdapter) Adapt(_ context.Context, documents []*schema.Document) ([]Block, error) {
	documents[0].Content = "mutated"
	documents[0].MetaData["nested"].(map[string]any)["value"] = "mutated"
	return []Block{{
		ID:            documents[0].ID,
		DocumentID:    documents[0].ID,
		Content:       documents[0].Content,
		SourceUnitIDs: []string{documents[0].ID},
		Metadata:      documents[0].MetaData,
	}}, nil
}

type errorStrategy struct {
	err error
}

func (*errorStrategy) Name() string { return "error_strategy" }
func (strategy *errorStrategy) Chunk(context.Context, StrategyInput) (*StrategyOutput, error) {
	return nil, strategy.err
}

type errorParentBuilder struct {
	err error
}

func (builder *errorParentBuilder) Build(context.Context, []Block) ([]ParentDraft, error) {
	return nil, builder.err
}

type errorSplitter struct {
	err error
}

func (splitter *errorSplitter) Split(context.Context, *schema.Document) ([]*schema.Document, error) {
	return nil, splitter.err
}

type emptySplitter struct{}

func (*emptySplitter) Split(context.Context, *schema.Document) ([]*schema.Document, error) {
	return nil, nil
}

type mutatingSplitter struct{}

func (*mutatingSplitter) Split(_ context.Context, parent *schema.Document) ([]*schema.Document, error) {
	parent.MetaData["nested"].(map[string]any)["value"] = "mutated"
	return []*schema.Document{{ID: parent.ID, Content: parent.Content, MetaData: parent.MetaData}}, nil
}

type errorIDGenerator struct {
	err error
}

func (generator *errorIDGenerator) Generate(context.Context, IDInput) (string, error) {
	return "", generator.err
}

type constantIDGenerator string

func (generator constantIDGenerator) Generate(context.Context, IDInput) (string, error) {
	return string(generator), nil
}

type invalidRelationStrategy struct{}

func (*invalidRelationStrategy) Name() string { return "invalid_relation" }
func (*invalidRelationStrategy) Chunk(_ context.Context, input StrategyInput) (*StrategyOutput, error) {
	chunk := Chunk{
		ID:            "chunk",
		Kind:          "test",
		Content:       input.Blocks[0].Content,
		DocumentID:    input.Blocks[0].DocumentID,
		Level:         0,
		NextID:        "missing",
		SourceUnitIDs: input.Blocks[0].SourceUnitIDs,
		Sequence:      1,
		Metadata:      input.Blocks[0].Metadata,
	}
	return &StrategyOutput{Chunks: []Chunk{chunk}, Relations: buildRelations([]Chunk{chunk})}, nil
}

type customStrategy struct{}

func (*customStrategy) Name() string { return "test_strategy" }
func (*customStrategy) Chunk(ctx context.Context, input StrategyInput) (*StrategyOutput, error) {
	chunks := make([]Chunk, 0, len(input.Blocks))
	for _, block := range input.Blocks {
		sequence := len(chunks) + 1
		id, err := input.IDGenerator.Generate(ctx, IDInput{
			Profile:       input.Profile,
			StrategyName:  "test_strategy",
			Kind:          "test",
			Level:         0,
			DocumentID:    block.DocumentID,
			Sequence:      sequence,
			Content:       block.Content,
			SourceUnitIDs: block.SourceUnitIDs,
		})
		if err != nil {
			return nil, err
		}
		chunks = append(chunks, Chunk{
			ID:            id,
			Kind:          "test",
			Content:       block.Content,
			DocumentID:    block.DocumentID,
			Level:         0,
			SourceUnitIDs: append([]string(nil), block.SourceUnitIDs...),
			Sequence:      sequence,
			Metadata:      cloneMetadata(block.Metadata),
		})
	}
	return &StrategyOutput{Chunks: chunks, Relations: buildRelations(chunks)}, nil
}

type identityTransformer struct{}

func (*identityTransformer) Transform(
	_ context.Context,
	documents []*schema.Document,
	_ ...document.TransformerOption,
) ([]*schema.Document, error) {
	return documents, nil
}
