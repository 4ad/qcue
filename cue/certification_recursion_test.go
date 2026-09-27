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

func TestQuantifiedRecursiveCertification(t *testing.T) {
	for _, tt := range []struct {
		name, source string
		valid        bool
	}{
		{"fold", `f(A, B): func(step: func(B,A)->B, seed:B, xs:[...A])->B: {
 if len(xs)==0 {out:seed}
 if len(xs)>0 {out:f(step,step(seed,xs[0]),xs[1:])}
}.out`, true},
		{"sum", `f:func(xs:[...int])->int:{
 if len(xs)==0 {out:0}
 if len(xs)>0 {out:f(xs[1:])+xs[0]}
 }.out`, true},
		{"self", `f: func(x: int) -> int: f(x)`, true},
		{"local_self", `f: func(n:int)->(func([...int])->int): {
sum:func(xs:[...int])->int:{
if len(xs)==0 {out:n}
if len(xs)>0 {out:sum(xs[1:])+xs[0]}
}.out
}.sum`, true},
		{"local_mutual", `f:func()->(func(int)->int):{
g:func(x:int)->int:h(x)
h:func(x:int)->int:g(x)
}.g`, true},
		{"local_generic", `f:func()->(forall A func(A)->A):{
g(A):func(x:A)->A:g(x)
}.g`, true},
		{"projected", `f:func()->(func(int)->int):{
g:{impl:func(x:int)->int:g(x)}.impl
}.g`, true},
		{"nested_projection", `f:func()->(func(int)->int):{
g:{r:{impl:func(x:int)->int:g(x)}}.r.impl
}.g`, true},
		{"mutual", `f: func(x: int) -> int: g(x)
g: func(x: int) -> int: f(x)`, true},
		{"generic", `f(A): func(x: A) -> A: f(x)`, true},
		{"record", `box: {g: func(x: int) -> int: f(x)}
f: func(x: int) -> int: box.g(x)`, true},
		{"callback", `apply: func(h: func(int) -> int, x: int) -> int: h(x)
f: func(x: int) -> int: apply(f, x)`, true},
		{"wrong_result", `f: func(x: int) -> string: g(x)
g: func(x: int) -> int: f(x)`, false},
		{"wrong_argument", `f: func(x: int) -> int: f("bad")`, false},
		{"wrong_callback", `apply: func(h: func(int) -> string, x: int) -> string: h(x)
f: func(x: int) -> int: apply(f, x)`, false},
		{"unchecked_sibling", `f: func(x: int) -> int: g(x)
g: func(x: int) -> int: {again: f(x), bad: x + "bad"}.again`, false},
		{"local_bad_body", `f:func()->(func(int)->int):{
g:func(x:int)->int:h(x)
h:func(x:int)->int:{again:g(x),bad:x+"bad"}.again
}.g`, false},
		{"local_data_cycle", `f:func()->int:{x:y,y:x}.x`, false},
		{"projected_bad_argument", `f:func()->(func(int)->int):{
g:{impl:func(x:int)->int:g("bad")}.impl
}.g`, false},
		{"projected_bad_sibling", `f:func(n:int)->(func(int)->int):{
g:{impl:func(x:int)->int:g(x)
bad:n+"bad"}.impl
}.g`, false},
		{"projected_data_cycle", `f:func()->int:{g:{v:g}.v}.g`, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			v := cuecontext.New().CompileString(tt.source)
			f := v.LookupPath(cue.ParsePath("f"))
			if !f.Exists() {
				t.Fatalf("invalid source: %v", v.Err())
			}
			err := f.Validate(cue.Concrete(true))
			if (err == nil) != tt.valid {
				t.Fatalf("valid=%v: %v", tt.valid, err)
			}
		})
	}
}
