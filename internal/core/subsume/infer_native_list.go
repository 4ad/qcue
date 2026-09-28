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

import (
	"slices"

	"cuelang.org/go/internal/core/adt"
)

func isListShapeBuiltin(c *adt.OpContext, b *adt.Builtin) bool {
	if b.Package == adt.InvalidLabel || b.Package.StringValue(c) != "list" {
		return false
	}
	switch b.Name {
	case "Reverse", "Drop", "Take", "Slice", "Repeat", "Concat", "FlattenN", "Sort", "SortStable", "SortStrings":
		return true
	}
	return false
}

// nativeListResult refines the result of an already admitted native packet.
// It reads predicates, never executing an arbitrary record or list schema as
// if it were a concrete value. Missing evidence leaves the primary contract.
func (p *inference) nativeListResult(b *adt.Builtin, packet adt.FuncType, protocol *adt.Function) adt.Value {
	if !isListShapeBuiltin(p.ctx, b) {
		return nil
	}
	args := make([]adt.Value, len(protocol.Params))
	for i, j := range adt.MatchFuncValueParams(packet.Fn, &adt.FuncValue{Fn: protocol}) {
		if j >= 0 {
			args[j], _ = packet.Fn.Params[i].Value.(adt.Value)
		}
	}
	for _, arg := range args {
		if arg == nil {
			return nil
		}
	}
	return p.nativeListAlternatives(b.Name, args, 0)
}

func (p *inference) nativeListAlternatives(name string, args []adt.Value, start int) adt.Value {
	if !p.step() {
		return nil
	}
	for i := start; i < len(args); i++ {
		if union, ok := adt.Unwrap(args[i]).(*adt.Disjunction); ok {
			var results []adt.Value
			for _, branch := range union.Values {
				packet := slices.Clone(args)
				packet[i] = branch
				result := p.nativeListAlternatives(name, packet, i+1)
				if result == nil {
					return nil
				}
				results = append(results, result)
			}
			return proofUnion(results)
		}
	}
	if name == "FlattenN" {
		depth, known := nativeListIndex(args[1])
		if known {
			return p.flattenList(args[0], &depth)
		}
		return p.flattenList(args[0], nil)
	}
	shape, ok := p.nativeListShape(args[0])
	if !ok {
		return nil
	}
	switch name {
	case "Reverse", "Sort", "SortStable", "SortStrings":
		if name == "Reverse" && shape.tail == nil {
			slices.Reverse(shape.prefix)
		} else {
			element := shape.elements()
			for i := range shape.prefix {
				shape.prefix[i] = element
			}
			if shape.tail != nil {
				shape.tail = element
			}
		}
		return p.nativeListValue(shape)
	case "Take", "Drop":
		n, known := nativeListIndex(args[1])
		if !known {
			if shape.tail != nil {
				return nil
			}
			if int64(len(shape.prefix))*int64(len(shape.prefix)+1)/2 > int64(p.remaining/2) {
				return nil
			}
			var results []adt.Value
			for i := 0; i <= len(shape.prefix); i++ {
				if !p.step() {
					return nil
				}
				part := nativeListShape{prefix: shape.prefix[:i]}
				if name == "Drop" {
					part.prefix = shape.prefix[i:]
				}
				result := p.nativeListValue(part)
				if result == nil {
					return nil
				}
				results = append(results, result)
			}
			return proofUnion(results)
		}
		if n < 0 {
			return p.nativeListFailure()
		}
		if name == "Drop" {
			if n == 0 {
				return args[0]
			}
			shape.prefix = shape.prefix[min(n, int64(len(shape.prefix))):]
			return p.nativeListValue(shape)
		}
		if n <= int64(len(shape.prefix)) || shape.tail == nil {
			return p.nativeListValue(nativeListShape{prefix: shape.prefix[:min(n, int64(len(shape.prefix)))]})
		}
		// Take clamps to the actual length: an open input may stop at any
		// length between its guaranteed prefix and the requested count.
		// Optional refinements must not consume the whole proof allowance
		// enumerating a large family already covered by the generic type.
		if n > int64(p.remaining/2) || n*(n+1)/2 > int64(p.remaining/2) {
			return nil
		}
		var results []adt.Value
		for i := int64(len(shape.prefix)); i <= n; i++ {
			if !p.step() {
				return nil
			}
			result := p.nativeListValue(nativeListShape{prefix: shape.prefix})
			if result == nil {
				return nil
			}
			results = append(results, result)
			shape.prefix = append(shape.prefix, shape.tail)
		}
		return proofUnion(results)
	case "Slice":
		return p.nativeListSlice(args[0], shape, args[1], args[2])
	case "Repeat":
		n, known := nativeListIndex(args[1])
		if shape.tail == nil && len(shape.prefix) == 0 {
			return args[0]
		}
		if !known {
			return nil
		}
		if n < 0 || n > adt.MaxRepeatCount {
			return p.nativeListFailure()
		}
		if n == 1 {
			return args[0]
		}
		if n > int64(p.remaining/2)/int64(max(1, len(shape.prefix))) {
			return nil
		}
		var out nativeListShape
		for range n {
			if !p.step() {
				return nil
			}
			out = joinNativeListShapes(out, shape)
		}
		return p.nativeListValue(out)
	case "Concat":
		if shape.tail == nil && len(shape.prefix) == 1 {
			return shape.prefix[0]
		}
		var out nativeListShape
		for _, element := range shape.prefix {
			part, ok := p.nativeListShape(element)
			if !ok {
				return nil
			}
			out = joinNativeListShapes(out, part)
		}
		if shape.tail != nil {
			part, ok := p.nativeListShape(shape.tail)
			if !ok {
				return nil
			}
			out = joinNativeListShapes(out, nativeListShape{tail: part.elements()})
		}
		return p.nativeListValue(out)
	}
	return nil
}

func (p *inference) nativeListSlice(value adt.Value, shape nativeListShape, low, high adt.Value) adt.Value {
	lo, loKnown := nativeListIndex(low)
	hi, hiKnown := nativeListIndex(high)
	if loKnown && lo < 0 || hiKnown && hi < 0 || loKnown && hiKnown && lo > hi {
		return p.nativeListFailure()
	}
	if loKnown && hiKnown {
		if lo == 0 && shape.tail == nil && hi == int64(len(shape.prefix)) {
			return value
		}
		return p.sliceValue(value, low, high)
	}
	if shape.tail != nil && !hiKnown {
		if loKnown {
			shape.prefix = shape.prefix[min(lo, int64(len(shape.prefix))):]
		}
		return p.nativeListValue(nativeListShape{tail: shape.elements()})
	}
	limit := int64(len(shape.prefix))
	if shape.tail != nil {
		limit = hi
	} else if loKnown && lo > limit || hiKnown && hi > limit {
		return p.nativeListFailure()
	}
	if limit > int64(p.remaining/2) || limit*limit*limit > int64(p.remaining/2) {
		return nil
	}
	start, end := int64(0), limit
	if loKnown {
		start, end = lo, lo
	}
	var results []adt.Value
	for i := start; i <= end; i++ {
		first, last := i, limit
		if hiKnown {
			first, last = hi, hi
		}
		for j := first; j <= last; j++ {
			if !p.step() {
				return nil
			}
			var part nativeListShape
			for k := i; k < j; k++ {
				element := p.project(value, adt.MakeIntLabel(adt.IntLabel, k))
				if element == nil {
					return nil
				}
				part.prefix = append(part.prefix, element)
			}
			result := p.nativeListValue(part)
			if result == nil {
				return nil
			}
			results = append(results, result)
		}
	}
	return proofUnion(results)
}

// A prefix is guaranteed present; a tail can occur zero or more times.
// Joining after an open tail retains minimum length but must allow elements
// from either segment at the positions whose exact offsets are unknown.
type nativeListShape struct {
	prefix []adt.Value
	tail   adt.Value
}

func (s nativeListShape) elements() adt.Value {
	values := slices.Clone(s.prefix)
	if s.tail != nil {
		values = append(values, s.tail)
	}
	if len(values) == 0 {
		return nil
	}
	return proofUnion(values)
}

func joinNativeListShapes(a, b nativeListShape) nativeListShape {
	if a.tail == nil {
		a.prefix = append(a.prefix, b.prefix...)
		a.tail = b.tail
		return a
	}
	values := []adt.Value{a.tail}
	if element := b.elements(); element != nil {
		values = append(values, element)
	}
	a.tail = proofUnion(values)
	for range b.prefix {
		a.prefix = append(a.prefix, a.tail)
	}
	return a
}

func (p *inference) nativeListShape(value adt.Value) (nativeListShape, bool) {
	if !p.step() {
		return nativeListShape{}, false
	}
	switch x := adt.Unwrap(value).(type) {
	case *adt.RigidType:
		return p.nativeListShape(x.Bound)
	case *adt.LiveType:
		return p.nativeListShape(x.Upper)
	case *adt.Disjunction:
		var parts []nativeListShape
		for _, branch := range x.Values {
			part, ok := p.nativeListShape(branch)
			if !ok {
				return nativeListShape{}, false
			}
			parts = append(parts, part)
		}
		if len(parts) == 0 {
			return nativeListShape{}, false
		}
		n := len(parts[0].prefix)
		closed := true
		for _, part := range parts {
			n = min(n, len(part.prefix))
			closed = closed && part.tail == nil && len(part.prefix) == len(parts[0].prefix)
		}
		out := nativeListShape{}
		for i := range n {
			var elements []adt.Value
			for _, part := range parts {
				elements = append(elements, part.prefix[i])
			}
			out.prefix = append(out.prefix, proofUnion(elements))
		}
		if !closed {
			var elements []adt.Value
			for _, part := range parts {
				elements = append(elements, part.prefix[n:]...)
				if part.tail != nil {
					elements = append(elements, part.tail)
				}
			}
			out.tail = proofUnion(elements)
		}
		return out, true
	}
	list, ok := value.(*adt.Vertex)
	if !ok || !list.IsList() {
		return nativeListShape{}, false
	}
	out := nativeListShape{}
	for field := range list.DerefValue().Elems() {
		element := p.project(value, field.Label)
		if element == nil {
			return nativeListShape{}, false
		}
		out.prefix = append(out.prefix, element)
	}
	if !list.IsClosedList() {
		out.tail = p.project(value, adt.MakeIntLabel(adt.IntLabel, int64(len(out.prefix))))
		if out.tail == nil {
			return nativeListShape{}, false
		}
		if refuted(out.tail) {
			out.tail = nil
		}
	}
	return out, true
}

func (p *inference) nativeListValue(shape nativeListShape) adt.Value {
	if len(shape.prefix) > p.remaining/2 {
		return nil
	}
	out := &adt.ListLit{}
	for _, element := range shape.prefix {
		if !p.step() {
			return nil
		}
		out.Elems = append(out.Elems, element)
	}
	if shape.tail != nil && !refuted(shape.tail) {
		out.Elems = append(out.Elems, &adt.Ellipsis{Value: shape.tail})
	}
	result := p.schema(nil, out)
	if v, ok := result.(*adt.Vertex); ok {
		p.constructors[v] = out
		fields := make(map[adt.Feature]adt.Value)
		for i, element := range shape.prefix {
			fields[adt.MakeIntLabel(adt.IntLabel, int64(i))] = element
		}
		p.projections[v] = fields
	}
	return result
}

func (p *inference) flattenList(value adt.Value, depth *int64) adt.Value {
	if depth != nil && *depth == 0 {
		return value
	}
	shape, ok := p.nativeListShape(value)
	if !ok {
		return nil
	}
	var out nativeListShape
	for _, element := range shape.prefix {
		part, ok := p.flattenElement(element, depth)
		if !ok {
			return nil
		}
		out = joinNativeListShapes(out, part)
	}
	if shape.tail != nil {
		part, ok := p.flattenElement(shape.tail, depth)
		if !ok {
			return nil
		}
		out = joinNativeListShapes(out, nativeListShape{tail: part.elements()})
	}
	return p.nativeListValue(out)
}

func (p *inference) flattenElement(value adt.Value, depth *int64) (nativeListShape, bool) {
	if !p.step() {
		return nativeListShape{}, false
	}
	if union, ok := adt.Unwrap(value).(*adt.Disjunction); ok {
		var results []adt.Value
		for _, branch := range union.Values {
			part, ok := p.flattenElement(branch, depth)
			if !ok {
				return nativeListShape{}, false
			}
			result := p.nativeListValue(part)
			if result == nil {
				return nativeListShape{}, false
			}
			results = append(results, result)
		}
		return p.nativeListShape(proofUnion(results))
	}
	if value.Kind()&adt.ListKind == 0 {
		return nativeListShape{prefix: []adt.Value{value}}, true
	}
	if value.Kind() != adt.ListKind {
		return nativeListShape{tail: &adt.Top{}}, true
	}
	var next *int64
	if depth != nil {
		n := int64(-1)
		if *depth > 0 {
			n = *depth - 1
		}
		next = &n
	}
	result := p.flattenList(value, next)
	if result == nil {
		return nativeListShape{}, false
	}
	if depth == nil {
		// An unknown depth may leave this element intact or descend into
		// it. Independent choices overapproximate the uniform runtime depth.
		intact := p.nativeListValue(nativeListShape{prefix: []adt.Value{value}})
		if intact == nil {
			return nativeListShape{}, false
		}
		result = proofUnion([]adt.Value{intact, result})
	}
	return p.nativeListShape(result)
}

func nativeListIndex(value adt.Value) (int64, bool) {
	switch x := adt.Unwrap(value).(type) {
	case *adt.Num:
		n, err := x.X.Int64()
		return n, err == nil
	case *adt.RigidType:
		return nativeListIndex(x.Bound)
	case *adt.LiveType:
		return nativeListIndex(x.Upper)
	}
	return 0, false
}

func (p *inference) nativeListFailure() adt.Value {
	return &adt.Bottom{Code: adt.EvalError, Err: p.ctx.Newf("invalid native list bounds")}
}
