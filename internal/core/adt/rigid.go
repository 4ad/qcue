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

import "cuelang.org/go/cue/ast"

// RigidType is an arbitrary type in a proof context. Its identity is its
// allocation, and Bound supplies only an upper approximation. In particular,
// a value satisfying Bound need not belong to RigidType. These values never
// escape the proof that introduced them and never act as program witnesses.
type RigidType struct {
	Param *TypeParameter
	Bound Value
}

func (r *RigidType) Source() ast.Node         { return r.Param.Src }
func (*RigidType) node()                      {}
func (*RigidType) expr()                      {}
func (*RigidType) declNode()                  {}
func (*RigidType) elemNode()                  {}
func (*RigidType) Concreteness() Concreteness { return Constraint }
func (r *RigidType) Kind() Kind {
	if r.Bound != nil {
		return r.Bound.Kind()
	}
	return TopKind
}
func (r *RigidType) validate(c *OpContext, value Value) *Bottom {
	if Unwrap(value) == r {
		return nil
	}
	return &Bottom{Code: IncompleteError, Err: c.Newf("membership of arbitrary type %s is unproved", r.Param.Src.Name.Name)}
}

// FunctionTypeParameters returns the still-bound universal telescope.
func FunctionTypeParameters(t FuncType) []*TypeParameter { return typeParameters(t.Env) }

// BindFunctionTypes opens a telescope with proof variables. It performs no
// bound checking: callers must prove the corresponding premises. Program
// instantiation instead uses instantiate, which checks bounds and universes.
func BindFunctionTypes(t FuncType, args []Value) FuncType {
	bindings := make(map[*TypeParameter]Value)
	for i, p := range typeParameters(t.Env) {
		if i < len(args) {
			bindings[p] = args[i]
		}
	}
	t.Env = instantiateEnvironment(t.Env, bindings)
	return t
}

// InstantiateFunctionType selects a use-site instance from a target packet
// schema. It does not introduce a new implementation or discharge the source
// universal obligation.
func InstantiateFunctionType(c *OpContext, source, target FuncType) (FuncType, *Bottom) {
	bindings := make([]funcArg, len(source.Fn.Params))
	matches := matchFuncParams(target.Fn, source.Fn, false)
	for i, j := range matches {
		if j >= 0 {
			bindings[j] = funcArg{expr: target.Fn.Params[i].Value, env: target.Env}
		}
	}
	f, b := (&FuncValue{Fn: source.Fn, Env: source.Env}).inferInstance(c, bindings)
	if b != nil {
		return FuncType{}, b
	}
	source.Env = f.Env
	return source, nil
}
