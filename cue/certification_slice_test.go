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
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
)

func TestQuantifiedSliceCertification(t *testing.T) {
	for _, tt := range []struct{ name, source, call, want string }{
		{"tail", `f(A): func(xs: [...A]) -> [...A]: xs[1:]`, `f([1,2,3])`, `[2,3]`},
		{"fixed", `f: func(xs: [1,"two",true]) -> ["two",true]: xs[1:]`, `f([1,"two",true])`, `["two",true]`},
		{"range", `f(A): func(xs: [...A]) -> [A,A]: xs[1:3]`, `f([1,2,3])`, `[2,3]`},
		{"dynamic", `f(A): func(xs: [...A], start:int, end:int) -> [...A]: xs[start:end]`, `f([1,2,3],1,2)`, `[2]`},
		{"bytes", `f: func(xs: bytes) -> bytes: xs[1:]`, `f('abc') == 'bc'`, `true`},
		{"bytes_literal", `f: func() -> 'bc': 'abc'[1:]`, `f() == 'bc'`, `true`},
		{"callback", `f: func(xs:[func(int)->int]) -> int: xs[:][0](1)`, `f([func(x:int)->int:x+1])`, `2`},
		{"homogeneous_callback", `f: func(xs:[...func(int)->int]) -> int: xs[1:][0](1)`, `f([func(x:int)->int:x,func(x:int)->int:x+1])`, `2`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			v := cuecontext.New().CompileString(tt.source + "\nout: " + tt.call)
			if err := v.LookupPath(cue.ParsePath("f")).Validate(cue.Concrete(true)); err != nil {
				t.Fatal(err)
			}
			semanticJSON(t, v, "out", tt.want)
		})
	}
	for _, source := range []string{
		`f: func(xs:[1]) -> string: xs[2:]`,
		`f: func(xs:[1]) -> string: xs[1:0]`,
		`f: func(xs:[1]) -> string: xs[-1:]`,
	} {
		v := cuecontext.New().CompileString(source)
		if err := v.LookupPath(cue.ParsePath("f")).Validate(cue.Concrete(true)); err != nil {
			t.Fatalf("checked failure: %s: %v", source, err)
		}
	}
	for _, source := range []string{
		`f: func(xs:[1]) -> _: xs["bad":]`,
		`f: func(xs:int) -> _: xs[0:]`,
		`f: func(xs:[1]) -> [true]: xs[:]`,
		`Ignore(A)=1
f: func()->1:Ignore([1]["bad":])`,
	} {
		v := cuecontext.New().CompileString(source)
		f := v.LookupPath(cue.ParsePath("f"))
		if !f.Exists() {
			t.Fatalf("invalid source: %v", v.Err())
		}
		if err := f.Validate(cue.Concrete(true)); err == nil {
			t.Fatalf("invalid slice certified: %s", source)
		}
	}
}
