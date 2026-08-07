package adapter

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/cloudwego/eino/schema"
	chunking "github.com/wo4zhuzi/eino-document-chunking"
	"github.com/wo4zhuzi/eino-document-chunking/internal/metadatautil"
)

const (
	documentAdapterName = "document"
	metadataDocumentID  = "document_id"
	metadataSource      = "_source"
)

// DocumentAdapter converts every non-empty Eino Document into one logical Block.
type DocumentAdapter struct{}

// NewDocumentAdapter creates the default format-neutral adapter.
func NewDocumentAdapter() *DocumentAdapter {
	return &DocumentAdapter{}
}

// Name implements FormatAdapter.
func (*DocumentAdapter) Name() string {
	return documentAdapterName
}

// Adapt implements FormatAdapter without modifying the supplied documents.
func (*DocumentAdapter) Adapt(ctx context.Context, documents []*schema.Document) ([]chunking.Block, error) {
	if err := contextError(ctx, "adapt documents"); err != nil {
		return nil, err
	}
	blocks := make([]chunking.Block, 0, len(documents))
	for inputIndex, document := range documents {
		if err := contextError(ctx, "adapt documents"); err != nil {
			return nil, err
		}
		if document == nil || strings.TrimSpace(document.Content) == "" {
			continue
		}
		metadata := metadatautil.Clone(document.MetaData)
		documentID := resolveDocumentID(inputIndex, document, metadata)
		unitID := strings.TrimSpace(document.ID)
		if unitID == "" {
			unitID = stableUnitID(documentID, inputIndex, document.Content)
		}
		blocks = append(blocks, chunking.Block{
			ID:            unitID,
			DocumentID:    documentID,
			Content:       document.Content,
			Sequence:      len(blocks) + 1,
			SourceUnitIDs: []string{unitID},
			Metadata:      metadata,
		})
	}
	return blocks, nil
}

func resolveDocumentID(index int, document *schema.Document, metadata map[string]any) string {
	for _, key := range []string{metadataDocumentID, metadataSource} {
		if value, ok := metadata[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	if id := strings.TrimSpace(document.ID); id != "" {
		return id
	}
	return stableUnitID("document", index, document.Content)
}

func stableUnitID(documentID string, index int, content string) string {
	sum := sha256.Sum256([]byte(
		"unit:" + documentID + ":" + strconv.Itoa(index) + ":" + content,
	))
	return "unit_" + hex.EncodeToString(sum[:])
}

func contextError(ctx context.Context, operation string) error {
	if ctx == nil {
		return chunking.ErrNilContext
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	return nil
}

var _ chunking.FormatAdapter = (*DocumentAdapter)(nil)
