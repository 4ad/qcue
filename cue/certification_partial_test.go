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

package cue_test

import (
	"fmt"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/internal/core/adt"
	"cuelang.org/go/internal/value"
)

// A callback can implement its own declared type without belonging to the
// stronger domain required by the closure that saves it. Check certification
// before executing a call, so execution cannot supply the missing evidence.
func TestQuantifiedSavedPacketMembership(t *testing.T) {
	for _, shape := range []struct{ domain, value string }{
		{"func(int) -> 1", "cb"},
		{"{f: func(int) -> 1}", "{f: cb}"},
		{"[func(int) -> 1]", "[cb]"},
	} {
		for _, binding := range []string{"%s, ...", "h: %s, ..."} {
			for _, body := range []string{"{v: x}.v", "1"} {
				for _, contract := range []string{"", " & (func(y: int) -> int)"} {
					t.Run(shape.domain+binding+body+contract, func(t *testing.T) {
						good := body == "1"
						src := fmt.Sprintf(`
f: func(h: %s, y: int) -> int: 0
cb: func(x: int) -> int: %s
p: f(%s)%s
g: func(y: int) -> int: p(y)
`, shape.domain, body, fmt.Sprintf(binding, shape.value), contract)
						v := cuecontext.New().CompileString(src)
						for _, path := range []string{"cb", "p", "g", ""} {
							x := v.LookupPath(cue.ParsePath(path))
							if err := x.Validate(); (err == nil) != (good || path == "cb") {
								t.Fatalf("%s: static membership check: %v, good=%v", path, err, good)
							}
							if err := x.Err(); err != nil {
								_, vertex := value.ToInternal(x)
								b := adt.CombineErrors(nil, vertex.Bottom(), vertex.ChildErrors)
								if good || path == "cb" || b == nil || b.Code != adt.BlockedError {
									t.Fatalf("%s: unexpected call diagnostic: %v", path, err)
								}
							}
							if err := x.Validate(cue.Concrete(true)); (err == nil) != (good || path == "cb") {
								t.Fatalf("%s: conformance %v, good=%v", path, err, good)
							}
						}
						called := cuecontext.New().CompileString(src + "\nout: g(2)")
						data, err := called.LookupPath(cue.ParsePath("out")).MarshalJSON()
						if (err == nil) != good || good && string(data) != "0" {
							t.Fatalf("call: %s, %v", data, err)
						}
					})
				}
			}
		}
	}
}

func TestQuantifiedSavedSelectedViews(t *testing.T) {
	for _, views := range []string{"f[int] & f[string]", "f[string] & f[int]"} {
		for _, argument := range []string{"1", `"s"`} {
			for _, binding := range []string{"%s, ...", "x: %s, ..."} {
				t.Run(views+argument+binding, func(t *testing.T) {
					v := semanticValue(t, fmt.Sprintf(`
f(A): func(x: A, y: bool) -> A: x
views: %s
p: views(%s)
client: func(y: bool) -> (int | string): p(y)
out: client(true)
`, views, fmt.Sprintf(binding, argument)))
					for _, path := range []string{"p", "client"} {
						if err := v.LookupPath(cue.ParsePath(path)).Validate(cue.Concrete(true)); err != nil {
							t.Fatalf("%s certificate: %v", path, err)
						}
					}
					semanticJSON(t, v, "out", argument)
				})
			}
		}
	}
}

func TestQuantifiedPartialSourceChecking(t *testing.T) {
	for _, tt := range []struct {
		name, source, output string
		valid                bool
	}{
		{"positional", `f:func(x:int,y:int)->int:y
g:func(x:int)->(func(int)->int):f(x,...)
out:g(1)(2)`, "2", true},
		{"named", `f:func(x:int,y:int)->int:y
g:func(x:int)->(func(int)->int):f(x:x,...)
out:g(1)(2)`, "2", true},
		{"generic", `f(A):func(x:A,y:A)->A:y
g(A):func(x:A)->(func(A)->A):f[A](x,...)
out:g[int](1)(2)`, "2", true},
		{"callback", `f:func(h:func(int,int)->int)->(func(int)->int):h(1,...)
g:func()->int:f(func(x:int,y:int)->int:y)(2)
out:g()`, "2", true},
		{"bad_argument", `f:func(x:int,y:int)->int:y
g:func(x:string)->(func(int)->int):f(x,...)
out:g("bad")(1)==_|_`, "true", true},
		{"bad_result", `f:func(x:int,y:int)->int:y
g:func(x:int)->(func(int)->string):f(x,...)`, "", false},
		{"bad_label", `f:func(x:int,y:int)->int:y
g:func(x:int)->(func(int)->int):f(z:x,...)`, "", false},
		{"bad_callback", `f:func(h:func(int)->1,y:int)->int:y
cb:func(x:int)->int:x
g:func()->(func(int)->int):f(cb,...)`, "", false},
		{"invalid_generic_body", `f(A):func(x:A,y:int)->A:0
g:func()->(func(int)->int):f[int](1,...)`, "", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			v := semanticValue(t, tt.source)
			err := v.LookupPath(cue.ParsePath("g")).Validate(cue.Concrete(true))
			if (err == nil) != tt.valid {
				t.Fatalf("certificate: valid=%v: %v", tt.valid, err)
			}
			if tt.valid {
				semanticJSON(t, v, "out", tt.output)
			}
		})
	}
}
