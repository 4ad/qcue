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

func TestQuantifiedConstructorMeets(t *testing.T) {
	for _, tt := range []struct{ name, source, call, want string }{
		{"record", `f:func()->{a:1,b:true}:{a:1}&{b:true}`, `f()`, `{"a":1,"b":true}`},
		{"projection", `f:func()->1:({a:1}&{b:true}).a`, `f()`, `1`},
		{"closed", `f:func()->close({a:1,b:true}):{a:1}&{b:true}`, `f()`, `{"a":1,"b":true}`},
		{"nested", `f:func()->{r:{a:1,b:true}}:{r:{a:1}}&{r:{b:true}}`, `f()`, `{"r":{"a":1,"b":true}}`},
		{"list", `f:func()->[{a:1,b:true}]:[{a:1}]&[{b:true}]`, `f()`, `[{"a":1,"b":true}]`},
		{"argument", `f:func(r:{a:int})->{a:int,b:true}:r&{b:true}`, `f({a:2})`, `{"b":true,"a":2}`},
		{"repeated", `f:func()->{a:1,b:true,c:"c"}:({a:1}&{b:true})&{c:"c"}`, `f()`, `{"a":1,"b":true,"c":"c"}`},
		{"callback", `f:func(g:func(int)->int)->int:({a:g}&{b:true}).a(1)`, `f(func(x:int)->int:x+1)`, `2`},
		{"method_capture", `f:func(n:int)->(func()->int):({capture:_
get:func()->int:capture}&{capture:n}).get`, `f(3)()`, `3`},
		{"method_capture_reverse", `f:func(n:int)->(func()->int):({capture:n}&{capture:_
get:func()->int:capture}).get`, `f(3)()`, `3`},
		{"generic_method_capture", `Code(T)={capture:_
get:func()->T:capture}
f(A):func(n:A)->(func()->A):(Code(A)&{capture:n}).get`, `f(3)()`, `3`},
		{"nested_method_capture", `f:func(n:int)->(func()->int):({r:{capture:_
get:func()->int:capture}}&{r:{capture:n}}).r.get`, `f(3)()`, `3`},
		{"conditional", `f:func()->{r:{a:1,b:true}}:{
if true {r:{a:1}}
if true {r:{b:true}}
}`, `f()`, `{"r":{"a":1,"b":true}}`},
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
		`f:func()->string:[{a:1}]&[{b:true}]`,
		`f:func(r:{a:int})->string:r&{b:true}`,
		`f:func()->{missing:_}:{a:1}&{b:true}`,
		`f:func(r:{a:int})->close({a:int,b:true}):r&{b:true}`,
		`f:func()->string:{if true {r:{a:1}}
if true {r:{b:true}}}`,
		`f:func()->(func()->close({a:int})):({
capture:{a:1}
get:func()->close({a:int}):capture
}&{capture:{b:true}}).get`,
		`f:func(n:string)->(func()->int):({capture:_
get:func()->int:capture}&{capture:n}).get`,
		`f:func(n:int)->(func()->int):({capture:_
get:func()->int:capture}&{other:n}).get`,
		`f:func(n:int)->(func()->int):({capture:_
get:func()->int:capture}&{capture:n,bad:n+"s"}).get`,
	} {
		v := cuecontext.New().CompileString(source)
		f := v.LookupPath(cue.ParsePath("f"))
		if !f.Exists() {
			t.Fatalf("invalid source: %v", v.Err())
		}
		if err := f.Validate(cue.Concrete(true)); err == nil {
			t.Fatalf("invalid result certified: %s", source)
		}
	}
	v := cuecontext.New().CompileString(`f:func(r:close({a:1}))->string:r&{b:true}`)
	if err := v.LookupPath(cue.ParsePath("f")).Validate(cue.Concrete(true)); err != nil {
		t.Fatalf("explicit data closedness lost: %v", err)
	}
}
