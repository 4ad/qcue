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

import "slices"

type closureComparison struct{ a, b *FuncValue }

// SameFunctionInstance compares code origins, captures, and partial arguments
// independently of retained contracts. If known is false, their identity
// still depends on unresolved captures or a cyclic comparison.
func SameFunctionInstance(c *OpContext, a, b *FuncValue) (same, known bool) {
	r := closureIdentity(c, a, b)
	return r == proofEstablished, r != proofUnknown
}

// closureIdentity compares operational descriptors, not the functions they
// compute. Unknown captures retain an equality obligation: comparing two
// upper approximations is not evidence that their witnesses are equal.
func closureIdentity(c *OpContext, a, b *FuncValue) proofResult {
	key := closureComparison{a, b}
	if c.checkingClosures[key] || c.checkingClosures[closureComparison{b, a}] {
		// A capture may refer back to a closure whose equality is currently
		// being checked. Retain that cyclic obligation rather than either
		// assuming equality or recursively demanding it without progress.
		return proofUnknown
	}
	if c.checkingClosures == nil {
		c.checkingClosures = make(map[closureComparison]bool)
	}
	c.checkingClosures[key] = true
	defer delete(c.checkingClosures, key)
	result := proofEstablished
	if !sameFunctionCode(a.Fn, b.Fn) {
		return proofRefuted
	}

	compare := func(x Expr, xe *Environment, y Expr, ye *Environment, template bool) {
		if x == y && xe == ye {
			return
		}
		if template && x == y && sameTemplateEnvironment(c, xe, ye) {
			return
		}
		if literal, ok := x.(*StructLit); template && ok && x == y && literal.References != nil {
			// The local x, y, and less fields describe a future comparison.
			// Identity of this saved template depends on its code and free
			// dependencies, not on completed inhabitants of those fields.
			switch templateLiteralIdentity(c, literal, xe, ye) {
			case proofRefuted:
				result = proofRefuted
			case proofUnknown:
				if result != proofRefuted {
					result = proofUnknown
				}
			}
			return
		}
		xv, _ := c.Evaluate(xe, x)
		yv, _ := c.Evaluate(ye, y)
		if cell, ok := xv.(*Vertex); template && ok && cell == yv {
			// A template consumes this schema cell itself, not an unknown
			// inhabitant of its fields. The same live cell identifies the
			// same saved template even before x and y have been supplied.
			return
		}
		comparison := proofUnknown
		if template {
			comparison = templateValueIdentity(c, xv, yv)
		} else {
			comparison = runtimeValueIdentity(c, xv, yv)
		}
		switch comparison {
		case proofUnknown:
			if result != proofRefuted {
				result = proofUnknown
			}
		case proofRefuted:
			result = proofRefuted
		}
	}
	if a.Env != b.Env {
		for _, x := range a.Fn.Captures {
			compare(x, a.Env, x, b.Env, false)
		}
	}
	for i := range a.Fn.Params {
		var x, y funcArg
		if i < len(a.args) {
			x = a.args[i]
		}
		if i < len(b.args) {
			y = b.args[i]
		}
		if (x.expr == nil) != (y.expr == nil) {
			return proofRefuted
		}
		if x.expr != nil {
			compare(x.expr, x.env, y.expr, y.env,
				a.NativeTemplateArgument(c, i) && b.NativeTemplateArgument(c, i))
		}
	}
	return result
}

func templateLiteralIdentity(c *OpContext, literal *StructLit, a, b *Environment) proofResult {
	result := proofEstablished
	for _, ref := range literal.References {
		x, _ := c.Evaluate(a, ref)
		y, _ := c.Evaluate(b, ref)
		switch runtimeValueIdentity(c, x, y) {
		case proofRefuted:
			return proofRefuted
		case proofUnknown:
			result = proofUnknown
		}
	}
	return result
}

// References to factory-local templates retain their original conjuncts.
// Match those code origins and environments rather than requiring the local
// comparison fields to be complete. A different source graph stays unknown;
// this is not an equivalence test for arbitrary CUE schemas.
func templateValueIdentity(c *OpContext, a, b Value) proofResult {
	x, xok := a.(*Vertex)
	y, yok := b.(*Vertex)
	if !xok || !yok || x.Bottom() != nil || y.Bottom() != nil {
		return runtimeValueIdentity(c, a, b)
	}
	xs := slices.Collect(x.LeafConjuncts())
	ys := slices.Collect(y.LeafConjuncts())
	if len(xs) == 0 || len(xs) != len(ys) {
		return proofUnknown
	}
	result := proofEstablished
	for i, left := range xs {
		right := ys[i]
		if left.Expr() != right.Expr() {
			return proofUnknown
		}
		if sameTemplateEnvironment(c, left.Env, right.Env) {
			continue
		}
		literal, ok := left.Expr().(*StructLit)
		if !ok || literal.References == nil {
			return proofUnknown
		}
		switch templateLiteralIdentity(c, literal, left.Env, right.Env) {
		case proofRefuted:
			return proofRefuted
		case proofUnknown:
			result = proofUnknown
		}
	}
	return result
}

// Evaluation can copy an environment without changing any of its live cells.
// Such copies identify the same schema, including its dynamic labels and
// predicate arguments. Equal field descriptions alone are not sufficient.
func sameTemplateEnvironment(c *OpContext, a, b *Environment) bool {
	if !sameTypeEnvironment(c, a, b) {
		return false
	}
	for a != b {
		if a.DynamicLabel != b.DynamicLabel {
			return false
		}
		a, b = a.Up, b.Up
	}
	return true
}

func concreteCapture(c *OpContext, v Value) bool {
	seen := make(map[Value]bool)
	functions := make(map[funcAnchorKey]bool)
	var check func(Value) bool
	check = func(v Value) bool {
		if v == nil {
			return false
		}
		// A recursive closure has a finite descriptor graph. Reaching an
		// already checked closure closes that graph coinductively; a data
		// cycle still requires a concrete supplied value.
		if f, ok := Unwrap(v).(*FuncValue); ok && functions[funcAnchorKey{fn: f.Fn, env: f.Env}] {
			return true
		}
		if seen[v] {
			return false
		}
		seen[v] = true
		defer delete(seen, v)
		if x, ok := v.(*Vertex); ok {
			x.Finalize(c)
			if !IsConcrete(x) || x.Bottom() != nil {
				return false
			}
			if x.IsList() && !x.IsClosedList() {
				// A known prefix is not a complete captured list. Its unknown
				// length also prevents it from identifying a singleton witness.
				return false
			}
			for _, a := range x.Arcs {
				if a.ArcType == ArcRequired {
					return false
				}
				if a.ArcType == ArcMember && !a.Label.IsLet() && !a.Label.IsDef() && !check(a) {
					return false
				}
			}
			if unwrapped := Unwrap(x); unwrapped != x {
				return check(unwrapped)
			}
			return true
		}
		if f, ok := v.(*FuncValue); ok {
			key := funcAnchorKey{fn: f.Fn, env: f.Env}
			if IsFuncType(f) || f.checkIdentities(c) != nil {
				return false
			}
			functions[key] = true
			defer delete(functions, key)

			for _, x := range f.Fn.Captures {
				v, _ := c.Evaluate(f.Env, x)
				if !check(v) {
					return false
				}
			}
			for i, a := range f.args {
				if a.expr != nil {
					if f.NativeTemplateArgument(c, i) {
						// The native consumes this schema itself. Its local
						// comparison fields are filled only on invocation;
						// external operands remain live in that environment.
						continue
					}
					v, _ := c.Evaluate(a.env, a.expr)
					if !check(v) {
						return false
					}
				}
			}
		}
		if _, bottom := v.(*Bottom); bottom {
			return false
		}
		return IsConcrete(v)
	}
	return check(v)
}

func mergeClosureIdentities(c *OpContext, a, b *FuncValue) (*FuncValue, *Bottom) {
	result := closureIdentity(c, a, b)
	if result == proofRefuted {
		return nil, c.NewErrf("conflicting function identities")
	}
	m := *a
	m.explicit = mergeFuncTypes(a.explicit, b.explicit)
	m.Types = mergeFuncTypes(a.Types, b.Types)
	if sameFunctionCode(a.Fn, b.Fn) && a.frontier != nil && b.frontier != nil &&
		(a.Fn != b.Fn || a.Env != b.Env || len(a.callViews) != 0 || len(b.callViews) != 0) {
		m.frontier = mergeFuncTypes(a.frontier, b.frontier)
		m.callViews = mergeCallViews(a, b)
		m.selection, m.projection = nil, nil
		m.nativeBinding = nil
	}
	if a.Fn != b.Fn || a.Env != b.Env {
		// Erased identity does not make distinct signature views
		// interchangeable. In particular an instantiated view must
		// retain the universal clause supplied by its other conjunct.
		t := FuncType{Fn: b.Fn, Env: b.Env}
		if b.IsPartial() {
			// Fn and Env reconstruct an unsaved descriptor. Preserve the
			// actual bound arguments when that reconstruction is insufficient.
			t.inhabitant = b
		}
		m.Types = mergeFuncTypes(m.Types, []FuncType{t})
	}

	m.identities = slices.Clone(a.identities)
	if result == proofUnknown && !slices.Contains(m.identities, b) {
		m.identities = append(m.identities, b)
	}
	for _, p := range b.identities {
		if p != a && !slices.Contains(m.identities, p) {
			m.identities = append(m.identities, p)
		}
	}
	return &m, nil
}

func (x *FuncValue) checkIdentities(c *OpContext) *Bottom {
	for _, peer := range x.identities {
		switch closureIdentity(c, x, peer) {
		case proofRefuted:
			return c.NewErrf("conflicting function identities")
		case proofUnknown:
			return &Bottom{Code: IncompleteError,
				Err: c.Newf("incomplete equality of captured values or partial arguments")}
		}
	}
	return nil
}
