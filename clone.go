package chunking

import (
	"github.com/cloudwego/eino/schema"
	"github.com/wo4zhuzi/eino-document-chunking/internal/metadatautil"
)

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
		MetaData: metadatautil.Clone(document.MetaData),
	}
}

func cloneBlocks(blocks []Block) []Block {
	cloned := make([]Block, len(blocks))
	for i := range blocks {
		cloned[i] = blocks[i]
		cloned[i].SourceUnitIDs = append([]string(nil), blocks[i].SourceUnitIDs...)
		cloned[i].Metadata = metadatautil.Clone(blocks[i].Metadata)
	}
	return cloned
}
