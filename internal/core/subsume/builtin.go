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

// ValidateBuiltin checks client contracts independently of their attachment.
// A callable primitive descriptor does not prove arbitrary universal arrows.
func ValidateBuiltin(c *adt.OpContext, b *adt.Builtin) *adt.Bottom {
	types := b.AdditionalTypes()
	quantified := false
	for _, t := range types {
		quantified = quantified || t.Fn.Quantified
	}
	if !quantified {
		return nil
	}
	s := &subsumer{ctx: c}
	for _, t := range types {
		if !s.builtinCapability(t, b) {
			return &adt.Bottom{Src: t.Fn.Source(), Code: adt.BlockedError,
				Err: c.Newf("builtin conformance remains unproved")}
		}
	}
	return nil
}

// Builtin descriptors are implementation-owned primitive contracts. Their
// result kinds describe every successful return; errors and incomplete calls
// do not promise a value. Client attachments are deliberately excluded from
// Protocol. ExternalFunc, unlike Builtin, supplies no such contract.
func primitiveContract(c *adt.OpContext, b *adt.Builtin) adt.FuncType {
	return b.CheckingType(c)
}

func (s *subsumer) builtinCapability(target adt.FuncType, b *adt.Builtin) bool {
	source := primitiveContract(s.ctx, b)
	primitive := source.Fn
	if primitive == nil {
		return false
	}
	if b.IsValidator(len(target.Fn.Params)) {
		constructor := *primitive
		constructor.Params = primitive.Params[1:]
		constructor.Ret = primitive.Params[0].Value
		return s.capabilitySignature(target, adt.FuncType{Fn: &constructor, Env: source.Env})
	}
	if isListComparerBuiltin(s.ctx, b) {
		return s.builtinBodyCapability(target, b)
	}
	if s.capabilitySignature(target, source) {
		return true
	}
	for _, refined := range b.CheckingRefinements(s.ctx) {
		if s.capabilitySignature(target, refined) {
			return true
		}
	}
	if isListShapeBuiltin(s.ctx, b) {
		return s.builtinBodyCapability(target, b)
	}
	if b.Package == adt.InvalidLabel {
		switch b.Name {
		case "and", "or", "len", "close":
			// Check the same argument-dependent rule used by direct calls
			// under the target's rigid packet. This adapter is proof syntax
			// only; it neither changes the builtin's protocol nor executes it.
			return s.builtinBodyCapability(target, b)
		}
	}
	// A singleton packet can be checked exhaustively. This is a proof over
	// its entire domain, not a successful sample of an infinite predicate.
	// First establish the full protocol and argument coverage independently
	// of the result. Optional/defaulted or generic packet families remain
	// outside this finite rule.
	fn := *target.Fn
	fn.Ret = nil
	if !s.capabilitySignature(adt.FuncType{Fn: &fn, Env: target.Env}, source) ||
		len(adt.FunctionTypeParameters(target)) != 0 || len(fn.Params) != len(primitive.Params) {
		return false
	}
	call := &adt.CallExpr{Fun: b.Implementation()}
	for _, param := range fn.Params {
		if param.ArcType == adt.ArcOptional || param.Default != nil {
			return false
		}
		v, ok := s.evalFuncConstraint(target.Env, param.Value)
		if !ok {
			return false
		}
		switch adt.Unwrap(v).(type) {
		case *adt.String, *adt.Bytes, *adt.Num, *adt.Bool, *adt.Null:
		default:
			return false
		}
		call.Args = append(call.Args, v)
		label := adt.InvalidLabel
		if !param.Positional {
			label = param.Label
		}
		call.ArgLabels = append(call.ArgLabels, label)
	}
	result, complete := s.ctx.Evaluate(target.Env, call)
	if !complete || result == nil {
		return false
	}
	if _, bottom := adt.Unwrap(result).(*adt.Bottom); bottom {
		return false
	}
	want, ok := s.evalFuncConstraint(target.Env, target.Fn.Ret)
	return ok && s.values(want, result)
}

func (s *subsumer) builtinBodyCapability(target adt.FuncType, b *adt.Builtin) bool {
	p := s.inference
	if p == nil {
		p = newInference(s.ctx)
	}
	defer p.enter()()
	// This adapter has the raw native slots, without a universal telescope
	// of its own. Its body instantiates the native's checking scheme under
	// the target packet, which may introduce a different number of binders.
	primitive := *b.Protocol(s.ctx)
	call := &adt.CallExpr{Fun: b.Implementation()}
	for _, param := range primitive.Params {
		call.Args = append(call.Args, &adt.FieldReference{Label: param.Local})
	}
	primitive.Body = call
	return p.function(&adt.FuncValue{Fn: &primitive}, target)
}
