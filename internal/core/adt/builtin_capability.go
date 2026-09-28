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

import "slices"

// FreezeSignature records the package's own declarations before clients can
// attach contracts. It is called only while constructing the package.
func (b *Builtin) FreezeSignature() {
	b.self().declared = slices.Clone(b.Types)
}

func (b *Builtin) declaredTypes() []FuncType { return b.self().declared }

func (b *Builtin) isDeclaredType(t FuncType) bool {
	// Imports may reevaluate the same package declaration in a fresh
	// environment, particularly with structure sharing disabled. Its code
	// origin still identifies that immutable package declaration. A client
	// signature has its own origin even when its text is identical.
	return slices.ContainsFunc(b.declaredTypes(), func(d FuncType) bool { return d.Fn == t.Fn })
}

// AdditionalTypes returns the contracts not supplied by the builtin package.
func (b *Builtin) AdditionalTypes() []FuncType {
	var result []FuncType
	for _, t := range b.Types {
		if !b.isDeclaredType(t) {
			result = append(result, t)
		}
	}
	return result
}

func (b *Builtin) guardedType(t FuncType) bool {
	return b.capabilityMode() && !b.isDeclaredType(t)
}

func (b *Builtin) capabilityMode() bool {
	for _, t := range b.Types {
		if t.Fn.Quantified && !b.isDeclaredType(t) {
			return true
		}
	}
	return false
}

func (b *Builtin) protocolTypes() []FuncType {
	if b.capabilityMode() {
		return b.declaredTypes()
	}
	return b.Types
}

// Implementation returns the primitive with only its package declarations.
// Proofs and counterexample checks must not assume client-added contracts.
func (b *Builtin) Implementation() *Builtin {
	copy := *b
	copy.Types = b.declaredTypes()
	return &copy
}

// Protocol describes the primitive's fixed call slots. It does not assert
// that all admitted calls succeed; that requires a separate primitive rule.
func (b *Builtin) Protocol(c *OpContext) *Function {
	f := &Function{Quantified: true, Ret: &BasicType{K: b.Result}}
	// Structural kinds need structural descriptions: a bare kind mask has
	// no list tail or field inventory for subsequent eliminations.
	switch b.Result {
	case ListKind:
		f.Ret = &ListLit{Elems: []Elem{&Ellipsis{}}}
	case StructKind:
		f.Ret = &StructLit{}
	case BottomKind:
		f.Ret = &Bottom{Code: EvalError, Err: c.Newf("builtin has no successful result")}
	}
	labels, _ := builtinParamLabels(b.declaredTypes())
	contracts := b.declaredTypes()
	signature := b.self().Signature
	if signature != nil {
		// Protocol describes the whole native domain. Erase universal
		// variables to their upper bounds here; CheckingType retains their
		// input-output relationships for each checked invocation.
		t := *signature
		for _, param := range FunctionTypeParameters(t) {
			var bound Value = &Top{}
			if param.Bound != nil {
				bound, _ = c.Evaluate(TypeParameterScope(t.Env, param), param.Bound)
			}
			t = BindFunctionTypes(t, []Value{bound})
		}
		contracts = append(slices.Clone(contracts), t)
	}
	for i, p := range b.Params {
		param := FuncParam{Positional: true, Value: p.Value,
			Local: anonParamLabel(c, i), Default: p.Default()}
		for label, index := range labels {
			if index == i {
				param.Label = label
			}
		}
		for _, t := range contracts {
			for j, index := range matchBuiltinParamsWith(t.Fn, b, b.declaredTypes()) {
				if index != i {
					continue
				}
				declared := t.Fn.Params[j]
				if declared.Value != nil {
					param.Value = builtinConstraint(param.Value, t.Env, declared.Value)
				}
				if declared.Default != nil && (signature == nil || t.Fn != signature.Fn) {
					param.Default, _ = c.Evaluate(t.Env, declared.Default)
				}
			}
		}
		f.Params = append(f.Params, param)
	}
	for _, t := range contracts {
		if t.Fn.Ret != nil {
			f.Ret = builtinConstraint(f.Ret, t.Env, t.Fn.Ret)
		}
	}
	return f
}

// CheckingType retains the primitive's universal relationships while adopting
// the native call slots, labels, defaults, and independently declared bounds.
// These contracts are implementation evidence, never client assertions.
func (b *Builtin) CheckingType(c *OpContext) FuncType {
	protocol := b.Protocol(c)
	signature := b.self().Signature
	if signature == nil || len(FunctionTypeParameters(*signature)) == 0 {
		return FuncType{Fn: protocol}
	}
	fn := *signature.Fn
	fn.Params = slices.Clone(protocol.Params)
	for i, index := range matchBuiltinParamsWith(signature.Fn, b, b.declaredTypes()) {
		if index >= 0 {
			fn.Params[index].Value = &BinaryExpr{Op: AndOp,
				X: signature.Fn.Params[i].Value, Y: fn.Params[index].Value}
		}
	}
	fn.Ret = &BinaryExpr{Op: AndOp, X: signature.Fn.Ret, Y: protocol.Ret}
	return FuncType{Fn: &fn, Env: signature.Env}
}

// Keep a package declaration in its own environment, including any named
// schemas. Only frozen declarations contribute: a client's result annotation
// cannot become implementation evidence through the native protocol.
func builtinConstraint(raw Expr, env *Environment, declared Expr) Expr {
	v := &Vertex{}
	v.AddConjunct(MakeRootConjunct(env, declared))
	return &BinaryExpr{Op: AndOp, X: raw, Y: v}
}

func (b *Builtin) capabilityApplies(c *OpContext, t FuncType, args []Value) (FuncType, proofResult) {
	f := b.Protocol(c)
	packet := callPacket{args: make([]funcArg, len(f.Params))}
	for i, arg := range args {
		packet.args[i] = funcArg{expr: arg}
	}
	projected, result := packet.project(t, f)
	if result != proofEstablished {
		return t, result
	}
	admission, result := projected.admit(c, t, false)
	if admission != nil {
		t = admission.clause
	}
	return t, result
}
