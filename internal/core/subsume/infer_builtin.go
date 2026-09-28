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

// The list sorting primitives invoke a record template by supplying x and y.
// Check that template under exactly the element descriptions the native can
// supply. Checking its uninstantiated comparisons would reject idiomatic
// comparers such as list.Ascending, whose number|string fields become concrete
// only when the native supplies an element pair.
func (p *inference) listComparerArguments(env *adt.Environment, callee adt.Value, call *adt.CallExpr) (map[int]adt.Value, bool) {
	var b *adt.Builtin
	var saved *adt.FuncValue
	switch f := callee.(type) {
	case *adt.Builtin:
		b = f
	case *adt.FuncValue:
		b, saved = f.NativeBuiltin(), f
	}
	if b == nil || !b.IsListComparer(p.ctx) {
		return nil, false
	}
	if saved == nil && !call.Partial && b.IsValidator(len(call.Args)) {
		// Construction saves the template without invoking a comparison.
		// Check its source operations under the empty invocation domain;
		// the eventual validated list supplies x and y at runtime.
		if len(call.ArgLabels) != 0 && call.ArgLabels[0] != adt.InvalidLabel {
			if i, ok := adt.BuiltinParamLabelIndex(b, call.ArgLabels[0]); !ok || i != 1 {
				return nil, true
			}
		}
		empty := p.schema(nil, &adt.ListLit{})
		comparer := p.listComparer(empty, proofBinding{env: env, expr: call.Args[0]})
		if comparer == nil {
			return nil, true
		}
		return map[int]adt.Value{0: comparer}, true
	}
	// Normalize the residual call together with the saved arguments. Both
	// packets use the primitive's original list and comparator coordinates.
	bindings := [2]proofBinding{}
	slots := [2]int{-1, -1}
	if saved != nil {
		for i := range bindings {
			bindings[i].env, bindings[i].expr = saved.BoundArgument(i)
		}
	}
	next := 0
	for i, arg := range call.Args {
		label := adt.InvalidLabel
		if i < len(call.ArgLabels) {
			label = call.ArgLabels[i]
		}
		index := -1
		if label == adt.InvalidLabel {
			for next < len(bindings) && bindings[next].expr != nil {
				next++
			}
			index = next
		} else {
			var ok bool
			index, ok = adt.BuiltinParamLabelIndex(b, label)
			if !ok {
				return nil, true
			}
		}
		if index < 0 || index >= len(bindings) || bindings[index].expr != nil {
			return nil, true
		}
		slots[index] = i
		bindings[index] = proofBinding{env: env, expr: arg}
	}
	if bindings[0].expr == nil && bindings[1].expr != nil && call.Partial {
		comparer := p.deferredListComparer(bindings[1])
		if comparer == nil {
			return nil, true
		}
		contextual := make(map[int]adt.Value)
		if slots[1] >= 0 {
			contextual[slots[1]] = comparer
		}
		return contextual, true
	}
	if bindings[0].expr == nil || bindings[1].expr == nil {
		return nil, false
	}
	var list adt.Value
	if slots[0] >= 0 {
		list = p.expr(bindings[0].env, bindings[0].expr)
	} else {
		list = p.boundArgument(bindings[0].env, bindings[0].expr)
	}
	if list == nil || list.Kind() != adt.ListKind {
		return nil, true
	}
	comparer := p.listComparer(list, bindings[1])
	if comparer == nil {
		return nil, true
	}
	contextual := make(map[int]adt.Value)
	if slots[0] >= 0 {
		contextual[slots[0]] = list
	}
	if slots[1] >= 0 {
		contextual[slots[1]] = comparer
	}
	return contextual, true
}

type comparerProofKey struct {
	binding proofBinding
	element adt.Value
	kind    adt.Kind
	unused  bool
}

func (p *inference) listComparer(list adt.Value, binding proofBinding) (result adt.Value) {
	element := p.listFold(list, false)
	if element == nil {
		return nil
	}
	shape, shaped := p.nativeListShape(list)
	unused := shaped && shape.tail == nil && len(shape.prefix) < 2
	key := comparerProofKey{binding: binding, element: adt.Unwrap(element), unused: unused}
	if unused {
		key.element = nil
	} else if basic, ok := key.element.(*adt.BasicType); ok {
		// Base kinds have no scoped dependencies. Their fresh descriptions
		// denote the same comparison domain across residual call proofs.
		key.kind, key.element = basic.K, nil
	}
	if certificate, ok := p.comparerProofs[key]; ok {
		available := true
		for _, h := range certificate.required {
			available = available && p.hypotheses[h]
		}
		if available {
			for _, h := range certificate.required {
				p.useHypothesis(h)
			}
			return certificate.result
		}
	}
	// Residual interfaces can check the same template repeatedly. Retain
	// its proof only for this source scope and comparison domain, with all
	// required callback hypotheses. Resumption rereads live captures.
	attempt := &proofAttempt{inherited: p.hypotheses, required: make(map[*adt.FuncValue]bool)}
	p.attempts = append(p.attempts, attempt)
	defer func() {
		p.attempts = p.attempts[:len(p.attempts)-1]
		if result != nil {
			certificate := proofCertificate{result: result}
			for h := range attempt.required {
				certificate.required = append(certificate.required, h)
			}
			p.comparerProofs[key] = certificate
		}
	}()
	if unused {
		// Sorting fewer than two elements never invokes the template.
		// Check its expressions under the empty comparison domain, while
		// retaining the native setup requirement that less is present.
		element = &adt.Bottom{Code: adt.EvalError, Err: p.ctx.Newf("empty comparison domain")}
	}
	if comparer := p.expr(binding.env, binding.expr); comparer != nil {
		if original := p.comparers[adt.Unwrap(comparer)]; original.expr != nil {
			return p.listComparer(list, original)
		}
		if unused {
			if p.project(comparer, p.ctx.StringLabel("less")) == nil {
				return nil
			}
			return p.unusedListComparer()
		}
		for _, name := range []string{"x", "y"} {
			bound := p.project(comparer, p.ctx.StringLabel(name))
			if bound == nil || !p.includes(bound, element) {
				return nil
			}
		}
		return comparer
	}
	bindings := []proofBinding{binding}
	record := p.prepareRecord(bindings)
	if record == nil {
		// Imported templates retain their original declaration scopes.
		// Inspect those declarations without materializing the comparator.
		if v, ok := p.schema(binding.env, binding.expr).(*adt.Vertex); ok {
			bindings = nil
			for _, c := range v.Conjuncts {
				bindings = append(bindings, proofBinding{c.Env, c.Expr()})
			}
			record = p.prepareRecord(bindings)
		}
	}
	if record == nil {
		return nil
	}
	if mode, present := record.scope.presence[p.ctx.StringLabel("less")]; unused && (!present || mode == adt.ArcOptional) {
		return nil
	}
	packet := &adt.StructLit{}
	for _, name := range []string{"x", "y"} {
		label := p.ctx.StringLabel(name)
		bound := p.expr(record.env, &adt.FieldReference{Label: label})
		if !unused && (bound == nil || !p.includes(bound, element)) {
			return nil
		}
		packet.Decls = append(packet.Decls, &adt.Field{Label: label, Value: element})
	}
	bindings = append(bindings, proofBinding{expr: packet})
	comparer, ok := p.recordMeet(bindings)
	if !ok || comparer == nil || !unused && refuted(comparer) {
		return nil
	}
	if unused {
		comparer = p.unusedListComparer()
	}
	return comparer
}

func (p *inference) nativeArgument(native *adt.Builtin, slot int, env *adt.Environment, expr adt.Expr) adt.Value {
	if native != nil && slot == 1 && native.IsListComparer(p.ctx) {
		return p.deferredListComparer(proofBinding{env: env, expr: expr})
	}
	return p.boundArgument(env, expr)
}

// Only native template storage consumes this description. It cannot establish
// comparison results or replace the source during a later invocation.
func (p *inference) deferredListComparer(binding proofBinding) adt.Value {
	if value, ok := binding.expr.(adt.Value); ok {
		if original := p.comparers[adt.Unwrap(value)]; original.expr != nil {
			binding = original
		}
	}
	value := p.listComparer(p.schema(nil, &adt.ListLit{}), binding)
	if value != nil {
		p.comparers[adt.Unwrap(value)] = binding
	}
	return value
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
	if precise := p.nativeListResult(f, target, &protocol); precise != nil {
		if p.includes(result, precise) {
			result = precise
		} else {
			result = p.eagerMeet(result, precise)
		}
	}
	if precise := p.nativeGroundResult(f, target, &protocol); precise != nil {
		if p.includes(result, precise) {
			return precise
		}
		return p.eagerMeet(result, precise)
	}
	if f.Package != adt.InvalidLabel {
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
