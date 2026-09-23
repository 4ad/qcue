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
func (b *Builtin) FreezeSignature() { b.declared = slices.Clone(b.Types) }

// AdditionalTypes returns the contracts not supplied by the builtin package.
func (b *Builtin) AdditionalTypes() []FuncType {
	var result []FuncType
	for _, t := range b.Types {
		if !slices.Contains(b.declared, t) {
			result = append(result, t)
		}
	}
	return result
}

func (b *Builtin) guardedType(t FuncType) bool {
	return b.capabilityMode() && !slices.Contains(b.declared, t)
}

func (b *Builtin) capabilityMode() bool {
	for _, t := range b.Types {
		if t.Fn.Quantified && !slices.Contains(b.declared, t) {
			return true
		}
	}
	return false
}

func (b *Builtin) protocolTypes() []FuncType {
	if b.capabilityMode() {
		return b.declared
	}
	return b.Types
}

// Implementation returns the primitive with only its package declarations.
// Proofs and counterexample checks must not assume client-added contracts.
func (b *Builtin) Implementation() *Builtin {
	copy := *b
	copy.Types = b.declared
	return &copy
}

// Protocol describes the primitive's fixed call slots. It does not assert
// that all admitted calls succeed; that requires a separate primitive rule.
func (b *Builtin) Protocol(c *OpContext) *Function {
	f := &Function{Quantified: true, Ret: &BasicType{K: b.Result}}
	labels, _ := builtinParamLabels(b.declared)
	for i, p := range b.Params {
		param := FuncParam{Positional: true, Value: p.Value,
			Local: anonParamLabel(c, i), Default: p.Default()}
		for label, index := range labels {
			if index == i {
				param.Label = label
			}
		}
		for _, t := range b.declared {
			for j, index := range matchBuiltinParamsWith(t.Fn, b, b.declared) {
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
	act := c.newInlineVertex(nil, &StructMarker{})
	for i, arg := range args {
		v := c.newInlineVertex(nil, nil, MakeRootConjunct(nil, arg))
		v.Label = f.Params[i].Local
		act.Arcs = append(act.Arcs, v)
	}
	if len(typeParameters(t.Env)) != 0 {
		bindings := make([]funcArg, len(t.Fn.Params))
		for i, j := range matchFuncParams(t.Fn, f, false) {
			if j >= 0 && j < len(args) {
				bindings[i] = funcArg{expr: args[j]}
			}
		}
		inst, err := (&FuncValue{Fn: t.Fn, Env: t.Env}).inferInstance(c, bindings)
		if err != nil {
			if err.IsIncomplete() {
				return t, proofUnknown
			}
			return t, proofRefuted
		}
		t.Env = inst.Env
	}
	return t, capabilityApplies(c, t, f, nil, act)
}
