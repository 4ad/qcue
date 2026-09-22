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
	"cuelang.org/go/cue/build"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/format"
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

func TestQuantifiedDataMeet(t *testing.T) {
	ctx := cuecontext.New()
	v := ctx.CompileString(`
@experiment(quantified)
empty(A): [...A]
impossible(A): A
impossibleRecord(A): {value: A}
optional(A): {value?: A}
union: forall A (A | int)
`)
	for _, path := range []string{"impossible", "impossibleRecord"} {
		if err := v.LookupPath(cue.ParsePath(path)).Validate(); err == nil {
			t.Errorf("%s: missing empty-instance refutation", path)
		}
	}
	for path, want := range map[string]string{"empty": `[]`, "optional": `{}`} {
		got, err := v.LookupPath(cue.ParsePath(path)).MarshalJSON()
		if err != nil || string(got) != want {
			t.Errorf("%s: got %s, %v; want %s", path, got, err, want)
		}
	}
	if err := v.LookupPath(cue.ParsePath("union")).Unify(ctx.CompileString(`"x"`)).Err(); err == nil {
		t.Fatal("universal union lost its empty instance")
	}
}

func TestQuantifiedAbstractCall(t *testing.T) {
	ctx := cuecontext.New()
	v := ctx.CompileString(`
@experiment(quantified)
f(A): func(A) -> A
r: f(3)
bad: f(3) & string
`)
	if err := v.LookupPath(cue.ParsePath("bad")).Err(); err == nil {
		t.Fatal("symbolic call failed to propagate its result constraint")
	}
	if _, err := v.LookupPath(cue.ParsePath("r")).MarshalJSON(); err == nil {
		t.Fatal("an arrow hypothesis materialized a result without execution")
	}
	v = v.Unify(ctx.CompileString(`@experiment(quantified)
f(A): func(x: A) -> A: x`))
	if got, err := v.LookupPath(cue.ParsePath("r")).Int64(); err != nil || got != 3 {
		t.Fatalf("supplied implementation: %d, %v", got, err)
	}
}

func TestQuantifiedAbstractOverloads(t *testing.T) {
	for _, src := range []string{
		`f: (func(int) -> string) & (func(string) -> int)
         out: f("x") & bool`,
		`f(A: int): func(A, A) -> A
         f(A: string): func(A, A) -> A
         out: f("x", "y") & int`,
	} {
		t.Run(src, func(t *testing.T) {
			v := cuecontext.New().CompileString("@experiment(quantified)\n" + src)
			if err := v.LookupPath(cue.ParsePath("out")).Validate(); err == nil {
				t.Fatal("abstract overload did not propagate the applicable result")
			}
		})
	}
}

func TestQuantifiedStructuralRecursion(t *testing.T) {
	for _, src := range []string{
		`sum: func(xs: [...int]) -> int: {
             if len(xs) == 0 {out: 0}
             if len(xs) > 0 {out: xs[0] + sum(xs[1:])}
         }.out
         out: sum([1, 2, 3])`,
		`sum: func(seed: int, xs: [...int]) -> int: {
             if len(xs) == 0 {out: seed}
             if len(xs) > 0 {out: sum(seed + xs[0], xs[1:])}
         }.out
         out: sum(0, [1, 2, 3])`,
		`fold: func(step: func(int, int) -> int, seed: int, xs: [...int]) -> int: {
             if len(xs) == 0 {out: seed}
             if len(xs) > 0 {out: fold(step, step(seed, xs[0]), xs[1:])}
         }.out
         plus: func(x: int, y: int) -> int: x + y
         out: fold(plus, 0, [1, 2, 3])`,
	} {
		t.Run(src, func(t *testing.T) {
			v := cuecontext.New().CompileString("@experiment(quantified)\n" + src)
			got, err := v.LookupPath(cue.ParsePath("out")).Int64()
			if err != nil || got != 6 {
				t.Fatalf("got %d, %v (root: %v)", got, err, v.Err())
			}
		})
	}
}

func TestQuantifiedRecursionRequiresDescent(t *testing.T) {
	for _, src := range []string{
		`loop: func(xs: [...int]) -> int: loop(xs)
         out: loop([1])`,
		`loop: func(xs: [...int]) -> int: loop([0, for x in xs {x}])
         out: loop([1])`,
		`loop: func(xs: [...int], ys: [...int]) -> int: loop(ys, xs)
         out: loop([1, 2], [1])`,
	} {
		t.Run(src, func(t *testing.T) {
			v := cuecontext.New().CompileString("@experiment(quantified)\n" + src)
			out := v.LookupPath(cue.ParsePath("out"))
			if !out.Exists() || out.Validate() == nil {
				t.Fatalf("missing recursion rejection (root: %v)", v.Err())
			}
		})
	}
}

func TestQuantifiedSlicePreservesSource(t *testing.T) {
	v := cuecontext.New().CompileString(`
@experiment(quantified)
f: func(xs: [...int]) -> _: {
    tail: xs[1:]
    head: xs[0]
    original: xs
    last: xs[2:]
}
out: f([1, 2, 3])
`)
	got, err := v.LookupPath(cue.ParsePath("out")).MarshalJSON()
	const want = `{"tail":[2,3],"head":1,"original":[1,2,3],"last":[3]}`
	if err != nil || string(got) != want {
		t.Fatalf("got %s, %v; want %s", got, err, want)
	}
}

func TestQuantifiedFiniteWitnesses(t *testing.T) {
	v := cuecontext.New().CompileString(`
@experiment(quantified)
x: exists (n in 1 | 2) {a: n, b: n}
x: {a: 2}
bad: exists (n in 1 | 2) {a: n, b: n}
bad: {a: 1, b: 2}
dependent: forall (n in 0 | 1) exists (m in 0 | 1) {ok: true & (n == m)}
independent: exists (m in 0 | 1) forall (n in 0 | 1) {ok: true & (n == m)}
empty: exists (n in _|_) {value: n}
vacuous: forall (n in _|_) _|_
`)
	for _, path := range []string{"bad", "independent", "empty"} {
		x := v.LookupPath(cue.ParsePath(path))
		if !x.Exists() || x.Validate() == nil {
			t.Errorf("%s: missing finite-witness conflict (root: %v)", path, v.Err())
		}
	}
	for path, want := range map[string]string{"x": `{"a":2,"b":2}`, "dependent": `{"ok":true}`} {
		got, err := v.LookupPath(cue.ParsePath(path)).MarshalJSON()
		if err != nil || string(got) != want {
			t.Errorf("%s: got %s, %v; want %s", path, got, err, want)
		}
	}
	if err := v.LookupPath(cue.ParsePath("vacuous")).Validate(); err != nil {
		t.Fatal(err)
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

func TestQuantifiedParametricAliases(t *testing.T) {
	for _, tt := range []struct{ src, want string }{
		{`Box(A) = {value: A}
Pair(A, B) = [A, B]
out: {box: Box(int) & {value: 7}, pair: Pair(int, string) & [3, "x"]}`,
			`{"box":{"value":7},"pair":[3,"x"]}`},
		{`Box(A) = {value: A}
Nested(B) = Box(Box(B))
out: Nested(string) & {value: value: "x"}`, `{"value":{"value":"x"}}`},
		{`let N = int & >=0
Box(A: N) = {value: A}
out: {let N = string, _unused: N, value: Box(3)}.value`, `{"value":3}`},
		{`Box(A) = {value: A}
wrap(A): func(x: A) -> Box(A): {value: x}
out: wrap("x")`, `{"value":"x"}`},
	} {
		t.Run(tt.src, func(t *testing.T) {
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
	for _, src := range []string{
		`Box(A) = {value: A}
         out: Box(int) & {value: "wrong"}`,
		`Box(A: number) = {value: A}
         out: Box(string)`,
		`Box(A) = {value: A}
         out: Box(int, string)`,
		`Loop(A) = Loop(A)
         out: Loop(int)`,
	} {
		t.Run(src, func(t *testing.T) {
			v := cuecontext.New().CompileString("@experiment(quantified)\n" + src)
			if v.Validate() == nil {
				t.Fatal("invalid alias application was accepted")
			}
		})
	}
}

const quantifiedCounters = `
@experiment(quantified)
#Counter: exists State {
    zero: State
    next: func(State) -> State
    read: func(State) -> (int & >=0)
}
counterV1: seal #Counter with (State = int & >=0) {
    zero: 0
    next: func(x: int & >=0) -> (int & >=0): x + 1
    read: func(x: int & >=0) -> (int & >=0): x
}
let CounterRep = close({count: int & >=0})
counterV2: seal #Counter with (State = CounterRep) {
    zero: {count: 0}
    next: func(s: CounterRep) -> CounterRep: {count: s.count + 1}
    read: func(s: CounterRep) -> (int & >=0): s.count
}
useCounter(S): func(c: {
    zero: S
    next: func(S) -> S
    read: func(S) -> (int & >=0)
}) -> (int & >=0): c.read(c.next(c.next(c.zero)))
`

func TestQuantifiedSealedCounters(t *testing.T) {
	for _, tt := range []struct{ name, expr, want string }{
		{"integer counter", `(open counterV1 as (S, C) {out: useCounter[S](C)}).out`, `2`},
		{"record counter", `(open counterV2 as (S, C) {out: useCounter[S](C)}).out`, `2`},
		{"copy", `(open (counterV1 & counterV1) as (S, C) {out: useCounter[S](C)}).out`, `2`},
		{"copy with metadata", `(open (counterV1 & {name: "counter"}) as (S, C) {out: useCounter[S](C)}).out`, `2`},
		{"equal abstract states", `(open counterV2 as (S, C) {out: C.read(C.next(C.zero) & C.next(C.zero))}).out`, `1`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			v := cuecontext.New().CompileString(quantifiedCounters + "\nout: " + tt.expr)
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

func TestQuantifiedSealGenerativity(t *testing.T) {
	ctx := cuecontext.New()
	v := ctx.CompileString(quantifiedCounters + `
make: func() -> #Counter: seal #Counter with (State = int & >=0) {
    zero: 0
    next: func(x: int & >=0) -> (int & >=0): x + 1
    read: func(x: int & >=0) -> (int & >=0): x
}
a: make()
b: make()
copy: a
good: (open (a & copy) as (S, C) {out: useCounter[S](C)}).out
bad: a & b
`)
	if got, err := v.LookupPath(cue.MakePath(cue.Str("good"))).Int64(); got != 2 || err != nil {
		t.Fatalf("copied seal: %d, %v (root: %v)", got, err, v.Err())
	}
	if err := v.LookupPath(cue.MakePath(cue.Str("bad"))).Err(); err == nil {
		t.Fatal("independently constructed seals unified")
	}
}

func TestQuantifiedComparablePackages(t *testing.T) {
	v := cuecontext.New().CompileString(`
@experiment(quantified)
#ComparablePair: exists A {
    left: A
    right: A
    equal: func(A, A) -> bool
}
compare: func(p: #ComparablePair) -> bool:
    (open p as (A, P) {out: P.equal(P.left, P.right)}).out
p: seal #ComparablePair with (A = int) {
    left: 3
    right: 3
    equal: func(x: int, y: int) -> bool: x == y
}
q: seal #ComparablePair with (A = string) {
    left: "a"
    right: "b"
    equal: func(x: string, y: string) -> bool: x == y
}
out: [compare(p), compare(q)]
`)
	got, err := v.LookupPath(cue.MakePath(cue.Str("out"))).MarshalJSON()
	if err != nil {
		t.Fatalf("%v (root: %v)", err, v.Err())
	}
	if string(got) != `[true,false]` {
		t.Fatalf("got %s", got)
	}
}

func TestQuantifiedOpaqueBoundaries(t *testing.T) {
	for _, expr := range []string{
		`counterV1.zero`,
		`counterV2["zero"]`,
		`(open counterV1 as (S, C) {out: C.zero + 1}).out`,
		`(open counterV2 as (S, C) {out: C.zero.count}).out`,
		`(open counterV1 as (S, C) {out: C.read(C.zero & C.next(C.zero))}).out`,
		`(open counterV1 as (S, C) {out: C.read(counterV2.zero)}).out`,
		`(open counterV1 as (S, C) {out: C.zero}).out`,
		`(open counterV1 as (S, C) {out: C.next}).out`,
		`(open counterV1 as (S, C) {out: C}).out`,
	} {
		t.Run(expr, func(t *testing.T) {
			v := cuecontext.New().CompileString(quantifiedCounters + "\nout: " + expr)
			if err := v.LookupPath(cue.MakePath(cue.Str("out"))).Validate(); err == nil {
				t.Fatal("invalid opaque observation was accepted")
			}
		})
	}
}

func TestQuantifiedFirstClassPackages(t *testing.T) {
	v := cuecontext.New().CompileString(`
@experiment(quantified)
#Showable: exists A {
    value: A
    show: func(A) -> string
}
p: seal #Showable with (A = int) {
    value: 7
    show: func(x: int) -> string: "\(x)"
}
q: seal #Showable with (A = string) {
    value: "hello"
    show: func(x: string) -> string: x
}
items: [p, q]
describe: func(p: #Showable) -> string:
    (open p as (A, P) {out: P.show(P.value)}).out
map(A, B): func(g: func(A) -> B, xs: [...A]) -> [...B]: [
    for x in xs {g(x)}
]
out: map(describe, items)
`)
	got, err := v.LookupPath(cue.MakePath(cue.Str("out"))).MarshalJSON()
	if err != nil {
		t.Fatalf("%v (root: %v)", err, v.Err())
	}
	if string(got) != `["7","hello"]` {
		t.Fatalf("got %s", got)
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

func TestQuantifiedPartialCapabilities(t *testing.T) {
	for _, tt := range []struct {
		name, src, want string
	}{
		{"residual result", `
add: func(x: int, y: int) -> int: x + y
p: add(1, ...)
p: func(int) -> 3
out: p(2)`, `3`},
		{"residual conflict", `
add: func(x: int, y: int) -> int: x + y
p: add(1, ...)
p: func(int) -> 4
out: p(2)`, ``},
		{"guard skips bound prefix", `
f: func(x: string, y: int) -> int: y
p: f("prefix", ...)
p: func(int) -> 3
out: p(2)`, ``},
		{"chained residual conflict", `
f: func(x: string, y: int, z: int) -> int: y + z
p: f("prefix", ...)
p: func(int, int) -> 4
q: p(1, ...)
out: q(2)`, ``},
		{"original contract retained", `
f: func(x: int, y: int) -> int: x + y
f: func(1, 2) -> 3
p: f(1, ...)
out: p(2)`, `3`},
		{"residual generic clause", `
f: func(x: string, y: int) -> int: y
p: f("prefix", ...)
p(A: int): func(A) -> A
out: p(2)`, `2`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			v := cuecontext.New().CompileString("@experiment(quantified)\n" + tt.src)
			out := v.LookupPath(cue.ParsePath("out"))
			if tt.want == "" {
				if err := out.Validate(); err == nil {
					t.Fatal("missing residual contract conflict")
				}
				return
			}
			got, err := out.MarshalJSON()
			if err != nil || string(got) != tt.want {
				t.Fatalf("got %s, %v; want %s", got, err, tt.want)
			}
		})
	}
}

func TestQuantifiedCapabilitySubsumption(t *testing.T) {
	for _, tt := range []struct {
		a, b string
		want bool
	}{
		{`forall A func(A) -> A`, `forall B func(B) -> B`, true},
		{`forall (A: int) func(A) -> A`, `forall B func(B) -> B`, true},
		{`forall A func(A) -> A`, `forall (B: int) func(B) -> B`, false},
		{`func(int) -> int`, `forall A func(A) -> A`, true},
		{`func(forall A func(A) -> A) -> int`, `func(forall B func(B) -> B) -> int`, true},
		{`forall (A, B) func(A) -> B`, `forall (X, Y) func(X) -> X`, false},
		{`forall (A, B: A) func(B) -> A`, `forall (X, Y: X) func(Y) -> X`, true},
		{`func(int) -> int`, `func(int) -> int !bridge`, false},
		{`func(int) -> int !bridge`, `func(int) -> int`, true},
		{`func(int) -> int !bridge`, `func(int) -> int !bridge`, true},
		{`func(int) -> int !bridge`, `func(int) -> int !io`, false},
		{`func(int) -> number`, `func(number) -> int`, true},
		{`func(number) -> int`, `func(int) -> number`, false},
		{`func(int) -> number`, `func(int) -> string`, false},
		{`func(x: int) -> number`, `func(y: number) -> int`, false},
		{`func(x?: int) -> int`, `func(x!: number) -> int`, false},
		{`func(x!: int) -> number`, `func(x?: number) -> int`, true},
		{`func(x: int = 1) -> number`, `func(x: number) -> int`, false},
		{`func(x: int) -> number`, `func(x: number = 0) -> int`, true},
		{`func(int) -> number`, `func(number, y: int = 0) -> int`, true},
		{`func(int) -> number`, `func(number, y: int) -> int`, false},
		{`func(int) -> number`, `(func(string) -> bool) & (func(number) -> int)`, true},
		{`(func(int) -> number) & (func(string) -> bool)`, `(func(number) -> int) & (func(string) -> bool)`, true},
		{`(func(int) -> number) & (func(string) -> bool)`, `func(number) -> int`, false},
	} {
		t.Run(tt.a+" / "+tt.b, func(t *testing.T) {
			v := cuecontext.New().CompileString("@experiment(quantified)\na: " + tt.a + "\nb: " + tt.b)
			a, b := v.LookupPath(cue.ParsePath("a")), v.LookupPath(cue.ParsePath("b"))
			if err := v.Validate(); err != nil {
				t.Fatal(err)
			}
			if err := a.Subsume(b); (err == nil) != tt.want {
				t.Fatalf("Subsume = %v; want %v", err, tt.want)
			}
		})
	}
}

func TestQuantifiedOpaqueCallbacks(t *testing.T) {
	v := cuecontext.New().CompileString(`
@experiment(quantified)
#I: exists A {
    seed: A
    read: func(A) -> int
    apply: func(func(A) -> A, A) -> A
    build: func(int) -> (func(A) -> A)
}
p: seal #I with (A = int) {
    seed: 1
    read: func(x: int) -> int: x
    apply: func(f: func(int) -> int, x: int) -> int: f(x)
    build: func(n: int) -> (func(int) -> int): func(x: int) -> int: x + n
}
out: (open p as (T, P) {
    let identity = func(x: T) -> T: x
    out: [P.read(P.apply(identity, P.seed)), P.read(P.build(3)(P.seed)), P.read(P.build(4)(P.seed))]
}).out
`)
	got, err := v.LookupPath(cue.ParsePath("out")).MarshalJSON()
	if err != nil || string(got) != `[1,4,5]` {
		t.Fatalf("got %s, %v; want [1,4,5]", got, err)
	}
}

func TestQuantifiedOpaqueClosureEscape(t *testing.T) {
	for _, tt := range []struct{ body, call, want string }{
		{`{f: func() -> _: C.zero}`, `f()`, ``},
		{`{f: func() -> _: {value: C.zero}}`, `f()`, ``},
		{`{f: func() -> _: func() -> _: C.zero}`, `f()()`, ``},
		{`{f: [func() -> _: C.zero]}`, `f[0]()`, ``},
		{`{f: func() -> int: C.read(C.next(C.zero))}`, `f()`, `1`},
		{`{f: func() -> _: func() -> int: C.read(C.zero)}`, `f()()`, `0`},
	} {
		t.Run(tt.body, func(t *testing.T) {
			v := cuecontext.New().CompileString(quantifiedCounters + "\nview: open counterV1 as (S, C) " + tt.body + "\nout: view." + tt.call)
			out := v.LookupPath(cue.ParsePath("out"))
			if tt.want == "" {
				if err := out.Validate(); err == nil {
					t.Fatal("abstract value escaped through a closure")
				}
			} else if got, err := out.MarshalJSON(); err != nil || string(got) != tt.want {
				t.Fatalf("got %s, %v; want %s", got, err, tt.want)
			}
		})
	}
}

func TestQuantifiedPredicativeUniverses(t *testing.T) {
	for _, tt := range []struct {
		src string
		bad bool
	}{
		{`f: forall (A in Type(0)) func(x: A) -> A: x
out: f[int](3)`, false},
		{`f: forall (A in Type(0)) func(x: A) -> A: x
out: f[func(int) -> int]`, false},
		{`f: forall (A in Type(0)) func(x: A) -> A: x
out: f[forall B func(B) -> B]`, true},
		{`f: forall (A in Type(1)) func(x: A) -> A: x
out: f[forall (B in Type(0)) func(B) -> B]`, false},
		{`f: forall (A in Type(1)) func(x: A) -> A: x
out: f[forall (B in Type(1)) func(B) -> B]`, true},
		{`f(A): func(x: A) -> A: x
out: f[forall (B in Type(2)) func(B) -> B]`, false},
		{`Box(A in Type(0)) = {value: A}
out: Box(forall B func(B) -> B)`, true},
		{`f: forall (A in Type(0)) func(x: A) -> A: x
out: f[{call: forall B func(B) -> B}]`, true},
		{`f: forall (A in Type(0)) func(x: A) -> A: x
out: f[[...(forall B func(B) -> B)]]`, true},
	} {
		t.Run(tt.src, func(t *testing.T) {
			v := cuecontext.New().CompileString("@experiment(quantified)\n" + tt.src)
			out := v.LookupPath(cue.ParsePath("out"))
			if !out.Exists() {
				t.Fatalf("missing output: %v", v.Err())
			}
			err := out.Validate()
			if (err != nil) != tt.bad {
				t.Fatalf("validation = %v; want error %v", err, tt.bad)
			}
		})
	}
}

func TestQuantifiedCertification(t *testing.T) {
	for _, tt := range []struct {
		name, src string
		proved    bool
	}{
		{"identity", `f(A): func(x: A) -> A: x`, true},
		{"pair", `f(A, B): func(x: A, y: B) -> [A, B]: [x, y]`, true},
		{"swap", `f(A, B): func(x: [A, B]) -> [B, A]: [x[1], x[0]]`, true},
		{"record", `f(A): func(x: A) -> {value: A}: {value: x}`, true},
		{"projection", `f(A): func(x: {value: A}) -> A: x.value`, true},
		{"primitive", `f: func(x: int) -> int: x + 1`, true},
		{"body stronger than annotation", `f: func(x: number) -> number: 1
f: func(number) -> int`, true},
		{"higher rank hypothesis", `f: func(id: forall A func(A) -> A) -> [int, string]: [id(3), id("x")]`, true},
		{"nested function", `f(A): func(x: A) -> (func(int) -> A): func(y: int) -> A: x`, true},
		{"map", `f(A, B): func(g: func(A) -> B, xs: [...A]) -> [...B]: [for x in xs {g(x)}]`, true},
		{"flat map", `f(A, B): func(g: func(A) -> [...B], xs: [...A]) -> [...B]: [for x in xs for y in g(x) {y}]`, true},
		{"filter", `f(A): func(g: func(A) -> bool, xs: [...A]) -> [...A]: [for x in xs if g(x) {x}]`, true},
		{"text", `f: func(x: int) -> string: "\(x)"`, true},
		{"list length", `f(A): func(xs: [...A]) -> int: len(xs)`, true},
		{"actual callback proof", `g: func(x: int) -> int: "wrong"
apply: func(h: func(int) -> int, x: int) -> int: h(x)
f: func(x: int) -> int: apply(g, x)`, false},
		{"returned implementation proof", `f: func() -> (func(int) -> int): func(x: int) -> int: "wrong"`, false},
		{"unimplemented", `f(A): func(A) -> A`, false},
		{"unsupported arithmetic", `f: func(x: int & >0) -> (int & >=0): x - 1`, false},
		{"unchecked captured witness", `x: int
f: func() -> int: x`, false},
		{"proof dependency cycle", `f: func(x: int) -> int: g(x)
g: func(x: int) -> int: f(x)`, false},
		{"foreign boundary", `external: extern func(int) -> int !bridge
f: func(x: int) -> int: external(x)`, false},
		{"effectful callback", `f: func(g: func(int) -> int !bridge, x: int) -> int: g(x)`, false},
		{"optional presence", `f: func(x?: int) -> int: x`, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			v := cuecontext.New().CompileString("@experiment(quantified)\n" + tt.src)
			f := v.LookupPath(cue.ParsePath("f"))
			if !f.Exists() {
				t.Fatal(v.Err())
			}
			if err := f.Validate(); err != nil {
				t.Fatalf("ordinary validation: %v", err)
			}
			if err := f.Validate(cue.VerifyFunctions(true)); (err == nil) != tt.proved {
				t.Fatalf("certification: %v; want proof %v", err, tt.proved)
			}
		})
	}
}

func TestQuantifiedRecordDataProjections(t *testing.T) {
	ctx := cuecontext.New()
	v := ctx.CompileString(`
@experiment(quantified)
bad(A): {value: A, id: func(A) -> A}
good(A): {empty: [...A], absent?: A, id: func(x: A) -> A: x}
out: [good.empty, good.id(1), good.id("one")]
nested: forall A forall B {value: A | B}
`)
	for _, path := range []string{"bad", "nested"} {
		x := v.LookupPath(cue.ParsePath(path))
		if !x.Exists() || x.Validate() == nil {
			t.Fatalf("%s: missing empty-instance refutation (%v)", path, v.Err())
		}
	}
	if got, err := v.LookupPath(cue.ParsePath("out")).MarshalJSON(); err != nil || string(got) != `[[],1,"one"]` {
		t.Fatalf("got %s, %v; want [[],1,\"one\"]", got, err)
	}
}

func TestQuantifiedExport(t *testing.T) {
	for _, tt := range []struct{ src, field, call, want string }{
		{`id(A): func(x: A) -> A: x`, "id", `f(3)`, `3`},
		{`id(A): func(x: A) -> A: x
specialized: id[int]`, "specialized", `f(3)`, `3`},
		{`id(A: int): func(x: A) -> A: x`, "id", `f(3)`, `3`},
		{`constant(A): func(x: A) -> (forall B func(B) -> A): func<B>(y: B) -> A: x`, "constant", `f(3)("x")`, `3`},
		{`make(A): func(x: A) -> (func(int) -> A): func(y: int) -> A: x
f: make(3)`, "f", `f(0)`, `3`},
		{`prefix: "> "
f: func(x: string) -> string: prefix + x`, "f", `f("text")`, `"> text"`},
		{`f: forall A func(A) -> A`, "f", `f(3) & string`, ``},
	} {
		t.Run(tt.src, func(t *testing.T) {
			v := cuecontext.New().CompileString("@experiment(quantified)\n" + tt.src)
			field := v.LookupPath(cue.ParsePath(tt.field))
			if !field.Exists() {
				t.Fatal(v.Err())
			}
			text, err := format.Node(field.Syntax())
			if err != nil {
				t.Fatal(err)
			}
			rebuilt := cuecontext.New().CompileString("@experiment(quantified)\nf: " + string(text) + "\nout: " + tt.call)
			out := rebuilt.LookupPath(cue.ParsePath("out"))
			if !out.Exists() {
				t.Fatalf("export %s: %v", text, rebuilt.Err())
			}
			if tt.want == "" {
				if out.Validate() == nil {
					t.Fatalf("export lost universal obligation: %s", text)
				}
			} else if got, err := out.MarshalJSON(); err != nil || string(got) != tt.want {
				t.Fatalf("export %s: got %s, %v; want %s", text, got, err, tt.want)
			}
		})
	}
}

func TestQuantifiedImplementationConcreteness(t *testing.T) {
	v := cuecontext.New().CompileString(`
@experiment(quantified)
unknown(A): func(A) -> A
known(A): func(x: A) -> A: x
`)
	if err := v.LookupPath(cue.ParsePath("unknown")).Validate(cue.Concrete(true)); err == nil {
		t.Fatal("bodyless contract was mistaken for an implementation")
	}
	if err := v.LookupPath(cue.ParsePath("known")).Validate(cue.Concrete(true)); err != nil {
		t.Fatal(err)
	}
}

func TestQuantifiedCovariantExistentials(t *testing.T) {
	for _, tt := range []struct {
		schema, data string
		valid        bool
	}{
		{`exists A A`, `1`, true},
		{`exists A {left: A, right: A}`, `{left: 1, right: "x"}`, true},
		{`exists (A: int) A`, `1`, true},
		{`exists (A: int) A`, `"x"`, false},
		{`exists (A: int & >0) A`, `1`, true},
		{`exists (A: int & >0) A`, `0`, false},
		{`exists (A: int, B: A) {left: A, right: B}`, `{left: 1, right: 2}`, true},
		{`exists (A: int, B: A) {left: A, right: B}`, `{left: 1, right: "x"}`, false},
		{`exists A [...A]`, `[1, "x", true]`, true},
	} {
		t.Run(tt.schema+" / "+tt.data, func(t *testing.T) {
			v := cuecontext.New().CompileString("@experiment(quantified)\nout: (" + tt.schema + ") & (" + tt.data + ")")
			out := v.LookupPath(cue.ParsePath("out"))
			if !out.Exists() {
				t.Fatal(v.Err())
			}
			if err := out.Validate(cue.Concrete(true)); (err == nil) != tt.valid {
				t.Fatalf("validation: %v; want success %v", err, tt.valid)
			}
		})
	}
}

func TestQuantifiedWitnessCorrelation(t *testing.T) {
	for _, tt := range []struct {
		name, src string
		good, bad int
		want      string
	}{
		{"guard", `f: func(n: int) -> int: n
f: func(witness) -> 0
out: f(1)`, 0, 1, `1`},
		{"selector result", `record: {value: witness}
f: func() -> record.value: 1
out: f()`, 1, 2, `1`},
		{"alias result", `let alias = witness
f: func() -> alias: 1
out: f()`, 1, 2, `1`},
		{"indexed result", `items: [witness]
f: func() -> items[0]: 1
out: f()`, 1, 2, `1`},
		{"explicit type argument", `id(A): func(x: A) -> A: x
out: id[witness](1)`, 1, 2, `1`},
		{"result", `f: func() -> witness: 1
out: f()`, 1, 2, `1`},
		{"protocol", `f: func(x: witness) -> int: 0
out: f(1)`, 1, 2, `0`},
		{"type bound", `f(A: witness): func(x: A) -> A: x
out: f(1)`, 1, 2, `1`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			v := cuecontext.New().CompileString("@experiment(quantified)\nwitness: int\n" + tt.src)
			out := v.LookupPath(cue.ParsePath("out"))
			if !out.Exists() {
				t.Fatal(v.Err())
			}
			if err := out.Validate(); err != nil {
				t.Fatalf("unresolved witness was refuted: %v", err)
			}
			if err := out.Validate(cue.Concrete(true)); err == nil {
				t.Fatal("witness upper bound was mistaken for its singleton")
			}
			good := v.FillPath(cue.ParsePath("witness"), tt.good).LookupPath(cue.ParsePath("out"))
			if got, err := good.MarshalJSON(); err != nil || string(got) != tt.want {
				t.Fatalf("good refinement: %s, %v; want %s", got, err, tt.want)
			}
			bad := v.FillPath(cue.ParsePath("witness"), tt.bad).LookupPath(cue.ParsePath("out"))
			if err := bad.Validate(); err == nil {
				t.Fatal("incompatible witness refinement was accepted")
			}
		})
	}
	v := cuecontext.New().CompileString(`@experiment(quantified)
witness: int
f: func(x: int) -> witness: x`)
	if err := v.LookupPath(cue.ParsePath("f")).Validate(cue.VerifyFunctions(true)); err == nil {
		t.Fatal("a result singleton was certified from its upper approximation")
	}
}

func TestQuantifiedUniverseOccursCheck(t *testing.T) {
	for _, expr := range []string{`id[id]`, `id(id)`, `id[{f: id}]`} {
		v := cuecontext.New().CompileString("@experiment(quantified)\nid(A): func(x: A) -> A: x\nout: " + expr)
		out := v.LookupPath(cue.ParsePath("out"))
		if !out.Exists() || out.Validate() == nil {
			t.Fatalf("infinite universe level was accepted: %s (%v)", expr, v.Err())
		}
	}
}

func TestQuantifiedFileOrder(t *testing.T) {
	cases := [][]string{
		{`wrap(A): func(A) -> {value: A}`,
			`wrap(A): func(A) -> {tag: "wrapped"}`,
			`wrap(A): func(x: A) -> {value: A, tag: "wrapped"}: {value: x, tag: "wrapped"}
out: wrap(1)`},
		{`f: func(x: number) -> number: 1`, `f: func(number) -> int`, `out: f(1.5)`},
	}
	want := []string{`{"value":1,"tag":"wrapped"}`, `1`}
	for i, files := range cases {
		for _, order := range [][3]int{{0, 1, 2}, {0, 2, 1}, {1, 0, 2}, {1, 2, 0}, {2, 0, 1}, {2, 1, 0}} {
			t.Run(fmt.Sprint(i, order), func(t *testing.T) {
				instance := build.NewContext().NewInstance(".", nil)
				for _, n := range order {
					if err := instance.AddFile(fmt.Sprintf("part%d.cue", n), "@experiment(quantified)\npackage test\n"+files[n]); err != nil {
						t.Fatal(err)
					}
				}
				v := cuecontext.New().BuildInstance(instance)
				got, err := v.LookupPath(cue.ParsePath("out")).MarshalJSON()
				if err != nil {
					t.Fatal(err)
				}
				// Decode to compare records independently of declaration order.
				expected := cuecontext.New().CompileString(want[i])
				actual := expected.Context().CompileString(string(got))
				if expected.Subsume(actual, cue.Final()) != nil || actual.Subsume(expected, cue.Final()) != nil {
					t.Fatalf("got %s; want %s", got, want[i])
				}
			})
		}
	}
}

func TestQuantifiedClosureCompleteness(t *testing.T) {
	for _, tt := range []struct {
		name, src string
		complete  bool
	}{
		{"body capture", `n: int
f: func() -> int: n`, false},
		{"default capture", `n: int
f: func(x: int = n) -> int: x`, false},
		{"erased schema", `#N: int
f: func(x: #N) -> #N: x`, true},
		{"erased schema alias", `let N = int
f: func(x: N) -> N: x`, true},
		{"recursive descriptor", `f: func(xs: [...int]) -> int: ({
if len(xs) == 0 {out: 0}
if len(xs) > 0 {out: xs[0] + f(xs[1:])}
}).out`, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			v := cuecontext.New().CompileString("@experiment(quantified)\n" + tt.src)
			f := v.LookupPath(cue.ParsePath("f"))
			if !f.Exists() || f.Validate() != nil {
				t.Fatal(v.Err())
			}
			if err := f.Validate(cue.Concrete(true)); (err == nil) != tt.complete {
				t.Fatalf("concrete: %v; want complete %v", err, tt.complete)
			}
			if !tt.complete {
				if err := v.FillPath(cue.ParsePath("n"), 1).LookupPath(cue.ParsePath("f")).Validate(cue.Concrete(true)); err != nil {
					t.Fatalf("concrete capture refinement: %v", err)
				}
			}
		})
	}
}

func TestQuantifiedOpaqueCompositeTransport(t *testing.T) {
	for _, tt := range []struct{ schema, body, observation, want string }{
		{`Box(A) = {value?: A}`, `{value: 3}`, `P.read(P.box.value)`, `3`},
		{`Box(A) = {value?: A}`, `{}`, `P.read(P.seed)`, `1`},
		{`Box(A) = {[string]: A}`, `{one: 2, two: 3}`, `[P.read(P.box.one), P.read(P.box.two)]`, `[2,3]`},
		{`Box(A) = [...A]`, `[2, 3]`, `[P.read(P.box[0]), P.read(P.box[1])]`, `[2,3]`},
	} {
		t.Run(tt.schema+tt.body, func(t *testing.T) {
			v := cuecontext.New().CompileString(`@experiment(quantified)
` + tt.schema + `
#I: exists A {seed: A, box: Box(A), read: func(A) -> int}
p: seal #I with (A = int) {
seed: 1
box: ` + tt.body + `
read: func(x: int) -> int: x
}
out: (open p as (T, P) {out: ` + tt.observation + `}).out`)
			got, err := v.LookupPath(cue.ParsePath("out")).MarshalJSON()
			if err != nil || string(got) != tt.want {
				t.Fatalf("got %s, %v; want %s", got, err, tt.want)
			}
		})
	}
}

func TestQuantifiedOpaqueUniverseBoundary(t *testing.T) {
	v := cuecontext.New().CompileString(`@experiment(quantified)
#I: exists (A in Type(0)) {value: A}
p: seal #I with (A = forall (B in Type(0)) func(B) -> B) {
value: func<B>(x: B) -> B: x
}`)
	if err := v.LookupPath(cue.ParsePath("p")).Validate(); err == nil {
		t.Fatal("representation witness exceeded the existential universe")
	}
}

func TestQuantifiedOpaqueGenericOperations(t *testing.T) {
	v := cuecontext.New().CompileString(`@experiment(quantified)
#I: exists State {
seed: State
read: func(State) -> int
keep(A): func(A, State) -> {value: A, state: State}
empty(A): func() -> [...A]
}
p: seal #I with (State = int) {
seed: 1
read: func(x: int) -> int: x
keep(A): func(x: A, s: int) -> {value: A, state: int}: {value: x, state: s}
empty(A): func() -> [...A]: []
}
out: (open p as (S, P) {
let pair = P.keep("hello", P.seed)
out: [pair.value, P.read(pair.state), P.empty[int](), P.read(P.keep(P.seed, P.seed).value), P.read(P.keep({value: P.seed}, P.seed).value.value)]
}).out`)
	got, err := v.LookupPath(cue.ParsePath("out")).MarshalJSON()
	if err != nil || string(got) != `["hello",1,[],1,1]` {
		t.Fatalf("got %s, %v; want [\"hello\",1,[],1,1]", got, err)
	}
}
