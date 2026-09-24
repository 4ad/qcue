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
	labels, _ := builtinParamLabels(b.declaredTypes())
	for i, p := range b.Params {
		param := FuncParam{Positional: true, Value: p.Value,
			Local: anonParamLabel(c, i), Default: p.Default()}
		for label, index := range labels {
			if index == i {
				param.Label = label
			}
		}
		for _, t := range b.declaredTypes() {
			for j, index := range matchBuiltinParamsWith(t.Fn, b, b.declaredTypes()) {
				if index == i && t.Fn.Params[j].Default != nil {
					param.Default, _ = c.Evaluate(t.Env, t.Fn.Params[j].Default)
				}
			}
		}
		f.Params = append(f.Params, param)
	}
	return f
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
