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
	"strings"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/format"
)

// These forms preserve the same internal reference to the copied record.
// Check both a known capture bound and one established only by the meet.
func TestQuantifiedRecordCaptureRefinements(t *testing.T) {
	for _, field := range []string{"z:int", "z!:int", "z:_", "z?:int"} {
		for _, tt := range []struct{ name, source string }{
			{"reference", "_f: %s\nout:(_f & {z:10}).f(100,1000)"},
			{"reverse", "_f: %s\nout:({z:10} & _f).f(100,1000)"},
			{"named", "_f: %s\n_instance:_f & {z:10}\nout:_instance.f(100,1000)"},
			{"let", "_f: %s\nlet instance=_f & {z:10}\nout:instance.f(100,1000)"},
			{"embedding", "_f: %s\nout:{_f,z:10}.f(100,1000)"},
			{"literal", "out:(%s & {z:10}).f(100,1000)"},
			{"definition", "#F: %s\nout:(#F & {z:10}).f(100,1000)"},
			{"selection", "_box:{template:%s}\nout:(_box.template & {z:10}).f(100,1000)"},
			{"alias", "Code(A)=%s\nout:(Code(int) & {z:10}).f(100,1000)"},
		} {
			t.Run(field+"/"+tt.name, func(t *testing.T) {
				source := fmt.Sprintf(tt.source, "{"+field+"\nf:func(x:int,y:int)->int:x+y+z}")
				v := cuecontext.New().CompileString(source)
				semanticJSON(t, v, "out", "1110")
				if field == "z:int" || field == "z!:int" {
					if err := v.Validate(); err != nil {
						t.Fatalf("whole document: %v", err)
					}
				}
			})
		}
	}
}

func TestQuantifiedRecordCaptureScopes(t *testing.T) {
	for _, tt := range []struct{ name, source, want string }{
		{"external", `z:3
_f:{f:func(x:int)->int:x+z}
out:(_f & {z:10}).f(1)`, "4"},
		{"instances", `_f:{z!:int,f:func(x:int)->int:x+z}
a:_f & {z:10}
b:_f & {z:20}
out:[a.f(1),b.f(1)]`, "[11,21]"},
		{"nested", `_f:{r:{z!:int,f:func(x:int)->int:x+z}}
out:(_f & {r:z:10}).r.f(1)`, "11"},
		{"default", `_f:{z!:int,f:func(x:int=z)->int:x}
out:(_f & {z:10}).f()`, "10"},
		{"partial", `_f:{z!:int,f:func(x:int,y:int)->int:x+y+z}
g:(_f & {z:10}).f(100,...)
out:g(1000)`, "1110"},
		{"embedding_sibling", `out:{z:10
{f:func(x:int)->int:x+z}
}.f(1)`, "11"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			v := cuecontext.New().CompileString(tt.source)
			if err := v.Validate(); err != nil {
				t.Fatal(err)
			}
			semanticJSON(t, v, "out", tt.want)
		})
	}
}

func TestQuantifiedRecordPresenceCertification(t *testing.T) {
	for _, tt := range []struct {
		name, source string
		valid        bool
	}{
		{"required_supplied", `f:func()->close({z:int}):({z!:int}&{z:10})`, true},
		{"optional_supplied", `f:func()->close({z:int}):({z?:int}&{z:10})`, true},
		{"optional_missing", `f:func()->{z:int}:({z?:int}&{})`, false},
		{"optional_reference", `f:func()->int:{r?:{z:1},out:(r & {}).z}.out`, false},
		{"optional_selection", `f:func()->int:({r?:{z:1}} & {}).r.z`, false},
		{"optional_inventory", `f:func()->close({}):({z?:int}&{})`, false},
		{"open_inventory", `f:func()->close({z:int}):({z:10,...}&{})`, false},
		{"closed_failure", `#D:{z:int}
f:func()->string:(#D & {z:1,extra:2})`, true},
		{"closed_nested_failure", `#D:{r:{z:int}}
f:func()->string:(#D & {r:{z:1,extra:2}})`, true},
		{"closed_selection_failure", `#D:{r:{z:int}}
f:func()->string:(#D.r & {z:1,extra:2})`, true},
		{"closed_list_failure", `#D:{r:[{z:int}]}
f:func()->string:(#D & {r:[{z:1,extra:2}]})`, true},
		{"closed_embedding", `#D:{z:int}
f:func()->{z:int,extra:int}:{#D,z:1,extra:2}`, true},
		{"closed_embedding_not_failure", `#D:{z:int}
f:func()->string:{#D,z:1,extra:2}`, false},
		{"close_failure", `f:func()->string:(close({z:int}) & {z:1,extra:2})`, true},
		{"close_nonrecursive", `f:func()->{r:{z:int,extra:int}}:(close({r:{z:int}}) & {r:{z:1,extra:2}})`, true},
		{"open_definition", `#D:{r:{z:int,...}}
f:func()->{r:{z:int,extra:int}}:(#D & {r:{z:1,extra:2}})`, true},
		{"invalid_sibling", `f:func(n:int)->int:({z!:int,g:func()->int:z}&{z:n,bad:n+"s"}).g()`, false},
		{"wrong_capture", `f:func(n:string)->int:({z:_,g:func()->int:z}&{z:n}).g()`, false},
		{"missing_capture", `f:func(n:int)->int:({z:_,g:func()->int:z}&{other:n}).g()`, false},
		{"data_cycle", `f:func()->int:({a:b,b:a}&{}).a`, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			v := cuecontext.New().CompileString(tt.source)
			f := v.LookupPath(cue.ParsePath("f"))
			if !f.Exists() {
				t.Fatalf("invalid test source: %v", v.Err())
			}
			if err := f.Validate(); (err == nil) != tt.valid {
				t.Fatalf("valid=%v: %v", tt.valid, err)
			}
		})
	}
}

func TestQuantifiedRequiredCapturePropagation(t *testing.T) {
	const source = `_template:{z!:int,f:func(x:int)->int:x+z}
a:_template
b:_template
out:a.f(1)
other:b.f(1)`
	for _, mode := range []string{"direct", "source", "final"} {
		t.Run(mode, func(t *testing.T) {
			ctx := cuecontext.New()
			v := ctx.CompileString(source)
			_ = v.Validate()
			if _, err := v.LookupPath(cue.ParsePath("out")).Int64(); err == nil {
				t.Fatal("missing capture produced a result")
			}
			if mode != "direct" {
				var options []cue.Option
				if mode == "final" {
					options = append(options, cue.Final())
				}
				text, err := format.Node(v.Syntax(options...))
				if err != nil {
					t.Fatal(err)
				}
				v = ctx.CompileString(string(text))
				if err := v.Err(); err != nil {
					t.Fatalf("reimport %s: %v", text, err)
				}
			}
			for _, fill := range []bool{false, true} {
				var ready cue.Value
				if fill {
					ready = v.FillPath(cue.ParsePath("a.z"), 10)
				} else {
					ready = v.Unify(ctx.CompileString("a:z:10"))
				}
				semanticJSON(t, ready, "out", "11")
				if err := ready.LookupPath(cue.ParsePath("b.f")).Validate(cue.Concrete(true)); err == nil {
					t.Fatal("refining a supplied b's independent capture")
				}
				both := ready.FillPath(cue.ParsePath("b.z"), 20)
				semanticJSON(t, both, "out", "11")
				semanticJSON(t, both, "other", "21")
				a := both.LookupPath(cue.ParsePath("a.f"))
				b := both.LookupPath(cue.ParsePath("b.f"))
				if err := a.Unify(b).Validate(); err == nil {
					t.Fatal("different capture environments identified their closures")
				}
				conflict := ready.Unify(ctx.CompileString("a:z:>10"))
				if err := conflict.LookupPath(cue.ParsePath("out")).Validate(cue.Concrete(true)); err == nil {
					t.Fatal("computed result lost its capture demand")
				}
			}
		})
	}
}

// A required capture has a usable description before it has a supplied value.
// Lexical references and selectors must agree, and proving the body must not
// make the closure concrete or discharge the call's runtime capture demand.
func TestQuantifiedRequiredCaptures(t *testing.T) {
	for _, tt := range []struct{ name, fields, body, refinement string }{
		{"lexical", "z!:int", "x+y+z", "z:10"},
		{"selector", "config:{z!:int}", "x+y+config.z", "config:z:10"},
		{"transitive", "z!:int\nlet n=z", "x+y+n", "z:10"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := cuecontext.New()
			source := "_f:{" + tt.fields + "\nf:func(x:int,y:int)->int:" + tt.body + "}\n"
			v := ctx.CompileString(source)
			if err := v.Validate(); err != nil {
				t.Fatalf("prototype: %v", err)
			}
			f := v.LookupPath(cue.MakePath(cue.Hid("_f", "_"), cue.Str("f")))
			if err := f.Validate(cue.Concrete(true)); err == nil {
				t.Fatal("unsupplied capture made a concrete closure")
			}
			missing := ctx.CompileString(source + "bar:_f.f(100,1000)")
			if _, err := missing.LookupPath(cue.ParsePath("bar")).Int64(); err == nil {
				t.Fatal("call supplied its own missing capture")
			}
			v = ctx.CompileString(source + "bar:(_f & {" + tt.refinement + "}).f(100,1000)")
			if err := v.Validate(); err != nil {
				t.Fatal(err)
			}
			semanticJSON(t, v, "bar", "1110")
		})
	}
}

func TestQuantifiedCaptureEvidenceDoesNotInventPremises(t *testing.T) {
	for _, source := range []string{
		`z?:int; f:func()->int:z`,
		`z!:string; f:func()->int:z`,
		`z!:int; f:func()->string:z`,
		`z!:int; z!:string; f:func()->int:z`,
		`z!:int; z!:1+"s"; f:func()->int:z`,
		`z!:_; f:func()->int:z`,
	} {
		v := cuecontext.New().CompileString(strings.ReplaceAll(source, ";", "\n"))
		if !v.LookupPath(cue.ParsePath("f")).Exists() {
			t.Fatalf("invalid test source: %v", v.Err())
		}
		if err := v.Validate(); err == nil {
			t.Fatalf("unjustified capture evidence: %s", source)
		}
	}
}
