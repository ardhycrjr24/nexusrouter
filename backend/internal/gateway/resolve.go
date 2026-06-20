package gateway

import (
	"context"
	"strings"

	"github.com/ardhycrjr24/nexusrouter/backend/internal/connectors"
	"github.com/ardhycrjr24/nexusrouter/backend/internal/dispatch"
	"github.com/ardhycrjr24/nexusrouter/backend/internal/store"
)

// resolveResult carries both the targets and the strategy metadata needed
// by the pipeline to apply round-robin or other rotation strategies.
type resolveResult struct {
	Targets  []dispatch.Target
	PlanOpts dispatch.PlanOptions
}

// ChainSource resolves a named chain for a tenant.
type ChainSource interface {
	ListByTenant(ctx context.Context, tenantID string) ([]store.Chain, error)
}

// AliasSource resolves a model alias to a provider/model target.
type AliasSource interface {
	Get(ctx context.Context, alias string) (store.ModelAlias, error)
}

// resolveTargets turns an inbound model string into an ordered fallback chain.
//
// Four forms are supported, in priority order:
//   - "provider/model"  -> a single explicit target (e.g. "openai/gpt-4o").
//     Slashes beyond the first are kept in the model id so vendor-namespaced
//     ids like "anthropic/claude-3.5" via openrouter still work.
//   - "chain:name"       -> the named routing chain's steps.
//   - bare "name"        -> resolved as a chain named "name" if one exists,
//     then as a model alias. A bare name is never assumed to be a provider
//     model; routing stays explicit and predictable.
//
// When a chain is resolved, the returned resolveResult carries the chain ID
// and strategy so the dispatcher can apply round-robin rotation.
func resolveTargets(ctx context.Context, chains ChainSource, aliases AliasSource, tenantID, model string) (resolveResult, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return resolveResult{}, errBadModel("model is required")
	}

	// chain:<name>
	if name, ok := strings.CutPrefix(model, "chain:"); ok {
		return chainResult(ctx, chains, tenantID, name)
	}

	// provider/model — resolve provider alias (e.g. "mmtp" -> "xiaomi-tokenplan").
	if provider, rest, ok := strings.Cut(model, "/"); ok && provider != "" && rest != "" {
		if spec, ok := connectors.SpecByAlias(provider); ok {
			provider = spec.ID
		}
		return resolveResult{Targets: []dispatch.Target{{Provider: provider, Model: rest}}}, nil
	}

	// bare name -> try a chain first
	res, err := chainResult(ctx, chains, tenantID, model)
	if err == nil {
		return res, nil
	}

	// bare name -> try an alias
	if aliases != nil {
		alias, aerr := aliases.Get(ctx, model)
		if aerr == nil && alias.Target != "" {
			if provider, rest, ok := strings.Cut(alias.Target, "/"); ok && provider != "" && rest != "" {
				return resolveResult{Targets: []dispatch.Target{{Provider: provider, Model: rest}}}, nil
			}
		}
	}

	return resolveResult{}, errBadModel("model must be 'provider/model', a chain name, or an alias: " + model)
}

// chainResult resolves a chain by name and extracts its strategy metadata.
func chainResult(ctx context.Context, chains ChainSource, tenantID, name string) (resolveResult, error) {
	list, err := chains.ListByTenant(ctx, tenantID)
	if err != nil {
		return resolveResult{}, err
	}
	for _, c := range list {
		if c.Name == name {
			targets := dispatch.TargetsFromChain(c)
			if len(targets) == 0 {
				return resolveResult{}, errBadModel("chain has no steps: " + name)
			}
			// Append fallback model as last-resort target when configured.
			if c.FallbackProvider != "" && c.FallbackModel != "" {
				targets = append(targets, dispatch.Target{
					Provider: c.FallbackProvider,
					Model:    c.FallbackModel,
				})
			}
			opts := dispatch.PlanOptions{
				ChainID: c.ID,
			}
			switch normalizeStrategyToken(c.Strategy) {
			case string(dispatch.StrategyRoundRobin), "round_robin", "roundrobin":
				opts.Strategy = dispatch.StrategyRoundRobin
			default:
				opts.Strategy = dispatch.StrategyFallback
			}
			return resolveResult{Targets: targets, PlanOpts: opts}, nil
		}
	}
	return resolveResult{}, errBadModel("no chain named " + name)
}

// badModelError signals an unresolvable model string (a client error).
type badModelError struct{ msg string }

func (e badModelError) Error() string { return e.msg }

func errBadModel(msg string) error { return badModelError{msg: msg} }
