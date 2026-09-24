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

// runtimeValueIdentity compares concrete inhabitants, independently of their
// retained predicates. It must never be used to deduplicate constraint graphs:
// equal inhabitants can carry different obligations. Unlike Equal, it also
// distinguishes an unresolved comparison from evidence of inequality.
func runtimeValueIdentity(c *OpContext, a, b Value) proofResult {
	return compareRuntimeValues(c, a, b, false)
}

// runtimeEquality implements the language's data observation: hidden fields
// and definitions are not compared. It shares the recursive inhabitant
// comparison with closure captures and opaque values, so nesting cannot turn
// a runtime observation into constraint-graph equality.
func runtimeEquality(c *OpContext, a, b Value, op Op) Value {
	r := compareRuntimeValues(c, a, b, true)
	if r == proofUnknown {
		return &Bottom{Code: IncompleteError, Err: c.Newf("runtime equality remains unresolved")}
	}
	return c.NewBool((r == proofEstablished) == (op == EqualOp))
}

func compareRuntimeValues(c *OpContext, a, b Value, regularOnly bool) proofResult {
	active := make(map[[2]Value]bool)
	var compare func(Value, Value, bool) proofResult
	compare = func(a, b Value, regularOnly bool) proofResult {
		if a == nil || b == nil {
			return proofUnknown
		}
		for _, v := range []Value{a, b} {
			if x, ok := v.(*Vertex); ok {
				x.Finalize(c)
			}
			if bottom(v) != nil {
				return proofUnknown
			}
		}
		a, b = Default(a), Default(b)
		if !IsConcrete(a) || !IsConcrete(b) {
			return proofUnknown
		}
		key := [2]Value{a, b}
		if active[key] {
			return proofUnknown
		}
		active[key] = true
		defer delete(active, key)
		if x, ok := a.(*Vertex); ok {
			y, ok := b.(*Vertex)
			if !ok {
				if value := Unwrap(x); value != x {
					return compare(value, b, regularOnly)
				}
				return proofRefuted
			}
			x, y = x.DerefValue(), y.DerefValue()
			if x.sealed != y.sealed {
				return proofRefuted
			}
			if x.Kind()&(StructKind|ListKind) != 0 {
				if x.Kind() != y.Kind() {
					return proofRefuted
				}
				result := proofEstablished
				observed := func(arc *Vertex) bool {
					// Definitions and absent optionals constrain inhabitants;
					// they are not components of a runtime descriptor.
					return arc.ArcType == ArcMember && !arc.Label.IsLet() &&
						!arc.Label.IsDef() && (!regularOnly || arc.Label.IsRegular())
				}
				for _, arc := range x.Arcs {
					if !observed(arc) {
						continue
					}
					other := y.LookupRaw(arc.Label)
					if other == nil || other.ArcType != ArcMember {
						return proofRefuted
					}
					switch compare(arc, other, regularOnly) {
					case proofRefuted:
						return proofRefuted
					case proofUnknown:
						result = proofUnknown
					}
				}
				for _, arc := range y.Arcs {
					if observed(arc) {
						other := x.LookupRaw(arc.Label)
						if other == nil || other.ArcType != ArcMember {
							return proofRefuted
						}
					}
				}
				return result
			}
		}
		if y, ok := b.(*Vertex); ok {
			if value := Unwrap(y); value != y {
				return compare(a, value, regularOnly)
			}
		}
		a, b = Unwrap(a), Unwrap(b)
		switch x := a.(type) {
		case *FuncValue:
			if y, ok := b.(*FuncValue); ok {
				if !concreteCapture(c, x) || !concreteCapture(c, y) {
					return proofUnknown
				}
				return closureIdentity(c, x, y)
			}
		case *Builtin:
			if y, ok := b.(*Builtin); ok && x.self() == y.self() {
				return proofEstablished
			}
		case *OpaqueValue:
			if y, ok := b.(*OpaqueValue); ok && x.carrier == y.carrier {
				return compare(x.private, y.private, false)
			}
		default:
			if Equal(c, a, b, 0) {
				return proofEstablished
			}
		}
		return proofRefuted
	}
	return compare(a, b, regularOnly)
}
