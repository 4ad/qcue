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

func TestQuantifiedInstantiation(t *testing.T) {
	for _, tt := range []struct{ name, src, want string }{
		{"independent calls", `
id(A): func(x: A) -> A: x
out: [id(3), id("hello"), id(true)]`, `[3,"hello",true]`},
		{"explicit arguments", `
id: func<A>(x: A) -> A: x
out: [id[int](3), id[string]("hello")]`, `[3,"hello"]`},
		{"repeated parameter", `
pair(A: number): func(x: A, y: A) -> [A, A]: [x, y]
out: pair(1, 2.5)`, `[1,2.5]`},
		{"tuple variables", `
swap(A, B): func(p: [A, B]) -> [B, A]: [p[1], p[0]]
out: swap([1, "x"])`, `["x",1]`},
		{"nested result", `
constant(A): func(x: A) -> (forall B func(B) -> A):
    func<B>(y: B) -> A: x
out: [constant(3)("x"), constant("hi")(false)]`, `[3,"hi"]`},
		{"shadowing", `
f(A): func(x: A) -> _: {g: func<A>(y: A) -> A: y, x: x}
out: [f(3).g("x"), f("x").g(3)]`, `["x",3]`},
		{"generic map", `
map(A, B): func(g: func(A) -> B, xs: [...A]) -> [...B]: [
    for x in xs {g(x)}
]
inc: func(x: int) -> int: x + 1
text: func(x: int) -> string: "\(x)"
out: [map(inc, [1, 2, 3]), map(text, [1, 2, 3])]`, `[[2,3,4],["1","2","3"]]`},
		{"generic callback", `
flatMap(A, B): func(g: func(A) -> [...B], xs: [...A]) -> [...B]: [
    for x in xs for y in g(x) {y}
]
duplicate(A): func(x: A) -> [A, A]: [x, x]
out: flatMap(duplicate, [1, 2])`, `[1,1,2,2]`},
		{"composition", `
compose(A, B, C): func(g: func(B) -> C, f: func(A) -> B) ->
    func(A) -> C: func(x: A) -> C: g(f(x))
inc: func(x: int) -> int: x + 1
text: func(x: int) -> string: "\(x)"
out: compose(text, inc)(4)`, `"5"`},
		{"higher rank callback", `
id(A): func(x: A) -> A: x
useBoth: func(p: forall A func(A) -> A) -> [int, string]: [
    p(7), p("seven"),
]
out: useBoth(id)`, `[7,"seven"]`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			v := cuecontext.New().CompileString("@experiment(quantified)\n" + tt.src)
			got, err := v.LookupPath(cue.MakePath(cue.Str("out"))).MarshalJSON()
			if err != nil {
				t.Fatalf("%v (root: %v)", err, v.Err())
			}
			if string(got) != tt.want {
				t.Fatalf("got %s; want %s", got, tt.want)
			}
		})
	}
}

func TestQuantifiedUniversalRefutations(t *testing.T) {
	for _, src := range []string{
		`bad(A): func(x: A) -> A: 0`,
		`bad(A: number): func(x: A) -> A: x + 1`,
		`bad(A): func(A) -> A
         bad: func(int) -> bool`,
		`bad: forall A func(A) -> A
         bad: func(x: int) -> int: x`,
	} {
		t.Run(src, func(t *testing.T) {
			v := cuecontext.New().CompileString("@experiment(quantified)\n" + src)
			if err := v.LookupPath(cue.MakePath(cue.Str("bad"))).Err(); err == nil {
				t.Fatal("missing universal counterexample")
			}
		})
	}
}

func TestQuantifiedInvalidInstances(t *testing.T) {
	for _, src := range []string{
		`id(A): func(x: A) -> A: x
         out: id[int]("wrong")`,
		`pair(A: number): func(x: A, y: A) -> [A, A]: [x, y]
         out: pair[string]("a", "b")`,
		`id(A in int): func(x: int) -> int: x
         out: id(1)`,
	} {
		t.Run(src, func(t *testing.T) {
			v := cuecontext.New().CompileString("@experiment(quantified)\n" + src)
			if err := v.Validate(); err == nil {
				t.Fatal("invalid instance was accepted")
			}
		})
	}
}

// The paper's self-unification examples exercise descriptor identity across
// copied environments, not just references to the same evaluator pointer.
func TestQuantifiedClosureIdentity(t *testing.T) {
	for _, tt := range []struct{ name, src, path, want string }{
		{"copying", `
x: {f: func(y: int) -> int: y}
a: x.f & x.f
b: x.f & {x}.f
c: x.f & (x & {g: 1}).f
r: [a(2), b(2), c(2)]
`, "r", `[2,2,2]`},
		{"capture copying", `
x: {n: 3, f: func(y: int) -> int: y + n}
f: x.f & (x & {g: 1}).f
r: f(2)
`, "r", `5`},
		{"unrelated captures", `
x: {unused: int, f: func(y: int) -> int: y}
f: (x & {unused: 1}).f & (x & {unused: 2}).f
r: f(2)
`, "r", `2`},
		{"normalized partial arguments", `
add: func(a: int, b: int) -> int: a + b
f: add(1, ...) & add(a: 1, ...)
r: f(2)
`, "r", `3`},
		{"explicit result combination", `
left: func(x: int) -> {a: int}: {a: x}
right: func(x: int) -> {b: int}: {b: x}
both: func(x: int) -> {a: int, b: int}: left(x) & right(x)
r: both(3)
`, "r", `{"a":3,"b":3}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			v := cuecontext.New().CompileString("@experiment(quantified)\n" + tt.src)
			r := v.LookupPath(cue.ParsePath(tt.path))
			got, err := r.MarshalJSON()
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Fatalf("got %s; want %s", got, tt.want)
			}
		})
	}
}

func TestQuantifiedDistinctClosures(t *testing.T) {
	for _, src := range []string{
		`a: func(x: int) -> int: x
         b: func(x: int) -> int: x
         out: a & b`,
		`d: [for v in [1, 2] {func(y: int) -> int: y + v}]
         out: d[0] & d[1]`,
		`f: func(a: int, b: int) -> int: a + b
         out: f(1, ...) & f(2, ...)`,
	} {
		t.Run(src, func(t *testing.T) {
			v := cuecontext.New().CompileString("@experiment(quantified)\n" + src)
			if err := v.LookupPath(cue.MakePath(cue.Str("out"))).Err(); err == nil {
				t.Fatal("distinct operational descriptors did not conflict")
			}
		})
	}
}

func TestQuantifiedCaptureRefinement(t *testing.T) {
	ctx := cuecontext.New()
	v := ctx.CompileString(`
@experiment(quantified)
let S = {n: int, f: func(x: int = n) -> int: 1}
a: S
b: S
f: a.f & b.f
out: f(0)
`)
	f := v.LookupPath(cue.MakePath(cue.Str("f")))
	if err := f.Err(); err != nil {
		t.Fatalf("unknown capture equality was refuted: %v", err)
	}
	if err := f.Validate(cue.Concrete(true)); err == nil {
		t.Fatal("unknown capture equality was certified")
	}
	out := v.LookupPath(cue.MakePath(cue.Str("out")))
	if _, err := out.Int64(); err == nil {
		t.Fatal("call lost its capture equality guard")
	}
	for _, tt := range []struct {
		refinement string
		conflict   bool
	}{
		{`a: n: 2, b: n: 2`, false},
		{`a: n: 2, b: n: 3`, true},
	} {
		r := v.Unify(ctx.CompileString(tt.refinement))
		r = r.LookupPath(cue.MakePath(cue.Str("out")))
		if tt.conflict {
			if err := r.Validate(); err == nil {
				t.Fatal("different concrete captures did not conflict")
			}
		} else if got, err := r.Int64(); err != nil || got != 1 {
			t.Fatalf("equal refined captures: got %d, %v", got, err)
		}
	}
}

func TestQuantifiedCapabilities(t *testing.T) {
	for _, tt := range []struct {
		name, src, want string
	}{
		{"contravariant coverage", `
narrow: func(int) -> string
narrow: func(x: number) -> string: "ok"
out: [narrow(1), narrow(1.5)]`, `["ok","ok"]`},
		{"overlapping results", `
f: func(x: int) -> number: 1
f: func(number) -> int
out: f(3)`, ``}, // The wider domain must be rejected despite the constant body.
		{"body refines approximation", `
f: func(x: number) -> number: 1
f: func(number) -> int
out: f(3)`, `1`},
		{"guarded output clauses", `
classify: func(x: int) -> ("negative" | "nonnegative"): {
    if x < 0 {out: "negative"}
    if x >= 0 {out: "nonnegative"}
}.out
classify: (func(int & <0) -> "negative") & (func(int & >=0) -> "nonnegative")
out: [classify(-3), classify(2)]`, `["negative","nonnegative"]`},
		{"default belongs to closure", `
f: func(x: int = 20) -> int: x
f: func(int = 10) -> int
out: f()`, `20`},
		{"additional optional slot", `
f: func(x: int, y: int = 1) -> int: y
f: func(int) -> 1
out: [f(0), f(0, 3)]`, `[1,3]`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			v := cuecontext.New().CompileString("@experiment(quantified)\n" + tt.src)
			out := v.LookupPath(cue.MakePath(cue.Str("out")))
			if tt.want == "" {
				if err := out.Err(); err == nil {
					t.Fatal("missing capability conflict")
				}
				return
			}
			got, err := out.MarshalJSON()
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Fatalf("got %s, want %s", got, tt.want)
			}
		})
	}
}

func TestQuantifiedCapabilityRefutations(t *testing.T) {
	for _, src := range []string{
		`f: func(number) -> string
         f: func(x: int) -> string: "ok"`,
		`f: func(int) -> string
         f: func(x: int) -> int: x`,
		`f: (func(int) -> int) & (func(int) -> bool)`,
		`f: func(int = 1) -> int
         f: func(x: int) -> int: x`,
		`f: func(a: int) -> int
         f: func(b: int) -> int: b`,
	} {
		t.Run(src, func(t *testing.T) {
			v := cuecontext.New().CompileString("@experiment(quantified)\n" + src)
			if err := v.LookupPath(cue.MakePath(cue.Str("f"))).Err(); err == nil {
				t.Fatal("concrete counterexample did not refute capability")
			}
		})
	}
	for _, src := range []string{
		`f: func(number) -> number
         f: func(number) -> int`,
		`f: (func(int) -> string) & (func(string) -> int)`,
		`f: (func(_|_) -> int) & (func(_|_) -> string)`,
		`x: number
         f: func(x) -> string
         f: func(n: int) -> string: "ok"`,
	} {
		t.Run(src, func(t *testing.T) {
			v := cuecontext.New().CompileString("@experiment(quantified)\n" + src)
			if err := v.LookupPath(cue.MakePath(cue.Str("f"))).Err(); err != nil {
				t.Fatalf("unrefuted capability was rejected: %v", err)
			}
		})
	}
}

func TestQuantifiedCapabilityGuardRefinement(t *testing.T) {
	ctx := cuecontext.New()
	v := ctx.CompileString(`
@experiment(quantified)
f: func(x: int) -> int: x
f: func(0) -> 0
y: int
out: f(y)
`)
	if err := v.LookupPath(cue.MakePath(cue.Str("f"))).Err(); err != nil {
		t.Fatal(err)
	}
	for _, n := range []int{0, 1, 2} {
		r := v.FillPath(cue.MakePath(cue.Str("y")), n)
		got, err := r.LookupPath(cue.MakePath(cue.Str("out"))).Int64()
		if err != nil || got != int64(n) {
			t.Fatalf("refining y to %d: got %d, %v", n, got, err)
		}
	}
}
