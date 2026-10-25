package plugin

import (
	"context"
	"fmt"
)

const FactsContract = "facts.v1"

// FactsProviderPlugin reports observations using the target runner supplied by Topo.
type FactsProviderPlugin interface {
	Plugin
	Facts(context.Context, TargetRunner) (FactsResult, error)
}

type FactsResult struct {
	Groups []FactGroup
}

// Group IDs are local to a provider result, not globally unique device IDs.
type FactGroup struct {
	ID    string
	Facts []Fact
}

// A key's contract fixes its meaning, value type, unit and observation scope.
// Every fact has a value. Inconclusive observations are omitted; absence never
// implicitly means false. A change in a key's semantics requires a new key.
type Fact struct {
	Key         string
	Value       Value
	Label       string
	Description string
}

// Value is a tagged scalar on the Go RPC wire. Concrete scalar fields preserve
// false, zero and empty strings across gob encoding without registrations.
// Only the field selected by Type may have a nonzero value.
type Value struct {
	Type    string
	Boolean bool
	Integer int64
	Text    string
}

func (v Value) Validate() error {
	switch v.Type {
	case "boolean":
		if v.Integer == 0 && v.Text == "" {
			return nil
		}
	case "integer":
		if !v.Boolean && v.Text == "" {
			return nil
		}
	case "string":
		if !v.Boolean && v.Integer == 0 {
			return nil
		}
	}
	return fmt.Errorf("invalid fact value type or fields: %q", v.Type)
}

func (r FactsResult) Validate() error {
	groups := make(map[string]bool, len(r.Groups))
	for _, group := range r.Groups {
		if group.ID == "" || groups[group.ID] {
			return fmt.Errorf("empty or duplicate fact group: %q", group.ID)
		}
		groups[group.ID] = true
		keys := make(map[string]bool, len(group.Facts))
		for _, fact := range group.Facts {
			if fact.Key == "" || keys[fact.Key] {
				return fmt.Errorf("invalid or duplicate fact: %q in group %q", fact.Key, group.ID)
			}
			keys[fact.Key] = true
			if err := fact.Value.Validate(); err != nil {
				return fmt.Errorf("fact %q in group %q: %w", fact.Key, group.ID, err)
			}
		}
	}
	return nil
}
