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
			return &adt.Bottom{Src: t.Fn.Source(), Code: adt.IncompleteError,
				Err: c.Newf("builtin conformance remains unproved")}
		}
	}
	return nil
}

// Only primitives with a known total rule can discharge a pure arrow.
// Native parameter/result kinds alone say nothing about failure or effects.
func primitiveContract(c *adt.OpContext, b *adt.Builtin) *adt.Function {
	f := b.Protocol(c)
	if b.Package == adt.InvalidLabel && b.Name == "len" {
		f.Params[0].Value = &adt.BasicType{K: adt.StructKind | adt.ListKind | adt.StringKind | adt.BytesKind}
		return f
	}
	if b.Package != adt.InvalidLabel && b.Package.StringValue(c) == "strings" {
		switch b.Name {
		case "ToUpper", "ToLower", "ToTitle", "Compare", "Contains", "ContainsAny", "HasPrefix", "HasSuffix":
			return f
		}
	}
	return nil
}

func (s *subsumer) builtinCapability(target adt.FuncType, b *adt.Builtin) bool {
	primitive := primitiveContract(s.ctx, b)
	if primitive == nil {
		return false
	}
	source := adt.FuncType{Fn: primitive}
	if s.capabilitySignature(target, source) {
		return true
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
