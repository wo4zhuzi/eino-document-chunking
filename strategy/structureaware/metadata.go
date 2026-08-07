package structureaware

import (
	"fmt"

	chunking "github.com/wo4zhuzi/eino-document-chunking"
	"github.com/wo4zhuzi/eino-document-chunking/internal/metadatautil"
)

const (
	// MetadataStructureDepth records the original logical structure depth.
	MetadataStructureDepth = "eino_chunking.structure.depth"
	// MetadataStructurePath records the heading or section path.
	MetadataStructurePath = "eino_chunking.structure.path"
	// MetadataStructureBlockKinds records the logical block kinds in a chunk.
	MetadataStructureBlockKinds = "eino_chunking.structure.block_kinds"
)

var structureMetadataKeys = [...]string{
	MetadataStructureDepth,
	MetadataStructurePath,
	MetadataStructureBlockKinds,
}

func decorateStructureMetadata(
	metadata map[string]any,
	depth int,
	path []string,
	kinds []chunking.BlockKind,
) (map[string]any, error) {
	decorated := metadatautil.Clone(metadata)
	for _, key := range structureMetadataKeys {
		if _, exists := decorated[key]; exists {
			return nil, fmt.Errorf("%w: key %q is reserved by structure-aware strategy", chunking.ErrMetadataConflict, key)
		}
	}
	blockKinds := make([]string, len(kinds))
	for index, kind := range kinds {
		blockKinds[index] = string(kind)
	}
	decorated[MetadataStructureDepth] = depth
	decorated[MetadataStructurePath] = append([]string(nil), path...)
	decorated[MetadataStructureBlockKinds] = blockKinds
	return decorated, nil
}
