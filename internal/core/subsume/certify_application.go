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

// ValidateApplication checks the source packet against the callee's available
// interfaces. Its result describes successful computation, independently of
// whether a linked implementation can execute the call.
func ValidateApplication(ctx *adt.OpContext, env *adt.Environment, f *adt.FuncValue, call *adt.CallExpr) (adt.Value, *adt.Bottom) {
	p := newCertifier(ctx)
	defer p.enter()()
	return p.validateApplication(ctx, env, f, call)
}

func (p *certifier) validateApplication(ctx *adt.OpContext, env *adt.Environment, f *adt.FuncValue, call *adt.CallExpr) (adt.Value, *adt.Bottom) {
	// A bodyless declaration is a conditional import hypothesis. Supplied
	// bodies still pass their independent implementation checks in apply.
	p.assume(f, make(map[adt.Value]bool))
	if result := p.apply(env, f, call); result != nil {
		return result, nil
	}
	if p.failure != nil {
		return nil, p.failure
	}
	return nil, &adt.Bottom{Src: call.Source(), Code: adt.BlockedError,
		Err: ctx.NewPosf(adt.Pos(call), "function application remains unproved")}
}
