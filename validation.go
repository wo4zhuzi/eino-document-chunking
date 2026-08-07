package chunking

import (
	"fmt"
	"strings"
)

func validateOutput(chunks []Chunk, relations []Relation, blocks []Block) error {
	chunkByID := make(map[string]Chunk, len(chunks))
	sourceUnitIDs := make(map[string]struct{})
	for _, block := range blocks {
		for _, sourceUnitID := range block.SourceUnitIDs {
			sourceUnitIDs[sourceUnitID] = struct{}{}
		}
	}
	for index, chunk := range chunks {
		if strings.TrimSpace(chunk.ID) == "" || strings.TrimSpace(chunk.Content) == "" {
			return fmt.Errorf("%w: chunk at index %d has an empty id or content", ErrNoValidChunks, index)
		}
		if _, exists := chunkByID[chunk.ID]; exists {
			return fmt.Errorf("%w: chunk id %q", ErrDuplicateID, chunk.ID)
		}
		if chunk.DocumentID == "" || chunk.Kind == "" || chunk.Level < 0 {
			return fmt.Errorf("%w: chunk %q has invalid kind, level, or document id", ErrInvalidRelation, chunk.ID)
		}
		if chunk.Sequence != index+1 {
			return fmt.Errorf("%w: chunk %q sequence=%d want=%d", ErrInvalidRelation, chunk.ID, chunk.Sequence, index+1)
		}
		if chunk.TokenCount < 0 || chunk.CharacterCount < 1 {
			return fmt.Errorf("%w: chunk %q has invalid length statistics", ErrInvalidRelation, chunk.ID)
		}
		if len(chunk.SourceUnitIDs) == 0 {
			return fmt.Errorf("%w: chunk %q has no source units", ErrInvalidRelation, chunk.ID)
		}
		for _, sourceUnitID := range chunk.SourceUnitIDs {
			if _, exists := sourceUnitIDs[sourceUnitID]; !exists {
				return fmt.Errorf("%w: chunk %q references unknown source unit %q", ErrInvalidRelation, chunk.ID, sourceUnitID)
			}
		}
		chunkByID[chunk.ID] = chunk
	}

	expectedRelations := make(map[string]struct{})
	for _, chunk := range chunks {
		if chunk.Kind == ChunkKindParent && chunk.ParentID != "" {
			return fmt.Errorf("%w: parent chunk %q cannot have a parent", ErrInvalidRelation, chunk.ID)
		}
		if chunk.Kind == ChunkKindChild && chunk.ParentID == "" {
			return fmt.Errorf("%w: child chunk %q has no parent", ErrInvalidRelation, chunk.ID)
		}
		if chunk.ParentID != "" {
			parent, exists := chunkByID[chunk.ParentID]
			if !exists || parent.Level >= chunk.Level || parent.DocumentID != chunk.DocumentID {
				return fmt.Errorf("%w: chunk %q has invalid parent %q", ErrInvalidRelation, chunk.ID, chunk.ParentID)
			}
			expectedRelations[relationKey(RelationTypeParentChild, parent.ID, chunk.ID)] = struct{}{}
		}
		if err := validateAdjacentChunk(chunk, chunk.PreviousID, false, chunkByID); err != nil {
			return err
		}
		if err := validateAdjacentChunk(chunk, chunk.NextID, true, chunkByID); err != nil {
			return err
		}
		if chunk.NextID != "" {
			expectedRelations[relationKey(RelationTypePreviousNext, chunk.ID, chunk.NextID)] = struct{}{}
		}
		for _, sourceUnitID := range chunk.SourceUnitIDs {
			expectedRelations[relationKey(RelationTypeSource, chunk.ID, sourceUnitID)] = struct{}{}
		}
	}

	seenRelations := make(map[string]struct{}, len(relations))
	for _, relation := range relations {
		key := relationKey(relation.Type, relation.FromID, relation.ToID)
		if _, exists := seenRelations[key]; exists {
			return fmt.Errorf("%w: duplicate relation %s", ErrInvalidRelation, key)
		}
		seenRelations[key] = struct{}{}
		if _, expected := expectedRelations[key]; !expected {
			return fmt.Errorf("%w: unexpected relation %s", ErrInvalidRelation, key)
		}
		if err := validateRelationLevels(relation, chunkByID); err != nil {
			return err
		}
	}
	if len(seenRelations) != len(expectedRelations) {
		return fmt.Errorf("%w: got %d relations, want %d", ErrInvalidRelation, len(seenRelations), len(expectedRelations))
	}
	return nil
}

func validateAdjacentChunk(chunk Chunk, adjacentID string, next bool, chunkByID map[string]Chunk) error {
	if adjacentID == "" {
		return nil
	}
	adjacent, exists := chunkByID[adjacentID]
	if !exists || adjacent.Kind != chunk.Kind || adjacent.Level != chunk.Level ||
		adjacent.DocumentID != chunk.DocumentID || adjacent.ParentID != chunk.ParentID {
		return fmt.Errorf("%w: chunk %q has invalid adjacent chunk %q", ErrInvalidRelation, chunk.ID, adjacentID)
	}
	if next && adjacent.PreviousID != chunk.ID {
		return fmt.Errorf("%w: next chunk %q does not point back to %q", ErrInvalidRelation, adjacent.ID, chunk.ID)
	}
	if !next && adjacent.NextID != chunk.ID {
		return fmt.Errorf("%w: previous chunk %q does not point forward to %q", ErrInvalidRelation, adjacent.ID, chunk.ID)
	}
	return nil
}

func validateRelationLevels(relation Relation, chunkByID map[string]Chunk) error {
	from, exists := chunkByID[relation.FromID]
	if !exists || relation.FromLevel != from.Level {
		return fmt.Errorf("%w: relation from level is invalid for %q", ErrInvalidRelation, relation.FromID)
	}
	if relation.Type == RelationTypeSource {
		if relation.ToLevel != -1 {
			return fmt.Errorf("%w: source relation target level must be -1", ErrInvalidRelation)
		}
		return nil
	}
	to, exists := chunkByID[relation.ToID]
	if !exists || relation.ToLevel != to.Level {
		return fmt.Errorf("%w: relation to level is invalid for %q", ErrInvalidRelation, relation.ToID)
	}
	return nil
}

func relationKey(relationType RelationType, fromID, toID string) string {
	return string(relationType) + "\x00" + fromID + "\x00" + toID
}
