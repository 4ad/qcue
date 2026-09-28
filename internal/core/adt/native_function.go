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

// IsListComparer identifies primitives which instantiate a comparison record
// by supplying x and y. The comparison record is a schema argument: its local
// fields need not be concrete before the primitive supplies an element pair.
func (b *Builtin) IsListComparer(c *OpContext) bool {
	return b.Package != InvalidLabel && b.Package.StringValue(c) == "list" &&
		(b.Name == "Sort" || b.Name == "SortStable" || b.Name == "IsSorted")
}

// NativeTemplateArgument reports a saved slot consumed as a template rather
// than as a completed data value. Its original source and environment remain
// necessary for each invocation and for faithful export.
func (f *FuncValue) NativeTemplateArgument(c *OpContext, slot int) bool {
	b := f.NativeBuiltin()
	return b != nil && slot == 1 && b.IsListComparer(c)
}

// nativeBinding retains the operation that saved native arguments, including
// its lexical environment. It is export provenance, not checking evidence.
type nativeBinding struct {
	subject *FuncValue
	env     *Environment
	call    *CallExpr
	types   []FuncType
}

// bindNativeArguments keeps every selected view at the same saved packet.
// A later identity merge may expose those views individually on export.
func (f *FuncValue) bindNativeArguments(args []funcArg, env *Environment, call *CallExpr) *FuncValue {
	copy := *f
	copy.args = args
	copy.nativeBinding = &nativeBinding{subject: f, env: env, call: call, types: f.Types}
	copy.selection, copy.projection = nil, nil
	copy.callViews = make([]*FuncValue, len(f.callViews))
	for i, view := range f.callViews {
		copy.callViews[i] = view.bindNativeArguments(args, env, call)
	}
	return &copy
}

// NativeBinding reconstructs the last native partial application and returns
// interfaces attached after that operation. Its subject retains earlier
// applications and type selections in their original order.
func (f *FuncValue) NativeBinding() (*Environment, *CallExpr, []FuncType) {
	b := f.nativeBinding
	if b == nil {
		return nil, nil, nil
	}
	call := *b.call
	subject := b.subject
	var extra []FuncType
	for _, t := range f.Types {
		if !slices.Contains(b.types, t) {
			if t.partial == nil {
				// Identity merging can recover an original full-packet
				// clause in another environment. It still belongs before
				// argument saving, even though it is absent from the local
				// snapshot. NativeBinding carries it back through earlier
				// stages until its full packet is available again.
				copy := *subject
				copy.Types = mergeFuncTypes(copy.Types, []FuncType{t})
				subject = &copy
				continue
			}
			if projected := f.nativeResidualInterface(t); projected != nil {
				// An identity merge can recover a clause from an intermediate
				// packet absent from this binding's provenance. Reconstruct
				// that branch at its original stage before saving more slots.
				extra = append(extra, FuncType{Fn: projected.Fn, Env: projected.Env, inhabitant: projected})
				continue
			}
			extra = append(extra, t)
		}
	}
	call.Fun = subject
	for _, peer := range f.identities {
		extra = append(extra, FuncType{Fn: peer.Fn, Env: peer.Env, inhabitant: peer})
	}
	return b.env, &call, extra
}

// nativeResidualInterface transports an earlier residual clause to this
// binding mask without changing the packet described by its signature.
func (f *FuncValue) nativeResidualInterface(t FuncType) *FuncValue {
	if t.partial == nil {
		return nil
	}
	call := &CallExpr{Partial: true}
	for i, param := range f.Fn.Params {
		env, expr := f.BoundArgument(i)
		if _, saved := t.partial.BoundArgument(i); saved != nil || expr == nil {
			continue
		}
		// Newly saved expressions can originate in different lexical
		// environments. Preserve each source separately during export.
		group := ConjunctGroup{MakeRootConjunct(env, expr)}
		call.Args = append(call.Args, &group)
		call.ArgLabels = append(call.ArgLabels, param.Label)
	}
	if len(call.Args) == 0 {
		return nil
	}
	subject := *t.partial
	subject.Types = mergeFuncTypes(subject.Types, []FuncType{t})
	return subject.bindNativeArguments(f.args, f.Env, call)
}

// FunctionValue exposes a native's own checking scheme to function type
// selection and packet binding. The synthetic body proves its interfaces by
// calling the primitive under arbitrary admitted inputs. Runtime calls go
// directly to the native implementation, without materializing the checking
// parameter or result descriptions as activation constraints.
func (b *Builtin) FunctionValue(c *OpContext) *FuncValue {
	if c.nativeFunctions == nil {
		c.nativeFunctions = make(map[*Builtin]*FuncValue)
	}
	origin := b.self()
	f := c.nativeFunctions[origin]
	if f == nil {
		t := origin.CheckingType(c)
		if t.Env == nil {
			t.Env = &Environment{Vertex: &Vertex{BaseValue: &StructMarker{}}}
		}
		fn := *t.Fn
		fn.Src = nil
		fn.Quantified = true
		fn.nativeOrigin = origin
		call := &CallExpr{Fun: origin.Implementation()}
		for _, p := range fn.Params {
			call.Args = append(call.Args, &FieldReference{Label: p.Local})
		}
		fn.Body = call
		f = &FuncValue{Fn: &fn, Env: t.Env, native: origin,
			Types: origin.CheckingRefinements(c)}
		for i := range f.Types {
			if f.Types[i].Env == nil {
				f.Types[i].Env = t.Env
			}
		}
		f.explicit = f.Obligations()
		c.nativeFunctions[origin] = f
	}
	copy := *f
	copy.native = b
	copy.Types = mergeFuncTypes(f.Types, b.AdditionalTypes())
	copy.nativeTypes = slices.Clone(copy.Types)
	copy.explicit = mergeFuncTypes(f.explicit, b.AdditionalTypes())
	return &copy
}

// NativeBuiltin returns the primitive retained by a function view, if any.
func (f *FuncValue) NativeBuiltin() *Builtin {
	if f.native != nil {
		return f.native
	}
	if f.Fn != nil {
		return f.Fn.nativeOrigin
	}
	return nil
}

// AdditionalNativeTypes excludes contracts restored by the native import.
func (f *FuncValue) AdditionalNativeTypes() []FuncType {
	var extra []FuncType
	for _, t := range f.Types {
		if !slices.Contains(f.nativeTypes, t) {
			extra = append(extra, t)
		}
	}
	return extra
}

func sameFunctionCode(a, b *Function) bool {
	return a == b || a != nil && b != nil && a.nativeOrigin != nil && a.nativeOrigin == b.nativeOrigin
}

func (f *FuncValue) callNative(c *OpContext, call *CallExpr, bindings []funcArg, unused *Bottom, state Flags) Value {
	if unused != nil {
		return unused
	}
	full := *call
	// The function invocation already checked attached interfaces in the
	// caller's protocol. This packet is normalized to native slot order;
	// execute the implementation without reapplying those source protocols.
	native := f.NativeBuiltin().Implementation()
	full.Fun, full.Partial, full.ArgLabels = native, false, nil
	full.Args = nil
	for i, p := range f.Fn.Params {
		arg := bindings[i]
		if arg.expr == nil {
			if p.Default == nil {
				if p.Label != InvalidLabel {
					return c.NewErrf("missing argument %s", p.Label.SelectorString(c))
				}
				return c.NewErrf("not enough arguments in function call")
			}
			arg = funcArg{expr: p.Default, env: f.Env}
		}
		value := c.newInlineVertex(nil, nil, MakeRootConjunct(arg.env, arg.expr))
		value.Finalize(c)
		full.Args = append(full.Args, value)
	}
	return native.rawCall(c, &full, state)
}
