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

func isListComparerBuiltin(c *adt.OpContext, b *adt.Builtin) bool {
	return b.Package != adt.InvalidLabel && b.Package.StringValue(c) == "list" &&
		(b.Name == "Sort" || b.Name == "SortStable" || b.Name == "IsSorted")
}

// The list sorting primitives invoke a record template by supplying x and y.
// Check that template under exactly the element descriptions the native can
// supply. Checking its uninstantiated comparisons would reject idiomatic
// comparers such as list.Ascending, whose number|string fields become concrete
// only when the native supplies an element pair.
func (p *inference) listComparerArguments(env *adt.Environment, callee adt.Value, call *adt.CallExpr) (map[int]adt.Value, bool) {
	b, ok := callee.(*adt.Builtin)
	if !ok || !isListComparerBuiltin(p.ctx, b) || len(call.Args) != 2 {
		return nil, false
	}
	slots := [2]int{-1, -1}
	next := 0
	for i := range call.Args {
		label := adt.InvalidLabel
		if i < len(call.ArgLabels) {
			label = call.ArgLabels[i]
		}
		if label == adt.InvalidLabel {
			for next < len(slots) && slots[next] >= 0 {
				next++
			}
			if next == len(slots) {
				return nil, true
			}
			slots[next] = i
		} else {
			index, ok := adt.BuiltinParamLabelIndex(b, label)
			if !ok || index >= len(slots) || slots[index] >= 0 {
				return nil, true
			}
			slots[index] = i
		}
	}
	listIndex, comparerIndex := slots[0], slots[1]
	if listIndex < 0 || comparerIndex < 0 || listIndex == comparerIndex {
		return nil, true
	}
	list := p.expr(env, call.Args[listIndex])
	if list == nil || list.Kind() != adt.ListKind {
		return nil, true
	}
	element := p.listFold(list, false)
	if element == nil {
		return nil, true
	}
	shape, shaped := p.nativeListShape(list)
	unused := shaped && shape.tail == nil && len(shape.prefix) < 2
	if unused {
		// Sorting fewer than two elements never invokes the template.
		// Check its expressions under the empty comparison domain, while
		// retaining the native setup requirement that less is present.
		element = &adt.Bottom{Code: adt.EvalError, Err: p.ctx.Newf("empty comparison domain")}
	}
	if comparer := p.expr(env, call.Args[comparerIndex]); comparer != nil {
		if unused {
			if p.project(comparer, p.ctx.StringLabel("less")) == nil {
				return nil, true
			}
			return map[int]adt.Value{listIndex: list, comparerIndex: p.unusedListComparer()}, true
		}
		for _, name := range []string{"x", "y"} {
			bound := p.project(comparer, p.ctx.StringLabel(name))
			if bound == nil || !p.includes(bound, element) {
				return nil, true
			}
		}
		return map[int]adt.Value{listIndex: list, comparerIndex: comparer}, true
	}
	bindings := []proofBinding{{env: env, expr: call.Args[comparerIndex]}}
	record := p.prepareRecord(bindings)
	if record == nil {
		// Imported templates retain their original declaration scopes.
		// Inspect those declarations without materializing the comparator.
		if v, ok := p.schema(env, call.Args[comparerIndex]).(*adt.Vertex); ok {
			bindings = nil
			for _, c := range v.Conjuncts {
				bindings = append(bindings, proofBinding{c.Env, c.Expr()})
			}
			record = p.prepareRecord(bindings)
		}
	}
	if record == nil {
		return nil, true
	}
	if mode, present := record.scope.presence[p.ctx.StringLabel("less")]; unused && (!present || mode == adt.ArcOptional) {
		return nil, true
	}
	packet := &adt.StructLit{}
	for _, name := range []string{"x", "y"} {
		label := p.ctx.StringLabel(name)
		bound := p.expr(record.env, &adt.FieldReference{Label: label})
		if !unused && (bound == nil || !p.includes(bound, element)) {
			return nil, true
		}
		packet.Decls = append(packet.Decls, &adt.Field{Label: label, Value: element})
	}
	bindings = append(bindings, proofBinding{expr: packet})
	comparer, ok := p.recordMeet(bindings)
	if !ok || comparer == nil || !unused && refuted(comparer) {
		return nil, true
	}
	if unused {
		comparer = p.unusedListComparer()
	}
	return map[int]adt.Value{listIndex: list, comparerIndex: comparer}, true
}

// This is evidence for a template with no invocation packets. Its result
// cannot affect the native call; it is not evidence about a comparator used
// with two or more elements, or about the standalone template's field values.
func (p *inference) unusedListComparer() adt.Value {
	return p.schema(nil, &adt.StructLit{Decls: []adt.Decl{
		&adt.Field{Label: p.ctx.StringLabel("x"), Value: &adt.Top{}},
		&adt.Field{Label: p.ctx.StringLabel("y"), Value: &adt.Top{}},
		&adt.Field{Label: p.ctx.StringLabel("less"), Value: &adt.BasicType{K: adt.BoolKind}},
	}})
}

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
	source := primitiveContract(p.ctx, f)
	if source.Fn == nil || ValidateBuiltin(p.ctx, f) != nil {
		return nil
	}
	if f.IsValidator(len(target.Fn.Params)) {
		// The implicit validator constructor saves the trailing slots;
		// its successful inhabitants have the validated slot's type.
		// Labels select only those trailing slots, just as at runtime.
		protocol := *source.Fn
		protocol.Params = protocol.Params[1:]
		protocol.Ret = nil
		if !(&subsumer{ctx: p.ctx, inference: p}).capabilitySignature(target, adt.FuncType{Fn: &protocol, Env: source.Env}) {
			return nil
		}
		result := p.schema(source.Env, source.Fn.Params[0].Value)
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
	if !(&subsumer{ctx: p.ctx, inference: p}).capabilitySignature(target, adt.FuncType{Fn: &protocol, Env: source.Env}) {
		return nil
	}
	sources := []adt.FuncType{source}
	results := append(slices.Clone(sources), f.CheckingRefinements(p.ctx)...)
	results = append(results, f.AdditionalTypes()...)
	result := p.callPackets(target, sources, results, nil, 0)
	if result == nil || refuted(result) {
		return result
	}
	if f.Package != adt.InvalidLabel {
		if precise := p.nativeListResult(f, target, &protocol); precise != nil {
			if p.includes(result, precise) {
				return precise
			}
			return p.eagerMeet(result, precise)
		}
		return result
	}
	switch f.Name {
	case "len":
		return p.length(target.Fn.Params[0].Value.(adt.Value))
	case "and", "or":
		return p.listFold(target.Fn.Params[0].Value.(adt.Value), f.Name == "and")
	case "close":
		return p.closeResult(f, target.Fn.Params[0].Value.(adt.Value))
	case "__reclose", "__closeAll", "testExperiment", "validator":
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
