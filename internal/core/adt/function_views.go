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

func mergeCallViews(a, b *FuncValue) []*FuncValue {
	var views []*FuncValue
	for _, f := range []*FuncValue{a, b} {
		incoming := f.callViews
		if len(incoming) == 0 {
			incoming = []*FuncValue{f}
		}
		for _, v := range incoming {
			if !slices.Contains(views, v) {
				views = append(views, v)
			}
		}
	}
	return views
}

func equalCallViews(a, b []*FuncValue) bool {
	if len(a) != len(b) {
		return false
	}
	for _, x := range a {
		if !slices.ContainsFunc(b, func(y *FuncValue) bool {
			return x.Fn == y.Fn && x.Env == y.Env &&
				equalFuncTypes(x.Types, y.Types) &&
				equalFuncTypes(x.selectionClauses(), y.selectionClauses())
		}) {
			return false
		}
	}
	return true
}

// CallViews exposes a conjunction of checked views without duplicating their
// code origin on source export. Later contracts remain additional conjuncts.
func (f *FuncValue) CallViews() (views []*FuncValue, extra []FuncType) {
	views = f.callViews
	var entailed []FuncType
	for _, view := range views {
		entailed = mergeFuncTypes(entailed, view.selectionAndOriginalClauses())
	}
	for _, t := range f.Types {
		if !slices.Contains(entailed, t) {
			extra = append(extra, t)
		}
	}
	return views, extra
}

// admittedCallView chooses a proof view, not an implementation: every view
// shares the same code and packet protocol. The body is executed once, and
// every applicable view contributes its result obligation. Unknown admission
// cannot be discarded as disjointness.
func (f *FuncValue) admittedCallView(c *OpContext, bindings []funcArg) (*FuncValue, *Bottom) {
	var admitted []*FuncValue
	unknown := false
	for _, view := range f.callViews {
		admission, applies := (callPacket{args: bindings}).admit(c, FuncType{Fn: view.Fn, Env: view.Env}, false)
		switch applies {
		case proofEstablished:
			copy := *view
			copy.Env = admission.clause.Env
			admitted = append(admitted, &copy)
		case proofUnknown:
			unknown = true
		}
	}
	if unknown {
		return nil, &Bottom{Code: IncompleteError, Err: c.Newf("selected call view admission remains unresolved")}
	}
	if len(admitted) == 0 {
		return nil, c.NewErrf("no selected instance admits this call packet")
	}
	copy := *f
	copy.Env = admitted[0].Env
	copy.callViews = nil
	copy.Types = slices.Clone(f.Types)
	for _, view := range admitted[1:] {
		fn := *view.Fn
		fn.Body = nil
		copy.Types = append(copy.Types, FuncType{Fn: &fn, Env: view.Env})
	}
	return &copy, nil
}
