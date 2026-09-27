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

package subsume

import "cuelang.org/go/internal/core/adt"

// length retains the size guaranteed by a checked argument's field or element
// inventory. An open record can have additional fields by width subtyping;
// hidden fields, definitions, and absent optionals do not increase its minimum.
func (p *certifier) length(value adt.Value) adt.Value {
	if !p.step() || value == nil {
		return nil
	}
	minimum, maximum := int64(0), int64(-1)
	switch x := adt.Unwrap(value).(type) {
	case *adt.Disjunction:
		var alternatives []adt.Value
		for _, branch := range x.Values {
			length := p.length(branch)
			if length == nil {
				return nil
			}
			alternatives = append(alternatives, length)
		}
		return proofUnion(alternatives)
	case *adt.RigidType:
		if x.Bound != nil {
			return p.length(x.Bound)
		}
	case *adt.String:
		minimum, maximum = int64(len(x.Str)), int64(len(x.Str))
	case *adt.Bytes:
		minimum, maximum = int64(len(x.B)), int64(len(x.B))
	case *adt.Vertex:
		if x.IsList() {
			for range x.Elems() {
				minimum++
			}
			if x.IsClosedList() {
				maximum = minimum
			}
		} else if x.Kind() == adt.StructKind {
			optional := int64(0)
			finite := true
			for _, info := range x.Structs {
				for _, decl := range info.StructLit.Decls {
					switch decl.(type) {
					case *adt.Field, *adt.LetField:
					default:
						finite = false
					}
				}
			}
			for _, field := range x.Arcs {
				if !field.Label.IsRegular() {
					continue
				}
				switch field.ArcType {
				case adt.ArcMember, adt.ArcRequired:
					minimum++
				case adt.ArcOptional:
					optional++
				default:
					finite = false
				}
			}
			if finite && x.IsClosedStruct() && (x.PatternConstraints == nil || len(x.PatternConstraints.Pairs) == 0) {
				maximum = minimum + optional
			}
		}
	}
	number := func(n int64) *adt.Num {
		v := &adt.Num{K: adt.IntKind}
		v.X.SetInt64(n)
		return v
	}
	if minimum == maximum {
		return number(minimum)
	}
	result := &adt.Conjunction{Values: []adt.Value{
		&adt.BasicType{K: adt.IntKind},
		&adt.BoundValue{Op: adt.GreaterEqualOp, Value: number(minimum)},
	}}
	if maximum >= 0 {
		result.Values = append(result.Values, &adt.BoundValue{Op: adt.LessEqualOp, Value: number(maximum)})
	}
	return p.schema(nil, result)
}
