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

type identityResult uint8

const (
	identityPending identityResult = iota
	identityEqual
	identityConflict
)

// closureIdentity compares operational descriptors, not the functions they
// compute. Unknown captures retain an equality obligation: comparing two
// upper approximations is not evidence that their witnesses are equal.
func closureIdentity(c *OpContext, a, b *FuncValue) identityResult {
	if a.Fn != b.Fn {
		return identityConflict
	}
	result := identityEqual
	compare := func(x Expr, xe *Environment, y Expr, ye *Environment) {
		if x == y && xe == ye {
			return
		}
		xv, _ := c.Evaluate(xe, x)
		yv, _ := c.Evaluate(ye, y)
		if !concreteCapture(c, xv) || !concreteCapture(c, yv) {
			if result != identityConflict {
				result = identityPending
			}
			return
		}
		if !Equal(c, xv, yv, 0) {
			result = identityConflict
		}
	}
	if a.Env != b.Env {
		for _, x := range a.Fn.Captures {
			compare(x, a.Env, x, b.Env)
		}
	}
	for i := range a.Fn.Params {
		var x, y funcArg
		if i < len(a.args) {
			x = a.args[i]
		}
		if i < len(b.args) {
			y = b.args[i]
		}
		if (x.expr == nil) != (y.expr == nil) {
			return identityConflict
		}
		if x.expr != nil {
			compare(x.expr, x.env, y.expr, y.env)
		}
	}
	return result
}

func concreteCapture(c *OpContext, v Value) bool {
	seen := make(map[Value]bool)
	functions := make(map[funcAnchorKey]bool)
	var check func(Value) bool
	check = func(v Value) bool {
		if v == nil || seen[v] {
			return false
		}
		seen[v] = true
		defer delete(seen, v)
		if x, ok := v.(*Vertex); ok {
			x.Finalize(c)
			if !IsConcrete(x) || x.Bottom() != nil {
				return false
			}
			for _, a := range x.Arcs {
				if a.ArcType == ArcRequired {
					return false
				}
				if a.ArcType == ArcMember && !a.Label.IsLet() && !check(a) {
					return false
				}
			}
			if unwrapped := Unwrap(x); unwrapped != x {
				return check(unwrapped)
			}
			return true
		}
		if f, ok := v.(*FuncValue); ok {
			key := funcAnchorKey{fn: f.Fn, env: f.Env}
			if IsFuncType(f) || functions[key] || len(f.identities) != 0 {
				return false
			}
			functions[key] = true
			defer delete(functions, key)
			for _, x := range f.Fn.Captures {
				v, _ := c.Evaluate(f.Env, x)
				if !check(v) {
					return false
				}
			}
			for _, a := range f.args {
				if a.expr != nil {
					v, _ := c.Evaluate(a.env, a.expr)
					if !check(v) {
						return false
					}
				}
			}
		}
		if _, bottom := v.(*Bottom); bottom {
			return false
		}
		return IsConcrete(v)
	}
	return check(v)
}

func mergeClosureIdentities(c *OpContext, a, b *FuncValue) (*FuncValue, *Bottom) {
	result := closureIdentity(c, a, b)
	if result == identityConflict {
		return nil, c.NewErrf("conflicting function identities")
	}
	if err := checkFuncTypeSetsMeet(c, a.Types, b.Types); err != nil {
		return nil, err
	}
	m := *a
	m.Types = mergeFuncTypes(a.Types, b.Types)
	m.identities = slices.Clone(a.identities)
	if result == identityPending && !slices.Contains(m.identities, b) {
		m.identities = append(m.identities, b)
	}
	for _, p := range b.identities {
		if p != a && !slices.Contains(m.identities, p) {
			m.identities = append(m.identities, p)
		}
	}
	return &m, nil
}

func (x *FuncValue) checkIdentities(c *OpContext) *Bottom {
	for _, peer := range x.identities {
		switch closureIdentity(c, x, peer) {
		case identityConflict:
			return c.NewErrf("conflicting function identities")
		case identityPending:
			return &Bottom{Code: IncompleteError,
				Err: c.Newf("incomplete equality of captured values or partial arguments")}
		}
	}
	return nil
}
