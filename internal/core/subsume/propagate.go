// Copyright 2026 CUE Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package subsume

import (
	"cuelang.org/go/internal/core/adt"
	"maps"
	"slices"
)

// Function, call and inclusion goals have different assumption stores. In
// particular, satisfying one call's packet cannot establish a function's
// universally quantified calling capability.
type functionGoal struct{ function *adt.FuncValue }
type applicationGoal struct {
	env    *adt.Environment
	callee adt.Value
	call   *adt.CallExpr
}
type inclusionGoal struct{ upper, lower adt.Value }
type observationGoal struct{ subject *adt.Vertex }

// ConfigurePropagation installs one engine for the evaluator's graph. Each
// goal owns a derivation store, positive certificates and live dependencies.
// Resuming a goal reuses its proved certificates; failure to find a proof
// contributes a residual obligation, never a source contradiction.
func ConfigurePropagation(ctx *adt.OpContext) {
	graph := adt.NewPropagation(1000000)
	ctx.Propagation = graph
	ctx.CheckFunction = func(ctx *adt.OpContext, f *adt.FuncValue) *adt.Bottom {
		if p, ok := ctx.Inference.(*inference); ok {
			return p.validateFunction(ctx, f)
		}
		p := newInference(ctx)
		goal := graph.Require(functionGoal{f}, p.work(func() adt.Evidence {
			return proofEvidence(nil, p.validateFunction(ctx, f))
		}))
		return goalError(ctx, goal, f, "function conformance")
	}
	ctx.ProveInclusion = func(ctx *adt.OpContext, upper, lower adt.Value) bool {
		if p, ok := ctx.Inference.(*inference); ok {
			return p.proveInclusion(ctx, upper, lower)
		}
		p := newInference(ctx)
		goal := graph.Require(inclusionGoal{upper, lower}, p.work(func() adt.Evidence {
			if p.proveInclusion(ctx, upper, lower) {
				return adt.Evidence{State: adt.Established}
			}
			return adt.Evidence{}
		}))
		return goal.State == adt.Established
	}
	ctx.CallEvidence = func(ctx *adt.OpContext, env *adt.Environment, f adt.Value, call *adt.CallExpr) *adt.Goal {
		key := applicationGoal{env, f, call}
		if p, ok := ctx.Inference.(*inference); ok {
			return graph.Require(scopedApplication{p, p.support, key}, p.scopedWork(func() adt.Evidence {
				value, err := p.validateApplication(ctx, env, f, call)
				return proofEvidence(value, err)
			}))
		}
		p := newInference(ctx)
		return graph.Require(key, p.work(func() adt.Evidence {
			value, err := p.validateApplication(ctx, env, f, call)
			return proofEvidence(value, err)
		}))
	}
	ctx.CheckArgument = func(env *adt.Environment, expr adt.Expr) adt.Value {
		if p, ok := ctx.Inference.(*inference); ok {
			return p.bindingDescription(p.boundArgument(env, expr))
		}
		value, _ := ctx.Evaluate(env, expr)
		return value
	}
	ctx.CheckBuiltin = ValidateBuiltin
	ctx.CheckSourceOperations = func(ctx *adt.OpContext, env *adt.Environment, expr adt.Expr) *adt.Bottom {
		if _, ok := ctx.Inference.(*inference); ok {
			// The expression rules emit source-operation obligations in
			// their lexical scopes. Evaluating a derived predicate here
			// does not introduce another source observation of that value.
			return nil
		}
		p := newInference(ctx)
		goal := graph.Require(sourceGoal{env, expr}, p.work(func() adt.Evidence {
			return proofEvidence(nil, p.validateSourceOperations(env, expr))
		}))
		return goalError(ctx, goal, expr, "source operation")
	}
	ctx.CheckInterfaces = func(ctx *adt.OpContext, v *adt.Vertex) *adt.Bottom {
		p := newInference(ctx)
		goal := graph.Require(observationGoal{v}, p.work(func() adt.Evidence {
			graph.Observe(v)
			if err := p.validateInterfaces(v); err != nil {
				return proofEvidence(nil, err)
			}
			return adt.Evidence{State: adt.Quiet}
		}))
		return goalError(ctx, goal, v, "interface observation")
	}
}

type sourceGoal struct {
	env  *adt.Environment
	expr adt.Expr
}

// work retains the finite derivation store with its goal. Search state is
// renewed after suspension, while proved certificates retain their explicit
// hypothesis support. The graph accounts for both dispatch and kernel steps.
func (p *inference) work(step func() adt.Evidence) func(*adt.Propagation) adt.Evidence {
	return func(*adt.Propagation) adt.Evidence {
		p.failure = nil
		p.remaining = 10000
		defer p.enter()()
		result := step()
		if result.State == adt.Pending && p.remaining == 0 {
			result.Wait = adt.WorkLimit
		}
		return result
	}
}

func goalError(ctx *adt.OpContext, goal *adt.Goal, source adt.Node, name string) *adt.Bottom {
	if goal.Err != nil {
		return goal.Err
	}
	if goal.State == adt.Pending {
		return &adt.Bottom{Src: source.Source(), Code: adt.BlockedError,
			Err: ctx.Newf("%s awaits evidence", name)}
	}
	return nil
}

func proofEvidence(value adt.Value, err *adt.Bottom) adt.Evidence {
	if err == nil {
		return adt.Evidence{State: adt.Established, Value: value}
	}
	if err.IsIncomplete() {
		return adt.Evidence{Err: err}
	}
	return adt.Evidence{State: adt.Rejected, Err: err}
}

// assumptionStore is an identity for a lexical proof context, not a set of
// accepted declarations. A child may introduce callback or induction
// hypotheses; the finite derivation must discharge or retain each one used.
type assumptionStore struct{ parent *assumptionStore }

type scopedProof struct {
	owner *inference
	store *assumptionStore
	proofKey
}

type coverageGoal struct {
	owner          *inference
	store          *assumptionStore
	target, source adt.FuncType
}

// scopedWork captures the assumptions of an inference when it is installed.
// Re-enqueuing it from an unrelated reduction must never borrow that
// reduction's callbacks, recursive assumptions, or proof-attempt support.
func (p *inference) scopedWork(step func() adt.Evidence) func(*adt.Propagation) adt.Evidence {
	support, hypotheses := p.support, maps.Clone(p.hypotheses)
	active, implementing, attempts := slices.Clone(p.active), slices.Clone(p.implementing), slices.Clone(p.attempts)
	return func(*adt.Propagation) adt.Evidence {
		oldSupport, oldHypotheses := p.support, p.hypotheses
		oldActive, oldImplementing, oldAttempts := p.active, p.implementing, p.attempts
		p.support, p.hypotheses = support, hypotheses
		p.active, p.implementing, p.attempts = active, implementing, attempts
		defer func() {
			p.support, p.hypotheses = oldSupport, oldHypotheses
			p.active, p.implementing, p.attempts = oldActive, oldImplementing, oldAttempts
		}()
		defer p.enter()()
		result := step()
		if result.State == adt.Pending && p.remaining == 0 {
			result.Wait = adt.WorkLimit
		}
		return result
	}
}

func (p *inference) function(f *adt.FuncValue, target adt.FuncType) bool {
	// Inductive discharge is a checked rule of the kernel. A worklist cycle
	// alone cannot establish it, and remains pending in Propagation.Require.
	if p.recursing(f) {
		return p.recursiveContracts(f, []adt.FuncType{target})
	}
	if p.ctx.Propagation == nil {
		return p.deriveFunction(f, target)
	}
	key := proofKey{f, target}
	goal := p.ctx.Propagation.Require(scopedProof{p, p.support, key}, p.scopedWork(func() adt.Evidence {
		if !p.deriveFunction(f, target) {
			return adt.Evidence{}
		}
		certificate := p.completed[key]
		return adt.Evidence{State: adt.Established, Value: certificate.result, Support: certificate}
	}))
	if goal.State != adt.Established {
		return false
	}
	certificate := goal.Support.(proofCertificate)
	p.completed[key] = certificate
	for _, h := range certificate.required {
		p.useHypothesis(h)
	}
	return true
}

// Coverage is an independent premise of the body goal. Call-local packet
// completion is deliberately absent from this propagator's assumptions.
func (p *inference) coverage(target, source adt.FuncType) bool {
	step := func() adt.Evidence {
		s := &subsumer{ctx: p.ctx, inference: p}
		if s.capabilitySignature(target, source) {
			return adt.Evidence{State: adt.Established}
		}
		return adt.Evidence{}
	}
	if p.ctx.Propagation == nil {
		return step().State == adt.Established
	}
	goal := p.ctx.Propagation.Require(coverageGoal{p, p.support, target, source}, p.scopedWork(step))
	return goal.State == adt.Established
}

type scopedApplication struct {
	owner *inference
	store *assumptionStore
	applicationGoal
}
