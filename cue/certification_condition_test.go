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

func TestQuantifiedConditionalCertification(t *testing.T) {
	for _, tt := range []struct{ name, source, call, want string }{
		{"boolean", `f:func(b:bool)->int:{if b {out:1}
 if !b {out:2}}.out`, `f(true)`, `1`},
		{"else", `f:func(b:bool)->int:{if b {out:1} else {out:2}}.out`, `f(false)`, `2`},
		{"numeric", `f:func(n:int)->int:{if n<=0 {out:1}
 if n>0 {out:2}}.out`, `f(-1)`, `1`},
		{"two_parameters", `f(A:number):func(x:A,y:A)->A:{if x<=y {out:x}
 if x>y {out:y}}.out`, `f[int&>=10](12,20)`, `12`},
		{"parameter_equality", `f:func(x:int,y:int)->bool:{if x==y {out:true}
 if x!=y {out:false}}.out`, `f(1,2)`, `false`},
		{"float_equality", `f:func(n:number)->int:{if n==0 {out:1}
 if n!=0 {out:2}}.out`, `f(0.0)`, `1`},
		{"length", `f:func(xs:[...int])->int:{if len(xs)==0 {out:0}
 if len(xs)>0 {out:xs[0]}}.out`, `f([2])`, `2`},
		{"ordinary_field", `f:func(b:bool)->int:{base:1
 if b {out:base}
 if !b {out:base+1}}.out`, `f(false)`, `2`},
		{"nested", `f:func(b:bool,c:bool)->int:{if b {if c {out:1} else {out:2}} else {out:3}}.out`, `f(true,false)`, `2`},
		{"optional_present", `f:func(x?:int)->int:{if x!=_|_ {out:x}
 if x==_|_ {out:0}}.out`, `f(x:3)`, `3`},
		{"optional_absent", `f:func(x?:int)->int:{if x!=_|_ {out:x}
 if x==_|_ {out:0}}.out`, `f()`, `0`},
		{"optional_reversed", `f:func(x?:int)->int:{if _|_!=x {out:x}
 if _|_==x {out:0}}.out`, `f(x:3)`, `3`},
		{"optional_negated", `f:func(x?:int)->int:{if !(x==_|_) {out:x} else {out:0}}.out`, `f(x:3)`, `3`},
		{"optional_closure", `f:func(x?:int)->(func()->int):{
 if x!=_|_ {out:func()->int:x}
 if x==_|_ {out:func()->int:0}}.out`, `f(x:3)()`, `3`},
		{"optional_callback", `f:func(g?:func(int)->int)->int:{
 if g!=_|_ {out:g(3)}
 if g==_|_ {out:0}}.out`, `f(g:func(x:int)->int:x)`, `3`},
		{"optional_nested", `f:func(x?:int,y?:int)->int:{
 if x!=_|_ {out:x}
 if x==_|_ {if y!=_|_ {out:y} else {out:0}}}.out`, `f(y:4)`, `4`},
		{"optional_checked_failure", `f:func(x?:int)->int:{
 if x!=_|_ {out:x}
 if x==_|_ {out:_|_}}.out`, `f()==_|_`, `true`},
		{"conditional_failure", `f:func(b:bool)->int:{if b {out:1} else {_|_}}.out`, `f(true)`, `1`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			v := cuecontext.New().CompileString("@experiment(try)\n" + tt.source + "\nout: " + tt.call)
			if err := v.LookupPath(cue.ParsePath("f")).Validate(cue.Concrete(true)); err != nil {
				t.Fatal(err)
			}
			semanticJSON(t, v, "out", tt.want)
		})
	}
	for _, source := range []string{
		`f:func(b:bool)->int:{if b {out:1}}.out`,
		`f:func(x:int,y:int)->int:{if x<=0 {out:1}
 if y>0 {out:2}}.out`,
		`f:func(x:int,y:int,z:int)->int:{if x<=y {out:1}
 if x>z {out:2}}.out`,
		`f:func(x:int,y:int)->int:{if x<y {out:1}
 if x>y {out:2}}.out`,
		`f:func(xs:[...int],ys:[...int])->int:{if len(xs)==0 {out:1}
 if len(ys)>0 {out:2}}.out`,
		`f:func(n:int)->int:{if n<0 {out:1}
 if n>0 {out:2}}.out`,
		`f:func(n:int)->int:{if n {out:1} else {out:2}}.out`,
		`f:func(n:int)->int:{if true {out:1} else {out:n+"bad"}}.out`,
		`f:func(n:int)->int:{if true {out:1}
 if false {bad:n+"bad"}}.out`,
		`f:func(n:int)->int:{if n<0 {out:1}
if n>=0 {out:"bad"}}.out`,
		`f:func(r:{a:int},b:bool)->close({a:int}):{if b {r} else {r}}`,
		`f:func(x?:int,y?:int)->int:{if x!=_|_ {out:y} else {out:0}}.out`,
		`f:func(x?:int)->int:{if x==_|_ {out:x} else {out:0}}.out`,
		`f:func(x?:int)->int:{if x!=_|_ {out:x} else {out:x}}.out`,
		`f:func(x?:int)->int:{if x!=_|_ {checked:x}
 out:x}.out`,
		`f:func(x?:int)->(func()->int):{outside:func()->int:x
 if x!=_|_ {out:outside} else {out:func()->int:0}}.out`,
		`f:func(x?:int,y?:string)->_:({if x!=_|_ {r:x}, if y!=_|_ {r:y}}).r`,
	} {
		v := cuecontext.New().CompileString("@experiment(try)\n" + source)
		f := v.LookupPath(cue.ParsePath("f"))
		if !f.Exists() {
			t.Fatalf("invalid source: %v", v.Err())
		}
		if err := f.Validate(cue.Concrete(true)); err == nil {
			t.Fatalf("unchecked conditional certified: %s", source)
		}
	}
}
