package operation

import (
	"context"
	"io"
)

type Predicate interface {
	Eval(ctx context.Context) bool
}

type Conditional struct {
	condition Predicate
	ifTrue    Operation
	ifFalse   Operation
}

func NewConditional(condition Predicate, ifTrue Operation, ifFalse Operation) Operation {
	return &Conditional{
		condition: condition,
		ifTrue:    ifTrue,
		ifFalse:   ifFalse,
	}
}

func (c *Conditional) Run(ctx context.Context, cmdOutput io.Writer) error {
	if c.condition.Eval(ctx) {
		return c.ifTrue.Run(ctx, cmdOutput)
	}
	return c.ifFalse.Run(ctx, cmdOutput)
}

func (c *Conditional) Description() string {
	// TODO: This needs proper context passed from the call site?
	if c.condition.Eval(context.Background()) {
		return c.ifTrue.Description()
	}
	return c.ifFalse.Description()
}
