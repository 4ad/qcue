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
	goal := c.invocationEvidence(f, call)
	if b := goal.Diagnostic(c, call); b != nil {
		return b
	}
	description := goal.Value
	// Checking an application may establish result constraints, including
	// a derived empty result. It never supplies the missing implementation.
	pending := &AbstractResult{Src: call.Source()}
	result := c.newInlineVertex(nil, nil,
		MakeRootConjunct(nil, pending), MakeRootConjunct(nil, description))
	result.Finalize(c)
	return result
}
