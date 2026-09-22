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
	clauses := append([]FuncType{{Fn: f.Fn, Env: f.Env}}, f.Types...)
	for i, t := range clauses {
		// Check packet membership in a fresh activation without asserting
		// that any implementation has run. Failure to find an admitted
		// packet merely leaves this abstract call without a useful clause.
		fn := *t.Fn
		fn.Body, fn.Ret = &Top{}, nil
		probe := &FuncValue{Fn: &fn, Env: t.Env}
		saved := c.PushState(c.Env(0), call.Source())
		result := probe.call(c, call, state)
		err := c.PopState(saved)
		if result == nil || err != nil {
			continue
		}
		if b, ok := Unwrap(result).(*Bottom); ok && b != nil {
			continue
		}
		actual := *t.Fn
		actual.Body = pending
		view := &FuncValue{Src: t.Fn.Src, Fn: &actual, Env: t.Env}
		view.Types = append(view.Types, clauses[:i]...)
		view.Types = append(view.Types, clauses[i+1:]...)
		return view.call(c, call, state)
	}
	return pending
}
