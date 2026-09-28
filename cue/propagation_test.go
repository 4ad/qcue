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
	"cuelang.org/go/cue/format"
)

func TestLiveDescriptionPropagation(t *testing.T) {
	for _, tt := range []struct {
		name, source string
		want         int64
	}{
		{"live_input", "Live: int\nf: func(x: Live)->int:2*x\na:f(2)", 4},
		{"fixed_input", "let Fixed=int\nf:func(x:Fixed)->int:2*x\na:f(2)", 4},
		{"refined_input", "Live:int\nLive:>=0\nf:func(x:Live)->int:2*x\na:f(2)", 4},
		{"stronger_bound", "A:number\nf:func(x:A)->int:x\nA:int\na:f(2)", 2},
		{"live_identity", "R:int\nf:func(x:R)->R:x\na:f(2)", 2},
		{"live_assertion", "R:int\nf:func(x:int)->R:x&R\na:f(2)", 2},
		{"singleton_result", "R:int\nR:2\nf:func(_:int)->R:2\na:f(0)", 2},
		{"evaluation_premise", "seed:func(x:int)->int:x+1\nk:seed(2)\nshift:func(x:int)->int:x+k\nlimit:shift(1)\nf:func(x:int)->int:x&<=limit\na:f(3)", 3},
		{"capture", "factor:int\nf:func(x:int)->int:factor*x\nfactor:3\na:f(4)", 12},
		{"live_bound", "Upper:number\nchoose(A:Upper):func(x:A,y:A)->A:{if x<=y {out:x}\n if x>y {out:y}}.out\na:choose[Upper](2,5)", 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			v := cuecontext.New().CompileString(tt.source)
			a := v.LookupPath(cue.ParsePath("a"))
			got, err := a.Int64()
			if err != nil || got != tt.want {
				t.Fatalf("a = %v, %v; want %d", a, err, tt.want)
			}
			if err := a.Validate(cue.Concrete(true)); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestLiveDescriptionPendingGoals(t *testing.T) {
	for _, tt := range []struct{ name, source, path string }{
		{"domain", "A:number\nf:func(x:A)->int:x", "f"},
		{"result", "R:int\nf:func(_:int)->R:2", "f"},
		{"coverage", "Small:int\nlocal:func(x:Small)->int:x+1\nconsumer:func(g:func(int)->int)->int:g(2)\na:consumer(local)", "a"},
		{"instance_bound", "Upper:number\nf(A:Upper):func(x:A)->A:x\na:f[int&>=0](2)", "a"},
		{"proof_cycle", "k:f(0)\nf:func(x:int)->int:k\na:f(1)", "a"},
		{"strict_unknown", "f:func(_:int)->int:7\na:f(_)", "a"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			v := cuecontext.New().CompileString(tt.source).LookupPath(cue.ParsePath(tt.path))
			if err := v.Validate(cue.Concrete(true)); err == nil {
				t.Fatalf("unresolved goal was accepted: %v", v)
			}
		})
	}
}

func TestPropagationLaterRefinement(t *testing.T) {
	for _, tt := range []struct {
		name, source, refinement string
		want                     int64
		fails                    bool
	}{
		{"bound", "A:number\nf:func(x:A)->int:x\na:f(2)", "A:int", 2, false},
		{"result", "R:int\nf:func(_:int)->R:2\na:f(0)", "R:2", 2, false},
		{"input_allowed", "Input:int\nf:func(x:Input)->int:2*x\na:f(2)", "Input:>=0", 4, false},
		{"let_excluded", "Input:int\nf:func(x:Input)->int:2*x\nlet call=f(2)\na:call", "Input:>2", 0, true},
		{"hidden_excluded", "_input:int\nf:func(x:_input)->int:2*x\na:f(2)", "_input:>2", 0, true},
		{"list_projection_excluded", "Input:int\nf:func(x:Input)->int:2*x\na:[f(2)][0]", "Input:>2", 0, true},
		{"embedded_excluded", "Input:int\nf:func(x:Input)->int:2*x\na:{f(2)}", "Input:>2", 0, true},
		{"projection_excluded", "Input:int\nf:func(x:Input)->int:2*x\na:{value:f(2)}.value", "Input:>2", 0, true},
		{"input_excluded", "Input:int\nf:func(x:Input)->int:2*x\na:f(2)", "Input:>2", 0, true},
		{"ignored_input_excluded", "Input:int\nf:func(x:Input)->int:7\na:f(2)", "Input:>2", 0, true},
		{"output_allowed", "Output:int\nf:func(x:int)->int:2*x\na:f(2)&Output", "Output:>=4", 4, false},
		{"output_excluded", "Output:int\nf:func(x:int)->int:2*x\na:f(2)&Output", "Output:<4", 0, true},
		{"capture", "factor:int\nf:func(x:int)->int:factor*x\na:f(4)", "factor:3", 12, false},
		{"universal_bound", "Upper:number\nf(A:Upper):func(x:A)->A:x\na:f[Upper](2)", "Upper:int", 2, false},
		{"result_excluded", "R:int\nf:func(_:int)->R:2\na:f(0)", "R:>2", 0, true},
	} {
		for _, mode := range []string{"direct", "source", "final"} {
			name := tt.name + "/" + mode
			t.Run(name, func(t *testing.T) {
				ctx := cuecontext.New()
				v := ctx.CompileString(tt.source)
				// Observe before refinement to exercise caches and residuals.
				_ = v.Validate()
				_, _ = v.LookupPath(cue.ParsePath("a")).Int64()
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
						t.Fatalf("export %s: %v", text, err)
					}
				}
				v = v.Unify(ctx.CompileString(tt.refinement))
				a := v.LookupPath(cue.ParsePath("a"))
				got, err := a.Int64()
				if tt.fails {
					if a.Validate(cue.Concrete(true)) == nil {
						t.Fatalf("lost live constraint: %v", a)
					}
				} else if err != nil || got != tt.want {
					t.Fatalf("after %s: a = %v, %v; want %d", tt.refinement, a, err, tt.want)
				}
			})
		}
	}
}

func TestPropagationObservationRefinement(t *testing.T) {
	const source = "limit:int\nf:func(x:int & >limit & <10)->_|_:_|_"
	for _, exported := range []bool{false, true} {
		ctx := cuecontext.New()
		v := ctx.CompileString(source)
		if err := v.LookupPath(cue.ParsePath("f")).Validate(); err == nil {
			t.Fatal("an unresolved domain guard discharged relevance")
		}
		if exported {
			text, err := format.Node(v.Syntax())
			if err != nil {
				t.Fatal(err)
			}
			v = ctx.CompileString(string(text))
		}
		for _, limit := range []int{10, 0} {
			r := v.FillPath(cue.ParsePath("limit"), limit)
			err := r.LookupPath(cue.ParsePath("f")).Validate()
			if (err == nil) != (limit == 10) {
				t.Fatalf("export=%v limit=%d: relevance: %v", exported, limit, err)
			}
		}
	}
}

func TestPropagationPacketIsolation(t *testing.T) {
	for _, call := range []string{"f(arg)", "f(arg, ...)()", "f(x: arg)"} {
		for _, body := range []string{"x", "0"} {
			ctx := cuecontext.New()
			v := ctx.CompileString("arg:{}\nf:func(x:{a:1})->_:" + body + "\nout:" + call)
			if err := v.LookupPath(cue.ParsePath("out")).Validate(cue.Concrete(true)); err != nil {
				t.Fatalf("%s returning %s: %v", call, body, err)
			}
			if v.LookupPath(cue.ParsePath("arg.a")).Exists() {
				t.Fatal("packet completion wrote back into its source")
			}
			for _, a := range []int{1, 2} {
				r := v.FillPath(cue.ParsePath("arg.a"), a)
				err := r.LookupPath(cue.ParsePath("out")).Validate(cue.Concrete(true))
				if (err == nil) != (a == 1) {
					t.Fatalf("%s returning %s, later arg.a=%d: %v", call, body, a, err)
				}
			}
		}
	}
}

// Every source below retains its old regression: a contradictory packet must
// fail. The packet meet is now an executable constraint, so the partial body's
// result theorem can be proved independently of that demanded failure.
func TestPropagationContradictoryPackets(t *testing.T) {
	for _, tt := range []struct{ source, call string }{
		{`let identity = func(x: int) -> int: x
f: func(x: string) -> int: identity(x)`, `f("bad")`},
		{`helper: func(x: int) -> int: x
f: func() -> string: _|_ & helper("wrong")`, `f()`},
		{`f: func(r: {a: func(int) -> 1} | {a: func(string) -> 2}) -> int: r.a(3)`, `f({a:func(x:string)->2:2})`},
		{`f:func(x:int)->int:f("bad")`, `f(1)`},
		{`f:func()->(func(int)->int):{g:{impl:func(x:int)->int:g("bad")}.impl}.g`, `f()(1)`},
	} {
		v := cuecontext.New().CompileString(tt.source + "\nout:" + tt.call)
		if err := v.LookupPath(cue.ParsePath("f")).Validate(cue.Concrete(true)); err != nil {
			t.Fatalf("partial body: %v\n%s", err, tt.source)
		}
		if err := v.LookupPath(cue.ParsePath("out")).Validate(); err == nil {
			t.Fatalf("contradictory packet completed: %s", tt.call)
		}
	}
}
