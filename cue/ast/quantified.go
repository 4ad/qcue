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

// A TypeParam declares a lexical quantifier parameter. Without a Sort it
// ranges over semantic types; Bound, if present, is an upper bound on that
// type. A Sort records an explicit "in" clause, including Type(n).
// Bounds and sorts resolve before the parameter enters scope.
type TypeParam struct {
	Name  *Ident
	In    token.Pos
	Sort  Expr
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
	if p.Sort != nil {
		return p.Sort.End()
	}
	return p.Name.End()
}

// A Quantifier constrains one subject at all (forall) or some (exists)
// assignments to its parameters. Parameters bind from left to right.
// Quantified fields and function-local generics are represented using this
// same expression, rather than introducing a separate binding discipline.
type Quantifier struct {
	Quantifier token.Pos
	Exists     bool
	Lparen     token.Pos
	Params     []*TypeParam
	Rparen     token.Pos
	Body       Expr

	comments
	expr
}

func (q *Quantifier) Pos() token.Pos  { return q.Quantifier }
func (q *Quantifier) pos() *token.Pos { return &q.Quantifier }
func (q *Quantifier) End() token.Pos  { return q.Body.End() }
