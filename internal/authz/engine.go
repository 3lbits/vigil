package authz

import (
	"context"
	"errors"
	"fmt"
	"maps"

	"github.com/open-policy-agent/opa/v1/rego"
)

var errUnexpectedResult = errors.New("authz: unexpected result type")

// Engine wraps a compiled OPA query. Thread-safe; compile once at startup.
type Engine struct {
	query rego.PreparedEvalQuery
}

// New compiles the given Rego policy source and returns a ready Engine.
// policySource should be the full content of the authz.rego file.
func New(ctx context.Context, policySource string) (*Engine, error) {
	q, err := rego.New(
		rego.Query("data.authz.allow"),
		rego.Module("authz.rego", policySource),
	).PrepareForEval(ctx)
	if err != nil {
		return nil, fmt.Errorf("authz compile: %w", err)
	}
	return &Engine{query: q}, nil
}

// Allow evaluates the policy for the given user, resource, and action.
// An optional extra map is merged into the OPA input (e.g. {"is_participant": true}).
func (e *Engine) Allow(ctx context.Context, userID, role, resource, action string, extra ...map[string]any) (bool, error) {
	input := map[string]any{
		"user":     map[string]any{"id": userID, "role": role},
		"resource": resource,
		"action":   action,
	}
	if len(extra) > 0 {
		maps.Copy(input, extra[0])
	}
	rs, err := e.query.Eval(ctx, rego.EvalInput(input))
	if err != nil {
		return false, fmt.Errorf("authz eval: %w", err)
	}
	if len(rs) == 0 || len(rs[0].Expressions) == 0 {
		return false, nil // undefined = deny
	}
	allowed, ok := rs[0].Expressions[0].Value.(bool)
	if !ok {
		return false, fmt.Errorf("%w: %T", errUnexpectedResult, rs[0].Expressions[0].Value)
	}
	return allowed, nil
}
