package operation

import (
	"context"
	"io"

	"github.com/arm/topo/internal/output/term"
)

type Sequence []Operation

func NewSequence(operations ...Operation) Sequence {
	return operations
}

func (s Sequence) Run(ctx context.Context, cmdOutput io.Writer) error {
	for _, op := range s {
		// TODO: should probably check context is not cancelled?
		if cmdOutput != nil {
			err := term.PrintHeader(cmdOutput, op.Description())
			if err != nil {
				return err
			}
		}
		if err := op.Run(ctx, cmdOutput); err != nil {
			return err
		}
	}
	return nil
}
