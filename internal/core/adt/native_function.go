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
	native := f.NativeBuiltin()
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
