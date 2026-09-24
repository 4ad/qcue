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

package adt

import "cuelang.org/go/cue/ast"

// Universal is a scoped Boolean predicate outside the distributive arrow
// fragment. Retaining it is essential: forall A (F(A) | G(A)) need not be
// equivalent to (forall A F(A)) | (forall A G(A)).
type Universal struct {
	Template *Quantified
	Env      *Environment
}

func (u *Universal) Source() ast.Node         { return u.Template.Source() }
func (*Universal) node()                      {}
func (*Universal) expr()                      {}
func (*Universal) declNode()                  {}
func (*Universal) elemNode()                  {}
func (*Universal) Kind() Kind                 { return TopKind }
func (*Universal) Concreteness() Concreteness { return Constraint }
func (u *Universal) validate(c *OpContext, v Value) *Bottom {
	return &Bottom{Src: u.Source(), Code: IncompleteError,
		Err: c.Newf("universal predicate remains unresolved")}
}

// AbstractResult retains the missing execution derivation of a symbolic
// call. An arrow hypothesis constrains the result, but cannot materialize it.
type AbstractResult struct{ Src ast.Node }

func (r *AbstractResult) Source() ast.Node         { return r.Src }
func (*AbstractResult) node()                      {}
func (*AbstractResult) expr()                      {}
func (*AbstractResult) declNode()                  {}
func (*AbstractResult) elemNode()                  {}
func (*AbstractResult) Kind() Kind                 { return TopKind }
func (*AbstractResult) Concreteness() Concreteness { return Constraint }
func (r *AbstractResult) validate(c *OpContext, v Value) *Bottom {
	return &Bottom{Src: r.Src, Code: IncompleteError,
		Err: c.Newf("function implementation remains unresolved")}
}

func (f *FuncValue) abstractCall(c *OpContext, call *CallExpr, state Flags) Value {
	pending := &AbstractResult{Src: call.Source()}
	if call.Partial {
		return pending
	}
	conjuncts := []Conjunct{MakeRootConjunct(nil, pending)}
	admit := func(t FuncType) *packetAdmission {
		// Binding determines protocol slots only. Admission must examine
		// the original packet, without executing a synthetic function or
		// conjoining parameter predicates into an activation.
		view := &FuncValue{Fn: t.Fn, Env: t.Env}
		saved := c.PushState(c.Env(0), call.Source())
		args, unused, bindErr := view.bindCall(c, call)
		var admitted *packetAdmission
		if unused == nil && bindErr == nil {
			admitted, _ = (callPacket{args: args}).admit(c, t, false)
		}
		if err := c.PopState(saved); err != nil {
			return nil
		}
		return admitted
	}
	propagate := func(admitted *packetAdmission) {
		if t := admitted.clause; t.Fn.Ret != nil {
			conjuncts = append(conjuncts, MakeRootConjunct(t.Env, t.Fn.Ret))
		}
	}
	covered := false
	seen := make(map[FuncType]bool)
	for _, clause := range f.CallClauses(c) {
		seen[clause] = true
		if admitted := admit(clause); admitted != nil {
			covered = true
			propagate(admitted)
		}
	}
	if covered {
		for _, clause := range f.ResultClauses(c) {
			if !seen[clause] {
				if admitted := admit(clause); admitted != nil {
					propagate(admitted)
				}
			}
		}
	}
	result := c.newInlineVertex(nil, nil, conjuncts...)
	result.Finalize(c)
	return result
}
