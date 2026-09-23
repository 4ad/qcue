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

// WitnessReference decodes an ordinary value reference in a type position as
// the singleton of that witness, preserving its lexical dependency. Unlike
// TypeReference, its current upper approximation is not the predicate itself.
type WitnessReference struct{ X Expr }

func (x *WitnessReference) Source() ast.Node { return x.X.Source() }
func (*WitnessReference) node()              {}
func (*WitnessReference) expr()              {}
func (*WitnessReference) declNode()          {}
func (*WitnessReference) elemNode()          {}
func (x *WitnessReference) evaluate(c *OpContext, state Flags) Value {
	v, complete := c.Evaluate(c.Env(0), x.X)
	if !complete {
		return v
	}
	switch Unwrap(v).(type) {
	case *FuncValue, *OpaqueType, *Existential, *Universal, *BuiltinValidator:
		// These descriptors already encode predicates. In particular an
		// opened representation type is not an ordinary runtime witness.
		return v
	}
	if v.Kind()&(StructKind|ListKind) == 0 && concreteCapture(c, v) {
		return v
	}
	// A concrete record is still an open structural predicate, and lists
	// may contain such records. Preserve the equality obligation even when
	// the current witness is fully known.
	return &WitnessType{Ref: x, Env: c.Env(0), Upper: v}
}

// WitnessType is an exact pending singleton, not its Upper approximation.
type WitnessType struct {
	Ref   *WitnessReference
	Env   *Environment
	Upper Value
}

func (x *WitnessType) Source() ast.Node         { return x.Ref.Source() }
func (*WitnessType) node()                      {}
func (*WitnessType) expr()                      {}
func (*WitnessType) declNode()                  {}
func (*WitnessType) elemNode()                  {}
func (x *WitnessType) Kind() Kind               { return x.Upper.Kind() }
func (*WitnessType) Concreteness() Concreteness { return Constraint }
func (x *WitnessType) validate(c *OpContext, value Value) *Bottom {
	witness, complete := c.Evaluate(x.Env, x.Ref.X)
	if !complete || !concreteCapture(c, witness) {
		return &Bottom{Src: x.Source(), Code: IncompleteError,
			Err: c.Newf("singleton witness in function signature remains unresolved")}
	}
	if !concreteCapture(c, value) {
		return &Bottom{Src: x.Source(), Code: IncompleteError, Err: c.Newf("singleton membership remains unresolved")}
	}
	if Equal(c, witness, value, 0) {
		return nil
	}
	return c.NewErrf("value conflicts with the singleton witness in its signature")
}
