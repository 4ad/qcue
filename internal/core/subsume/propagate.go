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

import "cuelang.org/go/internal/core/adt"

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

// ConfigurePropagation connects the proof kernel to the evaluator's retained
// constraint graph. The kernel performs finite derivations; the graph owns
// dependencies, suspension and renewed checking. No result of unsuccessful
// proof search is installed as a source contradiction.
func ConfigurePropagation(ctx *adt.OpContext) {
	graph := adt.NewPropagation(1000000)
	ctx.Propagation = graph
	ctx.CheckFunction = func(ctx *adt.OpContext, f *adt.FuncValue) *adt.Bottom {
		goal := graph.Require(functionGoal{f}, func(*adt.Propagation) adt.Evidence {
			return proofEvidence(nil, ValidateFunction(ctx, f))
		})
		if goal.State == adt.Pending && goal.Err == nil {
			return &adt.Bottom{Src: f.Source(), Code: adt.BlockedError,
				Err: ctx.Newf("function conformance awaits evidence")}
		}
		return goal.Err
	}
	ctx.ProveInclusion = func(ctx *adt.OpContext, upper, lower adt.Value) bool {
		goal := graph.Require(inclusionGoal{upper, lower}, func(*adt.Propagation) adt.Evidence {
			if ProveInclusion(ctx, upper, lower) {
				return adt.Evidence{State: adt.Established}
			}
			return adt.Evidence{}
		})
		return goal.State == adt.Established
	}
	ctx.CheckApplication = func(ctx *adt.OpContext, env *adt.Environment, f adt.Value, call *adt.CallExpr) (adt.Value, *adt.Bottom) {
		goal := graph.Require(applicationGoal{env, f, call}, func(*adt.Propagation) adt.Evidence {
			value, err := ValidateApplication(ctx, env, f, call)
			return proofEvidence(value, err)
		})
		if goal.State == adt.Pending && goal.Err == nil {
			return nil, &adt.Bottom{Src: call.Source(), Code: adt.BlockedError,
				Err: ctx.Newf("function invocation awaits evidence")}
		}
		return goal.Value, goal.Err
	}
	ctx.CheckBuiltin = ValidateBuiltin
	ctx.CheckSourceOperations = ValidateSourceOperations
	ctx.CheckInterfaces = func(ctx *adt.OpContext, v *adt.Vertex) *adt.Bottom {
		goal := graph.Require(observationGoal{v}, func(p *adt.Propagation) adt.Evidence {
			p.Observe(v)
			if err := ValidateInterfaces(ctx, v); err != nil {
				return proofEvidence(nil, err)
			}
			return adt.Evidence{State: adt.Quiet}
		})
		if goal.State == adt.Pending && goal.Err == nil {
			return &adt.Bottom{Src: v.Source(), Code: adt.BlockedError,
				Err: ctx.Newf("interface observation awaits evidence")}
		}
		return goal.Err
	}
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
