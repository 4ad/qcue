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

// builtinCall uses one protocol for direct calls, aliases, and primitive
// capability proofs. Argument-dependent results retain the checked packet.
func (p *inference) builtinCall(f *adt.Builtin, target adt.FuncType) adt.Value {
	// Primitive successful-result rules and packet coverage are separate
	// obligations.
	// Use the same protocol as builtin capability inclusion, including
	// its label and omission rules.
	source := adt.FuncType{Fn: primitiveContract(p.ctx, f)}
	if source.Fn == nil || ValidateBuiltin(p.ctx, f) != nil {
		return nil
	}
	if f.IsValidator(len(target.Fn.Params)) {
		// The implicit validator constructor saves the trailing slots;
		// its successful inhabitants have the validated slot's type.
		// This form has no argument labels or partial-call protocol.
		for _, param := range target.Fn.Params {
			if param.Label != adt.InvalidLabel {
				return nil
			}
		}
		protocol := *source.Fn
		protocol.Params = protocol.Params[1:]
		protocol.Ret = nil
		if !(&subsumer{ctx: p.ctx, inference: p}).capabilitySignature(target, adt.FuncType{Fn: &protocol}) {
			return nil
		}
		result := f.Params[0].Value
		if basic, ok := result.(*adt.BasicType); ok {
			switch basic.K {
			case adt.StructKind:
				return p.schema(nil, &adt.StructLit{})
			case adt.ListKind:
				return p.schema(nil, &adt.ListLit{Elems: []adt.Elem{&adt.Ellipsis{}}})
			}
		}
		return result
	}
	// Native slots consume values of their declared types. Unlike a CUE
	// activation, a native call cannot constrain an ill-typed argument
	// into an empty packet and use that contradiction as result evidence.
	protocol := *source.Fn
	protocol.Ret = nil
	if !(&subsumer{ctx: p.ctx, inference: p}).capabilitySignature(target, adt.FuncType{Fn: &protocol}) {
		return nil
	}
	sources := []adt.FuncType{source}
	results := append(slices.Clone(sources), f.AdditionalTypes()...)
	result := p.callPackets(target, sources, results, nil, 0)
	if result == nil || refuted(result) || f.Package != adt.InvalidLabel {
		return result
	}
	switch f.Name {
	case "len":
		return p.length(target.Fn.Params[0].Value.(adt.Value))
	case "and", "or":
		return p.listFold(target.Fn.Params[0].Value.(adt.Value), f.Name == "and")
	case "close":
		return p.closeResult(f, target.Fn.Params[0].Value.(adt.Value))
	case "__reclose", "__closeAll", "testExperiment":
		return target.Fn.Params[0].Value.(adt.Value)
	}
	return result
}

func (p *inference) closeResult(b *adt.Builtin, value adt.Value) adt.Value {
	if !p.step() {
		return nil
	}
	switch x := adt.Unwrap(value).(type) {
	case *adt.RigidType, *adt.LiveType:
		// Closing only narrows the input. A rigid or live predicate cannot be
		// executed as a concrete record: that would create a spurious failure
		// and prove arbitrary result annotations by explosion.
		return value
	case *adt.Disjunction:
		var results []adt.Value
		for _, branch := range x.Values {
			result := p.closeResult(b, branch)
			if result == nil {
				return nil
			}
			results = append(results, result)
		}
		return proofUnion(results)
	}
	return p.schema(nil, &adt.CallExpr{Fun: b.Implementation(), Args: []adt.Expr{value}})
}
