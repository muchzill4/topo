package operation

import (
	"context"
	"io"

	"github.com/arm/topo/internal/output/term"
)

func Run(ctx context.Context, w io.Writer, op Operation) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if w != nil {
		if err := term.PrintHeader(w, op.Description()); err != nil {
			return err
		}
	}
	return op.Run(ctx, w)
}
