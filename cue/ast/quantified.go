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

package ast

import "cuelang.org/go/cue/token"

// A TypeParam binds a semantic type. Bound is an optional upper bound,
// resolved before this parameter enters scope.
type TypeParam struct {
	Name  *Ident
	Colon token.Pos
	Bound Expr

	comments
}

func (p *TypeParam) Pos() token.Pos  { return p.Name.Pos() }
func (p *TypeParam) pos() *token.Pos { return p.Name.pos() }
func (p *TypeParam) End() token.Pos {
	if p.Bound != nil {
		return p.Bound.End()
	}
	return p.Name.End()
}

// A Quantifier constrains one subject at every assignment to its type
// parameters. Parameters bind from left to right.
// Quantified fields and function-local generics are represented using this
// same expression, rather than introducing a separate binding discipline.
type Quantifier struct {
	Quantifier token.Pos
	Lparen     token.Pos
	Params     []*TypeParam
	Rparen     token.Pos
	Body       Expr

	// Shorthand records a field declaration written as f(A): Body.
	// It affects source formatting only; the binding semantics are the same
	// as f: forall (A) Body. Outside a field, it prints as an explicit forall.
	Shorthand bool

	comments
	expr
}

func (q *Quantifier) Pos() token.Pos  { return q.Quantifier }
func (q *Quantifier) pos() *token.Pos { return &q.Quantifier }
func (q *Quantifier) End() token.Pos  { return q.Body.End() }

// A ParametricAlias abbreviates a description by capture-avoiding
// substitution. Unlike a quantified field, it does not constrain a subject.
type ParametricAlias struct {
	Name   *Ident
	Lparen token.Pos
	Params []*TypeParam
	Rparen token.Pos
	Equal  token.Pos
	Body   Expr

	comments
	decl
}

func (a *ParametricAlias) Pos() token.Pos  { return a.Name.Pos() }
func (a *ParametricAlias) pos() *token.Pos { return a.Name.pos() }
func (a *ParametricAlias) End() token.Pos  { return a.Body.End() }
