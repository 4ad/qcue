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

// TypeParameterScope finds the declaring telescope in a possibly deeper
// lexical environment. Bounds use that scope's relative references, not the
// field or nested function scope in which the parameter is later observed.
func TypeParameterScope(env *Environment, param *TypeParameter) *Environment {
	for ; env != nil; env = env.Up {
		if env.types != nil {
			for _, p := range env.types.quantifier.Params {
				if p == param {
					return env
				}
			}
		}
	}
	return nil
}

// SelectFunctionType performs explicit universal elimination. The source's
// conformance is a premise supplied by the caller; selection checks the type
// argument's bound and preserves the original obligations and call frontier.
func SelectFunctionType(c *OpContext, f *FuncValue, argument Value) (*FuncValue, *Bottom) {
	if argument == nil || !f.hasTypeSelection() {
		return nil, &Bottom{Code: IncompleteError,
			Err: c.Newf("no universal clause admits this type application")}
	}
	return f.selectType(c, argument)
}

// BoundArgumentInstances supplies candidate instances for the saved part of
// a packet, in the implementation's parameter coordinates. A conjunction of
// selected views admits any of their domains. The checker must still prove
// saved argument membership in one candidate; these instances neither freeze
// the residual protocol nor discharge the original universal obligations.
func (f *FuncValue) BoundArgumentInstances(c *OpContext) []FuncType {
	views := f.callViews
	if boundary, ok := f.Fn.Body.(*OpaqueCall); ok && f.frontier == nil {
		views = boundary.ProofAlternatives()
		for _, view := range views {
			if !sameOpaqueProtocol(f.Fn, view.Fn) {
				return nil
			}
		}
	}
	if len(views) == 0 {
		views = []*FuncValue{f}
	}
	var instances []FuncType
	for _, view := range views {
		env := view.Env
		if f.IsPartial() && len(typeParameters(env)) != 0 {
			inst, b := view.inferInstance(c, f.args)
			if b != nil {
				continue
			}
			env = inst.Env
		}
		instances = append(instances, FuncType{Fn: view.Fn, Env: env})
	}
	return instances
}

// BindFunctionPacket constructs a residual proof descriptor using the runtime
// packet-binding protocol, without evaluating the body or accepting argument
// membership. The caller separately proves each supplied argument's type.
func BindFunctionPacket(c *OpContext, f *FuncValue, packet FuncType) (*FuncValue, *Bottom) {
	call := &CallExpr{Partial: true}
	for _, arg := range packet.Fn.Params {
		call.Args = append(call.Args, arg.Value)
		call.ArgLabels = append(call.ArgLabels, arg.Label)
	}
	bindings, unused, b := f.bindCall(c, call)
	if b != nil {
		return nil, b
	}
	if unused != nil {
		return nil, unused
	}
	for i := range bindings {
		if i >= len(f.args) || f.args[i].expr == nil {
			bindings[i].env = packet.Env
		}
	}
	copy := *f
	copy.args = bindings
	return &copy, nil
}

// FunctionTypeArguments returns the lexical substitutions selected for a
// function value, keyed by source binder identity for faithful syntax export.
func FunctionTypeArguments(t FuncType) map[*ast.TypeParam]Value {
	args := make(map[*ast.TypeParam]Value)
	for env := t.Env; env != nil; env = env.Up {
		if env.types != nil {
			for param, value := range env.types.arguments {
				args[param.Src] = value
			}
		}
	}
	return args
}

// BindFunctionTypes opens a telescope with proof variables. It performs no
// bound checking: callers must prove the corresponding premises. Program
// instantiation instead uses instantiate, which checks the declared bounds.
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
