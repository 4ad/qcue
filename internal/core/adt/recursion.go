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

// functionActivation records a finite structural measure independently of
// type instantiation. Once chosen, the same list slot must decrease at every
// re-entry of this code origin; alternating decreases in different slots is
// not a termination argument.
type functionActivation struct {
	fn        *FuncValue
	lengths   []int
	measure   int
	recursive bool
}

func (c *OpContext) enterFunction(fn *FuncValue, bindings []funcArg) (functionActivation, *Bottom) {
	a := functionActivation{fn: fn, measure: -1, lengths: make([]int, len(bindings))}
	for i, arg := range bindings {
		a.lengths[i] = -1
		if arg.expr == nil {
			continue
		}
		v, _ := c.Evaluate(arg.env, arg.expr)
		if v, ok := v.(*Vertex); ok && v.IsList() {
			v.Finalize(c)
			if v.Bottom() == nil && v.IsClosedList() {
				n := 0
				for range v.Elems() {
					n++
				}
				a.lengths[i] = n
			}
		}
	}
	for i := len(c.activeFunctionCalls) - 1; i >= 0; i-- {
		parent := &c.activeFunctionCalls[i]
		if parent.fn.Fn != fn.Fn {
			continue
		}
		// Calling a distinct closure already present in the caller's finite
		// capture graph traverses an existing descriptor. Creating another
		// closure at the same origin during the call does not establish that
		// descent, and still needs a decreasing list argument.
		if closureIdentity(c, parent.fn, fn) == proofRefuted && capturedFunction(c, parent.fn, fn) {
			continue
		}
		a.recursive = true
		measure := parent.measure
		if measure < 0 {
			for j, n := range a.lengths {
				if n >= 0 && j < len(parent.lengths) && n < parent.lengths[j] {
					measure = j
					break
				}
			}
		}
		if measure < 0 || measure >= len(a.lengths) || a.lengths[measure] < 0 ||
			a.lengths[measure] >= parent.lengths[measure] {
			return a, &Bottom{Src: fn.Source(), Code: StructuralCycleError,
				Err: c.Newf("recursive call has no strictly decreasing finite list argument")}
		}
		a.measure, parent.measure = measure, measure
		break
	}
	return a, nil
}

func capturedFunction(c *OpContext, outer, inner *FuncValue) bool {
	seen := make(map[Value]bool)
	functions := make(map[funcAnchorKey]bool)
	var visit func(Value) bool
	var captures func(*FuncValue) bool
	captures = func(f *FuncValue) bool {
		key := funcAnchorKey{fn: f.Fn, env: f.Env}
		if functions[key] {
			return false
		}
		functions[key] = true
		for _, ref := range f.Fn.Captures {
			v, _ := c.Evaluate(f.Env, ref)
			if visit(v) {
				return true
			}
		}
		for _, arg := range f.args {
			if arg.expr != nil {
				v, _ := c.Evaluate(arg.env, arg.expr)
				if visit(v) {
					return true
				}
			}
		}
		return false
	}
	visit = func(v Value) bool {
		if v == nil || seen[v] {
			return false
		}
		seen[v] = true
		if f, ok := Unwrap(v).(*FuncValue); ok {
			return closureIdentity(c, f, inner) == proofEstablished || captures(f)
		}
		if v, ok := v.(*Vertex); ok {
			for _, a := range v.Arcs {
				if a.ArcType == ArcMember && !a.Label.IsLet() && visit(a) {
					return true
				}
			}
		}
		return false
	}
	return captures(outer)
}

// recursiveArgument records already established ground scalar/list results
// without retaining the syntax of an earlier activation as a new recursive
// equation. Unknown values keep their original bindings and dependencies.
// Records are deliberately left alone: even a known set of fields can retain
// presence and pattern constraints that a data snapshot would discard.
func recursiveArgument(c *OpContext, value Value) Value {
	if v, ok := value.(*Vertex); ok {
		v.Finalize(c)
		if v.Bottom() != nil {
			return nil
		}
	}
	switch v := Unwrap(value).(type) {
	case *Null, *Bool, *Num, *String, *Bytes, *OpaqueValue, *FuncValue:
		return v
	case *Vertex:
		v.Finalize(c)
		if !v.IsList() || !v.IsClosedList() || v.Bottom() != nil {
			return nil
		}
		list := &ListLit{}
		for a := range v.Elems() {
			x := recursiveArgument(c, a)
			if x == nil {
				return nil
			}
			list.Elems = append(list.Elems, x)
		}
		result := c.newInlineVertex(nil, nil, MakeRootConjunct(nil, list))
		result.Finalize(c)
		return result
	}
	return nil
}
