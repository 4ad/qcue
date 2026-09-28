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

// LiveReference names a description coordinate in the source graph. Its
// current bounds are proof premises; the reference itself remains available
// to future unification. In particular, an integer bound does not mean that
// every integer belongs to this live description.
type LiveReference struct{ X Expr }

func (x *LiveReference) Source() ast.Node { return x.X.Source() }
func (*LiveReference) node()              {}
func (*LiveReference) expr()              {}
func (*LiveReference) declNode()          {}
func (*LiveReference) elemNode()          {}
func (x *LiveReference) evaluate(c *OpContext, state Flags) Value {
	v, complete := c.Evaluate(c.Env(0), x.X)
	if !complete {
		return v
	}
	switch Unwrap(v).(type) {
	case *FuncValue, *Universal, *BuiltinValidator:
		// These descriptors already retain their predicate structure.
		return v
	}
	if v.Kind()&(StructKind|ListKind) == 0 && concreteCapture(c, v) {
		return v
	}
	// Do not replace the live coordinate by its current approximation.
	return &LiveType{Ref: x, Env: c.Env(0), Upper: v}
}

// LiveType retains the source and lexical rebinding of a description together
// with a proved upper bound. A lower bound is available only from independent
// evidence, such as a completed scalar singleton.
type LiveType struct {
	Ref   *LiveReference
	Env   *Environment
	Upper Value
}

func (x *LiveType) Source() ast.Node         { return x.Ref.Source() }
func (*LiveType) node()                      {}
func (*LiveType) expr()                      {}
func (*LiveType) declNode()                  {}
func (*LiveType) elemNode()                  {}
func (x *LiveType) Kind() Kind               { return x.Upper.Kind() }
func (*LiveType) Concreteness() Concreteness { return Constraint }

// Subsumes proves inclusion into the live predicate. Compatibility with Upper
// alone cannot prove this opposite-direction fact.
func (x *LiveType) Subsumes(c *OpContext, value Value) bool {
	if y, ok := Unwrap(value).(*LiveType); ok && x.SameReference(c, y) {
		return true
	}
	witness, complete := c.Evaluate(x.Env, x.Ref.X)
	if !complete || witness == nil || witness.Kind()&(StructKind|ListKind) != 0 || !IsConcrete(witness) {
		return false
	}
	return Equal(c, witness, value, CheckStructural)
}

// SameReference compares source coordinates, not equal upper approximations.
// Distinct references to the same field may have different syntax nodes.
func (x *LiveType) SameReference(c *OpContext, y *LiveType) bool {
	if x.Ref == y.Ref && x.Env == y.Env {
		return true
	}
	a, aok := x.Ref.X.(Resolver)
	b, bok := y.Ref.X.(Resolver)
	if !aok || !bok {
		return false
	}
	av, _ := c.Lookup(x.Env, a)
	bv, _ := c.Lookup(y.Env, b)
	return av != nil && bv != nil && av.DerefValue() == bv.DerefValue()
}

func (x *LiveType) validate(c *OpContext, value Value) *Bottom {
	// Membership is a call-local constraint, not a coverage proof. Retain
	// the original reference so that new source information still validates
	// this operand, including when the body does not use it.
	m := checkMembership(c, value, scopedPredicate{x.Env, x.Ref.X})
	return m.meet.Bottom()
}
