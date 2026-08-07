package structureaware

import (
	"context"
	"fmt"

	chunking "github.com/wo4zhuzi/eino-document-chunking"
)

func contextError(ctx context.Context, operation string) error {
	if ctx == nil {
		return chunking.ErrNilContext
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	return nil
}
