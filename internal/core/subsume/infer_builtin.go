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

// listFold describes successful results of and/or. An optional tail contributes
// to a union, but cannot narrow a conjunction: it may have no elements at all.
// In particular, and([]) is top, so [...A] alone cannot prove result A.
func (p *inference) listFold(value adt.Value, conjunction bool) adt.Value {
	if !p.step() || value == nil {
		return nil
	}
	switch x := adt.Unwrap(value).(type) {
	case *adt.RigidType:
		return p.listFold(x.Bound, conjunction)
	case *adt.LiveType:
		return p.listFold(x.Upper, conjunction)
	case *adt.Disjunction:
		var results []adt.Value
		for _, branch := range x.Values {
			result := p.listFold(branch, conjunction)
			if result == nil {
				return nil
			}
			results = append(results, result)
		}
		return proofUnion(results)
	}
	list, ok := value.(*adt.Vertex)
	if !ok || !list.IsList() {
		return &adt.Top{}
	}
	var elements []adt.Value
	for field := range list.Elems() {
		element := p.project(list, field.Label)
		if element == nil {
			return nil
		}
		elements = append(elements, adt.Unwrap(element))
	}
	if conjunction {
		var result adt.Value = &adt.Top{}
		for _, element := range elements {
			result = p.sourceMeet(result, element)
			if result == nil {
				return nil
			}
		}
		return result
	}
	if !list.IsClosedList() {
		tail := p.project(list, adt.MakeIntLabel(adt.IntLabel, int64(len(elements))))
		if tail == nil {
			return nil
		}
		elements = append(elements, adt.Unwrap(tail))
	}
	if len(elements) == 0 {
		return &adt.Bottom{Code: adt.EvalError, Err: p.ctx.Newf("empty list in call to or")}
	}
	return proofUnion(elements)
}
