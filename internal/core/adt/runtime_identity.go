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
	if !concreteCapture(c, a) || !concreteCapture(c, b) {
		return proofUnknown
	}
	var compare func(Value, Value) proofResult
	compare = func(a, b Value) proofResult {
		a, b = Default(a), Default(b)
		if a == b {
			return proofEstablished
		}
		if x, ok := a.(*Vertex); ok {
			y, ok := b.(*Vertex)
			if !ok {
				if value := Unwrap(x); value != x {
					return compare(value, b)
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
				for _, arc := range x.Arcs {
					if arc.ArcType != ArcMember || arc.Label.IsLet() {
						continue
					}
					other := y.LookupRaw(arc.Label)
					if other == nil || other.ArcType != ArcMember {
						return proofRefuted
					}
					switch compare(arc, other) {
					case proofRefuted:
						return proofRefuted
					case proofUnknown:
						result = proofUnknown
					}
				}
				for _, arc := range y.Arcs {
					if arc.ArcType == ArcMember && !arc.Label.IsLet() {
						other := x.LookupRaw(arc.Label)
						if other == nil || other.ArcType != ArcMember {
							return proofRefuted
						}
					}
				}
				return result
			}
		}
		a, b = Unwrap(a), Unwrap(b)
		switch x := a.(type) {
		case *FuncValue:
			if y, ok := b.(*FuncValue); ok {
				return closureIdentity(c, x, y)
			}
		case *Builtin:
			if y, ok := b.(*Builtin); ok && x.self() == y.self() {
				return proofEstablished
			}
		case *OpaqueValue:
			if y, ok := b.(*OpaqueValue); ok && x.carrier == y.carrier {
				return compare(x.private, y.private)
			}
		default:
			if Equal(c, a, b, 0) {
				return proofEstablished
			}
		}
		return proofRefuted
	}
	return compare(a, b)
}
