package operation

import (
	"context"
	"io"
)

type Sequence []Operation

func NewSequence(operations ...Operation) Sequence {
	return operations
}

func (s Sequence) Run(ctx context.Context, cmdOutput io.Writer) error {
	for _, op := range s {
		if err := Run(ctx, cmdOutput, op); err != nil {
			return err
		}
	}
	return nil
}
