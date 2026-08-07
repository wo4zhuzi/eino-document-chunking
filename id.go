package chunking

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"strconv"
)

// SHA256IDGenerator generates stable content-addressed chunk IDs.
type SHA256IDGenerator struct{}

// NewSHA256IDGenerator creates the default stateless ID generator.
func NewSHA256IDGenerator() *SHA256IDGenerator {
	return &SHA256IDGenerator{}
}

// Generate implements IDGenerator.
func (*SHA256IDGenerator) Generate(ctx context.Context, input IDInput) (string, error) {
	if err := contextError(ctx, "generate chunk id"); err != nil {
		return "", err
	}
	hasher := sha256.New()
	writeHashField(hasher, input.Profile.Name)
	writeHashField(hasher, input.Profile.Version)
	writeHashField(hasher, input.StrategyName)
	writeHashField(hasher, string(input.Kind))
	writeHashField(hasher, strconv.Itoa(input.Level))
	writeHashField(hasher, input.DocumentID)
	writeHashField(hasher, input.ParentID)
	writeHashField(hasher, strconv.Itoa(input.Sequence))
	writeHashField(hasher, input.Content)
	for _, sourceUnitID := range input.SourceUnitIDs {
		writeHashField(hasher, sourceUnitID)
	}
	return "chunk_" + hex.EncodeToString(hasher.Sum(nil)), nil
}

func writeHashField(hasher hash.Hash, value string) {
	_, _ = fmt.Fprintf(hasher, "%d:%s|", len(value), value)
}
