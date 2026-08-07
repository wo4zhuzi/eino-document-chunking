package chunking

const (
	MetadataChunkID        = "eino_chunking.chunk_id"
	MetadataChunkKind      = "eino_chunking.chunk_kind"
	MetadataChunkLevel     = "eino_chunking.chunk_level"
	MetadataDocumentID     = "eino_chunking.document_id"
	MetadataParentID       = "eino_chunking.parent_id"
	MetadataPreviousID     = "eino_chunking.previous_id"
	MetadataNextID         = "eino_chunking.next_id"
	MetadataSourceUnitIDs  = "eino_chunking.source_unit_ids"
	MetadataSequence       = "eino_chunking.sequence"
	MetadataCharacterCount = "eino_chunking.character_count"
	MetadataTokenCount     = "eino_chunking.token_count"
	MetadataProfileName    = "eino_chunking.profile_name"
	MetadataProfileVersion = "eino_chunking.profile_version"
	MetadataStrategyName   = "eino_chunking.strategy_name"
	MetadataAdapterName    = "eino_chunking.adapter_name"
)

var reservedMetadataKeys = [...]string{
	MetadataChunkID,
	MetadataChunkKind,
	MetadataChunkLevel,
	MetadataDocumentID,
	MetadataParentID,
	MetadataPreviousID,
	MetadataNextID,
	MetadataSourceUnitIDs,
	MetadataSequence,
	MetadataCharacterCount,
	MetadataTokenCount,
	MetadataProfileName,
	MetadataProfileVersion,
	MetadataStrategyName,
	MetadataAdapterName,
}
