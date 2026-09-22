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

// Universes are cumulative. Base predicates and monomorphic constructors
// preserve their largest component level; a type quantifier raises the level
// above its domain. Unannotated binders can be instantiated at a larger level,
// whereas an explicit Type(n) is checked at every use, including aliases.
// Unknown formation facts remain obligations rather than falling back to 0.
func universeOf(c *OpContext, v Value, seen map[Expr]bool) (int, bool) {
	if v == nil {
		return 0, false
	}
	if seen[v] {
		return 0, true
	}
	seen[v] = true
	defer delete(seen, v)
	level := 0
	add := func(x Value) bool {
		n, ok := universeOf(c, x, seen)
		level = max(level, n)
		return ok
	}
	switch v := v.(type) {
	case *RigidType:
		return v.Param.Level, true
	case *FuncValue:
		known := true
		for _, p := range typeParameters(v.Env) {
			if p.ValueRange == nil {
				level = max(level, p.Level+1)
				known = known && p.ExplicitLevel
			}
		}
		n, ok := typeExpressionLevel(c, v.Env, v.Fn, seen)
		level = max(level, n)
		if !ok {
			return level, false
		}
		for _, t := range v.Types {
			n, ok := typeExpressionLevel(c, t.Env, t.Fn, seen)
			level = max(level, n)
			if !ok {
				return level, false
			}
		}
		return level, known
	case *Universal:
		return typeExpressionLevel(c, v.Env, v.Template, seen)
	case *Existential:
		return typeExpressionLevel(c, v.Env, v.Template, seen)
	case *Vertex:
		v.Finalize(c)
		if x := Unwrap(v); x != v {
			return universeOf(c, x, seen)
		}
		for _, a := range v.Arcs {
			if !a.Label.IsLet() && !add(a) {
				return level, false
			}
		}
		// Open list tails and record patterns need formation checks too.
		for conj := range v.LeafConjuncts() {
			if x, ok := conj.Elem().(Expr); ok {
				n, ok := typeExpressionLevel(c, conj.Env, x, seen)
				level = max(level, n)
				if !ok {
					return level, false
				}
			}
		}
	case *Conjunction:
		for _, x := range v.Values {
			if !add(x) {
				return level, false
			}
		}
	case *Disjunction:
		for _, x := range v.Values {
			if !add(x) {
				return level, false
			}
		}
	case *Bottom:
		return 0, !v.IsIncomplete()
	}
	return level, true
}

func typeExpressionLevel(c *OpContext, env *Environment, x Expr, seen map[Expr]bool) (int, bool) {
	if x == nil || seen[x] {
		return 0, true
	}
	if v, ok := x.(Value); ok {
		return universeOf(c, v, seen)
	}
	seen[x] = true
	defer delete(seen, x)
	level, known := 0, true
	add := func(x Expr) {
		n, ok := typeExpressionLevel(c, env, x, seen)
		level, known = max(level, n), known && ok
	}
	switch x := x.(type) {
	case *TypeReference:
		for e := env; e != nil; e = e.Up {
			if e.types != nil {
				if arg := e.types.arguments[x.Param]; arg != nil {
					return universeOf(c, arg, seen)
				}
			}
		}
		return x.Param.Level, true
	case *Function:
		for _, p := range x.Params {
			add(p.Value)
		}
		add(x.Ret)
	case *Quantified:
		for _, p := range x.Params {
			if p.ValueRange == nil {
				level = max(level, p.Level+1)
				known = known && p.ExplicitLevel
			}
			add(p.Bound)
		}
		add(x.Body)
	case *StructLit:
		for _, d := range x.Decls {
			switch d := d.(type) {
			case *Field:
				add(d.Value)
			case *BulkOptionalField:
				add(d.Value)
			case *Ellipsis:
				add(d.Value)
			default:
				known = false
			}
		}
	case *ListLit:
		for _, e := range x.Elems {
			if rest, ok := e.(*Ellipsis); ok {
				add(rest.Value)
			} else if e, ok := e.(Expr); ok {
				add(e)
			} else {
				known = false
			}
		}
	case *BinaryExpr:
		add(x.X)
		add(x.Y)
	case *DisjunctionExpr:
		for _, d := range x.Values {
			add(d.Val)
		}
	case *BoundExpr:
		add(x.Expr)
	default:
		v, complete := c.Evaluate(env, x)
		if !complete || v == nil {
			return 0, false
		}
		return universeOf(c, v, seen)
	}
	return level, known
}

// A scheme lives strictly above the universe of each of its binders.
// Instantiating one of those binders with that same scheme would require
// n >= n+1. This occurs check also follows schemes nested in data values.
func universeOccurs(c *OpContext, param *TypeParameter, value Value, seen map[Value]bool) bool {
	if value == nil || seen[value] {
		return false
	}
	seen[value] = true
	switch v := Unwrap(value).(type) {
	case *FuncValue:
		for _, p := range typeParameters(v.Env) {
			if p == param {
				return true
			}
		}
		for env := v.Env; env != nil; env = env.Up {
			if env.types != nil {
				for _, arg := range env.types.arguments {
					if universeOccurs(c, param, arg, seen) {
						return true
					}
				}
			}
		}
	case *Vertex:
		v.Finalize(c)
		for _, a := range v.Arcs {
			if universeOccurs(c, param, a, seen) {
				return true
			}
		}
	case *Conjunction:
		for _, x := range v.Values {
			if universeOccurs(c, param, x, seen) {
				return true
			}
		}
	case *Disjunction:
		for _, x := range v.Values {
			if universeOccurs(c, param, x, seen) {
				return true
			}
		}
	}
	return false
}
