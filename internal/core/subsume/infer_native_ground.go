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

	"cuelang.org/go/internal"
	"cuelang.org/go/internal/core/adt"
)

// nativeGroundResult evaluates an already admitted finite packet family. Only
// implementation-owned purity permits execution. Schema-consuming primitives
// keep their declared contracts: data inhabitants do not determine the schema
// syntax, definitions, or attributes those primitives can inspect.
func (p *inference) nativeGroundResult(b *adt.Builtin, packet adt.FuncType, protocol *adt.Function) adt.Value {
	if !b.Pure || b.NonConcrete {
		return nil
	}
	// Grounding is an optional refinement. Leave work for the primary
	// contract when a finite domain is too large to enumerate exhaustively.
	g := nativeGrounder{p: p, remaining: p.remaining / 4, active: make(map[adt.Value]bool)}
	packets := [][]adt.Expr{{}}
	var labels []adt.Feature
	for _, param := range packet.Fn.Params {
		if param.ArcType == adt.ArcOptional || param.Default != nil {
			return nil
		}
		value := p.schema(packet.Env, param.Value)
		values := g.values(value, false)
		if len(values) == 0 || len(values) > g.remaining/len(packets) {
			return nil
		}
		var next [][]adt.Expr
		for _, args := range packets {
			for _, value := range values {
				if !g.step() {
					return nil
				}
				next = append(next, append(slices.Clone(args), value))
			}
		}
		packets = next
		label := adt.InvalidLabel
		if !param.Positional {
			label = param.Label
		}
		labels = append(labels, label)
	}
	var results []adt.Value
	matches := adt.MatchFuncValueParams(packet.Fn, &adt.FuncValue{Fn: protocol})
	for _, args := range packets {
		if !g.step() {
			return nil
		}
		ordered := make([]adt.Value, len(protocol.Params))
		for i, j := range matches {
			if j >= 0 {
				ordered[j] = args[i].(adt.Value)
			}
		}
		if !g.admitsExpansion(b, ordered) {
			return nil
		}
		call := &adt.CallExpr{Fun: b.Implementation(), Args: args, ArgLabels: labels}
		result := p.schema(nil, call)
		if result == nil {
			return nil
		}
		if vertex, ok := result.(*adt.Vertex); ok && vertex.Kind()&(adt.StructKind|adt.ListKind) != 0 &&
			adt.Validate(p.ctx, vertex, &adt.ValidateConfig{Concrete: true, Runtime: true}) == nil {
			out := capturedInventory(vertex, make(map[*adt.Vertex]*adt.Vertex))
			p.grounded[out] = true
			p.constructors[out] = vertex
			result = out
		}
		results = append(results, result)
	}
	return proofUnion(results)
}

type nativeGrounder struct {
	p         *inference
	remaining int
	active    map[adt.Value]bool
}

func (g *nativeGrounder) step() bool {
	if g.remaining == 0 || !g.p.step() {
		return false
	}
	g.remaining--
	return true
}

// Small inputs can request large outputs. Do not expand repetition counts,
// shifts, decimal formatting precision, or numeric ranges beyond the allowance
// while checking a function which can still use its declared native contract.
func (g *nativeGrounder) admitsExpansion(b *adt.Builtin, args []adt.Value) bool {
	if b.Package == adt.InvalidLabel {
		return true
	}
	switch b.Package.StringValue(g.p.ctx) + "." + b.Name {
	case "strings.Repeat", "list.Repeat":
		n, ok := nativeListIndex(args[1])
		if !ok {
			return false
		}
		if n <= 0 || n > adt.MaxRepeatCount {
			return true // These cases fail or return an empty result.
		}
		width := 0
		switch value := adt.Unwrap(args[0]).(type) {
		case *adt.String:
			width = len(value.Str)
		case *adt.Vertex:
			for range value.Elems() {
				width++
			}
		}
		return width == 0 || n <= int64(g.remaining/width)
	case "math/bits.Lsh":
		n, ok := nativeListIndex(args[1])
		return ok && n/8 <= int64(g.remaining)
	case "strconv.FormatFloat":
		precision, ok := nativeListIndex(args[2])
		return ok && precision <= int64(g.remaining)
	case "list.Range":
		start, a := adt.Unwrap(args[0]).(*adt.Num)
		limit, z := adt.Unwrap(args[1]).(*adt.Num)
		step, s := adt.Unwrap(args[2]).(*adt.Num)
		if !a || !z || !s {
			return false
		}
		if step.X.IsZero() {
			return true
		}
		// Use the native's decimal arithmetic: rounding can make an
		// apparently short mathematical range stall instead of terminate.
		current := start.X
		for g.step() {
			order := current.Cmp(&limit.X)
			if !step.X.Negative && order >= 0 || step.X.Negative && order <= 0 {
				return true
			}
			var next internal.Decimal
			if _, err := internal.BaseContext.Add(&next, &current, &step.X); err != nil {
				return true
			}
			current = next
		}
		return false
	}
	return true
}

func (g *nativeGrounder) size(n int64) bool {
	if n > int64(g.remaining) {
		return false
	}
	for range n {
		if !g.step() {
			return false
		}
	}
	return true
}

// values covers every possible datum in a description, without selecting a
// preferred alternative or materializing an open list or record. Record field
// order is observable by serializers, so even a closed record predicate is not
// enough: the inventory must come from a constructor or supplied concrete data.
func (g *nativeGrounder) values(value adt.Value, supplied bool) []adt.Value {
	if value == nil || g.active[value] || !g.step() {
		return nil
	}
	g.active[value] = true
	defer delete(g.active, value)
	switch x := adt.Unwrap(value).(type) {
	case *adt.Null, *adt.Bool:
		return []adt.Value{x.(adt.Value)}
	case *adt.String:
		if g.size(int64(len(x.Str))) {
			return []adt.Value{x}
		}
		return nil
	case *adt.Bytes:
		if g.size(int64(len(x.B))) {
			return []adt.Value{x}
		}
		return nil
	case *adt.Num:
		exponent := int64(x.X.Exponent)
		if exponent < 0 {
			exponent = -exponent
		}
		if g.size(x.X.NumDigits() + exponent) {
			return []adt.Value{x}
		}
		return nil
	case *adt.RigidType:
		return g.values(x.Bound, false)
	case *adt.LiveType:
		return g.values(x.Upper, false)
	case *adt.Disjunction:
		var results []adt.Value
		for _, branch := range x.Values {
			values := g.values(branch, supplied)
			if len(values) == 0 {
				return nil
			}
			results = append(results, values...)
		}
		return results
	case *adt.Conjunction:
		// Any finite conjunct bounds the whole intersection. Keeping its
		// extra possibilities is conservative and never proves emptiness.
		for _, term := range x.Values {
			if values := g.values(term, supplied); len(values) != 0 {
				return values
			}
		}
		return nil
	case *adt.Vertex:
		original, _ := value.(*adt.Vertex)
		supplied = supplied || g.p.grounded[original] || g.p.grounded[x]
		if x.IsList() {
			if !x.IsClosedList() {
				return nil
			}
		} else if x.Kind() != adt.StructKind ||
			!supplied && g.p.constructors[original] == nil && g.p.constructors[x] == nil {
			return nil
		}
		base := x.ToDataSingle()
		results := []*adt.Vertex{base}
		for i, field := range x.Arcs {
			if !field.Label.IsRegular() {
				continue
			}
			if field.ArcType != adt.ArcMember && field.ArcType != adt.ArcRequired {
				return nil
			}
			var member adt.Value = field
			if v := g.p.projections[original][field.Label]; v != nil {
				member = v
			} else if v := g.p.projections[x][field.Label]; v != nil {
				member = v
			}
			values := g.values(member, supplied)
			if len(values) == 0 || len(values) > g.remaining/len(results) {
				return nil
			}
			var next []*adt.Vertex
			for _, result := range results {
				for _, v := range values {
					if !g.step() {
						return nil
					}
					out := result.ToDataSingle()
					out.Arcs = slices.Clone(result.Arcs)
					arc := field.ToDataSingle()
					if vertex, ok := v.(*adt.Vertex); ok {
						arc = vertex.ToDataSingle()
					} else {
						arc.BaseValue, arc.Arcs = v, nil
					}
					arc.Label, arc.ArcType = field.Label, adt.ArcMember
					out.Arcs[i] = arc
					next = append(next, out)
				}
			}
			results = next
		}
		values := make([]adt.Value, len(results))
		for i, result := range results {
			values[i] = result
		}
		return values
	}
	return nil
}
