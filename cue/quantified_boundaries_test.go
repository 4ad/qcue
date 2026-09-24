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
)

// Erasure is a congruence for runtime observations: putting two inhabitants
// into the same data context must not make their retained contracts visible.
// Conversely, differing code origins, captures, and saved arguments remain
// observable through each context. Expected identities are specified here
// independently of the evaluator's graph and closure comparators.
func TestQuantifiedBoundaryRuntimeEquality(t *testing.T) {
	cases := []struct {
		name, declarations, a, b string
		equal                    bool
	}{
		{"contract", `f: func(x: int) -> int: x`, `f`, `f & (func(int) -> int)`, true},
		{"erasure", `f(A): func(x: A) -> A: x`, `f[int]`, `f[string]`, true},
		{"builtin", `import "strings"`, `strings.ToUpper`, `strings.ToUpper & (func(string) -> string)`, true},
		{"origin", `f: func(x: int) -> int: x
g: func(x: int) -> int: x`, `f`, `g`, false},
		{"saved", `f: func(x: int, y: int) -> int: x+y`, `f(1, ...)`, `f(2, ...)`, false},
		{"sameSaved", `f: func(x: int, y: int) -> int: x+y`, `f(1, ...)`, `f(x: 1, ...)`, true},
		{"capture", `fs: [for n in [1, 2] {func(x: int) -> int: x+n}]`, `fs[0]`, `fs[1]`, false},
		{"predicates", ``, `{x: 1, #T: int, y?: int}`, `{x: 1, #T: string, y?: string}`, true},
	}
	contexts := []string{`[%s]`, `[{x: %s}]`, `[[%s]]`, `[{x: [%s], y: 0}]`}
	for _, tc := range cases {
		for _, context := range contexts {
			t.Run(tc.name+"/"+context, func(t *testing.T) {
				a, b := fmt.Sprintf(context, tc.a), fmt.Sprintf(context, tc.b)
				v := semanticValue(t, tc.declarations+fmt.Sprintf("\nout: [%s == %s, %s != %s]", a, b, a, b))
				if err := v.Validate(cue.Concrete(true)); err != nil {
					t.Fatal(err)
				}
				semanticJSON(t, v, "out", fmt.Sprintf("[%v,%v]", tc.equal, !tc.equal))
			})
		}
	}
}

func TestQuantifiedBoundaryOpaqueEquality(t *testing.T) {
	v := semanticValue(t, `
#M: exists A {left: A, right: A}
base: func(x: int) -> int: x
p: seal #M with (A = {f: func(int) -> int}) {
    left: {f: base}
    right: {f: base & (func(int) -> int)}
}
out: (open p as (A, P) {result: [
    P.left == P.right,
    [P.left] == [P.right],
    [{value: P.left}] == [{value: P.right}],
    [P.left] != [P.right],
]}).result
`)
	if err := v.Validate(cue.Concrete(true)); err != nil {
		t.Fatal(err)
	}
	semanticJSON(t, v, "out", `[true,true,true,false]`)
}

// A contract may constrain an abstract call only after original-packet
// admission. These cases independently enumerate exact record presence and
// scalar membership, then put the packet in several structural contexts.
func TestQuantifiedBoundaryAbstractAdmission(t *testing.T) {
	for _, label := range []string{"a", "_a"} {
		for _, context := range []string{`%s`, `{nested: %s}`, `[%s]`} {
			for _, tc := range []struct {
				packet string
				member bool
			}{
				{`{}`, false},
				{fmt.Sprintf(`{%s?: 1}`, label), false},
				{fmt.Sprintf(`{%s: 2}`, label), false},
				{fmt.Sprintf(`{%s: 1}`, label), true},
				{fmt.Sprintf(`{%s: 1, extra: 2}`, label), true},
			} {
				t.Run(label+"/"+context+"/"+tc.packet, func(t *testing.T) {
					guard := fmt.Sprintf(context, fmt.Sprintf(`{%s: 1}`, label))
					packet := fmt.Sprintf(context, tc.packet)
					v := semanticValue(t, fmt.Sprintf("f: func(%s) -> string\nout: f(%s) & 0", guard, packet))
					if refuted := v.Validate() != nil; refuted != tc.member {
						t.Fatalf("refuted=%v, original packet admitted=%v: %v", refuted, tc.member, v.Validate())
					}
					if err := v.Validate(cue.Concrete(true)); err == nil {
						t.Fatal("abstract call manufactured an execution witness")
					}
				})
			}
		}
	}
	// An intersection contributes every established consequence, regardless
	// of its head clause. A packet outside one domain remains possible.
	for _, contract := range []string{
		`(func({a: 1}) -> string) & (func({}) -> int)`,
		`(func({}) -> int) & (func({a: 1}) -> string)`,
	} {
		v := semanticValue(t, "f: "+contract+"\nout: f({}) & 0")
		if err := v.Validate(); err != nil {
			t.Fatalf("inapplicable clause constrained abstract result: %v", err)
		}
	}
}

func TestQuantifiedBoundaryAbstractRefinement(t *testing.T) {
	const original = `f: func({a: 1}) -> string
out: f({}) & 0`
	const implementation = `f: func(x: {}) -> (int|string): {
    if len(x) == 0 {r: 0}
    if len(x) > 0 {r: "ok"}
}.r`
	v := semanticValue(t, original)
	if err := v.Validate(); err != nil {
		t.Fatalf("satisfiable abstract program refuted: %v", err)
	}
	// Refinement of the same graph and rebuilding the combined source must
	// agree. An implementation can resolve uncertainty, never repair bottom.
	for _, refined := range []cue.Value{
		v.Unify(semanticValue(t, implementation)),
		semanticValue(t, original+"\n"+implementation),
	} {
		if err := refined.Validate(); err != nil {
			t.Fatal(err)
		}
		semanticJSON(t, refined, "out", "0")
	}
}

func TestQuantifiedBoundaryIdentityTransport(t *testing.T) {
	for _, tc := range []struct{ subject, refinement string }{
		{`{x?: int}`, `{x: "bad"}`},
		{`close({})`, `{x: 1}`},
		{`{[string]: int}`, `{x: "bad"}`},
		{`{nested: {x?: int}}`, `{nested: {x: "bad"}}`},
		{`{_x?: int}`, `{_x: "bad"}`},
	} {
		t.Run(tc.subject, func(t *testing.T) {
			v := semanticValue(t, `
#M: exists A {id: func({}) -> {}}
p: seal #M with (A = int) {id: func(x: {}) -> {}: x}
out: (open p as (A, P) {r: P.id(`+tc.subject+`) & `+tc.refinement+`}).r
`)
			if err := v.LookupPath(cue.ParsePath("out")).Validate(); err == nil {
				t.Fatal("identity transport discarded a constraint")
			}
		})
	}
}

// A changing abstract field must not weaken constraints on the containing
// record. Both accepted and rejected refinements are specified independently
// of transport, then tested through direct and higher-order crossings.
func TestQuantifiedBoundaryCompositeConstraints(t *testing.T) {
	for _, tc := range []struct{ subject, good, bad string }{
		{`{v: P.zero, x?: int}`, `{x: 1}`, `{x: "bad"}`},
		{`close({v: P.zero})`, `{}`, `{x: 1}`},
		{`{v: P.zero, [=~"^extra"]: int}`, `{extra: 1}`, `{extra: "bad"}`},
		{`{v: P.zero, nested: {x?: int}}`, `{nested: {x: 1}}`, `{nested: {x: "bad"}}`},
		{`{v: P.zero, _x?: int}`, `{_x: 1}`, `{_x: "bad"}`},
	} {
		for _, operation := range []string{`P.echo`, `P.callback(func(x: {v: A}) -> {v: A}: x)`} {
			for _, valid := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/valid=%v", tc.subject, operation, valid), func(t *testing.T) {
					refinement := tc.bad
					if valid {
						refinement = tc.good
					}
					source := `
#M: exists A {
 zero: A
 read: func(A) -> int
 echo: func({v: A}) -> {v: A}
 callback: func(func({v: A}) -> {v: A}) -> func({v: A}) -> {v: A}
}
p: seal #M with (A = int) {
 zero: 7
 read: func(x: int) -> int: x
 echo: func(x: {v: int}) -> {v: int}: x
 callback: func(f: func({v: int}) -> {v: int}) -> func({v: int}) -> {v: int}: f
}
delta: {}
out: (open p as (A, P) {
 let q = ` + operation + `(` + tc.subject + `) & delta
 r: (func(x: {v: A}) -> int: P.read(x.v))(q)
}).r
`
					original := semanticValue(t, source)
					for _, v := range []cue.Value{
						semanticValue(t, source+"\ndelta: "+refinement),
						original.FillPath(cue.ParsePath("delta"), original.Context().CompileString(refinement)),
					} {
						if err := v.Validate(); (err == nil) != valid {
							t.Fatalf("valid=%v: %v", valid, err)
						}
						if valid {
							if err := v.Validate(cue.Concrete(true)); err != nil {
								t.Fatal(err)
							}
							semanticJSON(t, v, "out", "7")
						}
					}
				})
			}
		}
	}
}

// Definitions and absent optional fields are predicates, not demanded runtime
// witnesses. Their restrictions must survive projection as well as refinement
// of the enclosing transported record.
func TestQuantifiedBoundaryTransportPredicates(t *testing.T) {
	for _, tc := range []struct{ public, private, subject, refinement, observe string }{
		{`{v: A, x?: A}`, `{v: int, x?: int}`, `{v: P.zero, x?: P.zero}`, `{x: %s}`, `q.x`},
		{`{v: A, #T: A}`, `{v: int, #T: int}`, `{v: P.zero, #T: P.zero}`, `{}`, `q.#T & %s`},
		{`{v: A, #T: {x: A}}`, `{v: int, #T: {x: int}}`, `{v: P.zero, #T: {x: P.zero}}`, `{}`, `(q.#T & {x: %s}).x`},
	} {
		for _, valid := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/valid=%v", tc.public, valid), func(t *testing.T) {
				value := `P.one`
				if valid {
					value = `P.zero`
				}
				refinement, observe := tc.refinement, tc.observe
				if refinement != `{}` {
					refinement = fmt.Sprintf(refinement, value)
				} else {
					observe = fmt.Sprintf(observe, value)
				}
				v := semanticValue(t, fmt.Sprintf(`
#M: exists A {zero: A, one: A, read: func(A) -> int, echo: func(%s) -> %s}
p: seal #M with (A = int) {
 zero: 7
 one: 8
 read: func(x: int) -> int: x
 echo: func(x: %s) -> %s: x
}
out: (open p as (A, P) {
 let q = P.echo(%s) & %s
 r: P.read(%s)
}).r
`, tc.public, tc.public, tc.private, tc.private, tc.subject, refinement, observe))
				if err := v.Validate(); (err == nil) != valid {
					t.Fatalf("valid=%v: %v", valid, err)
				}
				if valid {
					if err := v.Validate(cue.Concrete(true)); err != nil {
						t.Fatal(err)
					}
					semanticJSON(t, v, "out", "7")
				}
			})
		}
	}
}

func TestQuantifiedBoundaryScopedTransport(t *testing.T) {
	for _, tc := range []struct{ public, private, value, access string }{
		{`[...(A|null)]`, `[...int]`, `[7]`, `[0]`},
		{`[(A|null)]`, `[int]`, `[7]`, `[0]`},
		{`[{value: A|null}]`, `[{value: int}]`, `[{value: 7}]`, `[0].value`},
	} {
		t.Run(tc.public, func(t *testing.T) {
			v := semanticValue(t, fmt.Sprintf(`
#M: exists A {
    make: func() -> %s
    read: func(A) -> int
}
p: seal #M with (A = int) {
    make: func() -> %s: %s
    read: func(x: int) -> int: x
}
out: (open p as (A, P) {r: P.read(P.make()%s)}).r
`, tc.public, tc.private, tc.value, tc.access))
			if err := v.Validate(cue.Concrete(true)); err != nil {
				t.Fatal(err)
			}
			semanticJSON(t, v, "out", "7")
		})
	}
	v := semanticValue(t, `
#M: exists A {zero: A, f: func({#T: A, value: A}) -> int}
p: seal #M with (A = int) {
    zero: 7
    f: func(x: {#T: int, value: int}) -> int: x.value
}
out: (open p as (A, P) {r: P.f({#T: A, value: P.zero})}).r
`)
	if err := v.Validate(cue.Concrete(true)); err != nil {
		t.Fatal(err)
	}
	semanticJSON(t, v, "out", "7")
}

func TestQuantifiedBoundarySealWitnessSort(t *testing.T) {
	for _, tc := range []struct {
		witness string
		valid   bool
	}{
		{"1", true}, {"2", true}, {"7", false}, {`"bad"`, false}, {"int", false},
	} {
		for _, prefix := range []string{"n in 1|2, A", "A, n in 1|2"} {
			t.Run(prefix+"/"+tc.witness, func(t *testing.T) {
				v := semanticValue(t, fmt.Sprintf(`
#M: exists (%s) {value: n, zero: A}
p: seal #M with (n = %s, A = string) {value: %s, zero: "zero"}
out: (open p as (A, P) {r: P.value}).r
`, prefix, tc.witness, tc.witness))
				if err := v.Validate(cue.Concrete(true)); (err == nil) != tc.valid {
					t.Fatalf("witness valid=%v: %v", tc.valid, err)
				}
				if tc.valid {
					semanticJSON(t, v, "out", tc.witness)
				}
			})
		}
	}
}

// Every concrete input admitted by a certified existential client must have
// an executable elimination. Covariant membership supplies a constructive
// witness even when the input did not come from an explicit seal.
func TestQuantifiedBoundaryExistentialElimination(t *testing.T) {
	for _, subject := range []string{`{x: 1}`, `{x: "s"}`, `{x: {nested: [1,2]}}`, `{x: 1, extra: true}`} {
		for _, input := range []string{subject, "#M & " + subject} {
			t.Run(input, func(t *testing.T) {
				v := semanticValue(t, `
#M: exists A {x: A}
use: func(p: #M) -> int: (open p as (A, P) {result: 0}).result
out: use(`+input+`)
`)
				if err := v.Validate(cue.Concrete(true)); err != nil {
					t.Fatal(err)
				}
				semanticJSON(t, v, "out", "0")
			})
		}
	}
	v := semanticValue(t, `
#M: exists A {x: A, y: A}
p: #M & {x: 1, y: 1}
q: #M & {x: 1, y: 2}
out: [(open p as (A, P) {r: P.x == P.y}).r,
      (open q as (A, Q) {r: Q.x == Q.y}).r]
`)
	if err := v.Validate(cue.Concrete(true)); err != nil {
		t.Fatal(err)
	}
	semanticJSON(t, v, "out", `[true,false]`)
	v = semanticValue(t, `
#M: exists A {x: A}
p: #M & {x: 1}
copy: p
out: (open p as (A, P) {
    r: (open copy as (B, Q) {v: P.x == Q.x}).v
}).r
`)
	semanticJSON(t, v, "out", "true")
	// Covariant membership alone is insufficient if constructing its public
	// view requires an ambiguous union transport. Certification must retain
	// that obligation instead of promising an executable sealed witness.
	v = semanticValue(t, `
#M: exists A {x: A|null}
use: func(p: #M) -> int: (open p as (A, P) {result: 0}).result
out: use({x: null})
`)
	if err := v.Validate(cue.Concrete(true)); err == nil {
		t.Fatal("certified an ambiguous constructive witness transport")
	}
}

func TestQuantifiedBoundaryCallContracts(t *testing.T) {
	for _, intersection := range []string{
		`(func(int) -> int) & (func(string) -> string)`,
		`(func(string) -> string) & (func(int) -> int)`,
	} {
		for _, body := range []string{`g("x")`, `{h: g}.h("x")`, `[g][0]("x")`} {
			v := semanticValue(t, `
f: func(g: `+intersection+`) -> string: `+body+`
id: func(x: _) -> _: x
out: f(id)
`)
			if err := v.Validate(cue.Concrete(true)); err != nil {
				t.Fatalf("%s / %s: %v", intersection, body, err)
			}
			semanticJSON(t, v, "out", `"x"`)
		}
	}
	// Coverage and consequences are separate: both overlapping clauses
	// constrain the result even if the first alone is too weak to prove it.
	for _, intersection := range []string{
		`(func(int) -> int) & (func(int) -> 0)`,
		`(func(int) -> 0) & (func(int) -> int)`,
	} {
		v := semanticValue(t, `
f: func(g: `+intersection+`) -> 0: g(7)
zero: func(x: int) -> 0: 0
out: f(zero)
`)
		if err := v.Validate(cue.Concrete(true)); err != nil {
			t.Fatal(err)
		}
		semanticJSON(t, v, "out", "0")
	}
	// Original universal obligations must not reopen a selected view's
	// executable domain. Identity erasure does not supply this admission.
	v := semanticValue(t, `
id(A): func(x: A) -> A: x
bad: func() -> string: id[int]("x")
`)
	if err := v.Validate(cue.Concrete(true)); err == nil {
		t.Fatal("universal obligation reopened a selected call domain")
	}
	v = semanticValue(t, `
#F: func(int) -> int
bad: func(g: #F) -> int: {h: #F}.h(0)
`)
	if err := v.Validate(cue.Concrete(true)); err == nil {
		t.Fatal("constructing a schema manufactured a callback hypothesis")
	}
	v = semanticValue(t, `
base: func(x: int, y: int|string) -> (int|string): y
partial: base(0, ...) & (func(string) -> string)
f: func() -> string: partial("x")
out: f()
`)
	if err := v.Validate(cue.Concrete(true)); err != nil {
		t.Fatal(err)
	}
	semanticJSON(t, v, "out", `"x"`)
	for _, instance := range []string{"generic", "generic[int|string]"} {
		v := semanticValue(t, `
generic(A): func(x: A, y: A) -> A: y
partial: `+instance+`(0, ...)
f: func() -> (int|string): partial("x")
out: f()
`)
		if err := v.Validate(cue.Concrete(true)); err != nil {
			t.Fatalf("%s: %v", instance, err)
		}
		semanticJSON(t, v, "out", `"x"`)
	}
}

// The finite model is identity on {0,1}. Its output guarantee on a domain D
// is exactly D, regardless of how its singleton clauses are ordered. Check
// both proof and execution against set inclusion rather than another CUE
// expression or the implementation's clause enumeration.
func TestQuantifiedBoundaryOverloadCoverage(t *testing.T) {
	predicate := func(set int) string {
		return []string{"_|_", "0", "1", "0|1"}[set]
	}
	for domain := 1; domain < 4; domain++ {
		for result := 1; result < 4; result++ {
			for _, contract := range []string{
				`(func(0) -> 0) & (func(1) -> 1)`,
				`(func(1) -> 1) & (func(0) -> 0)`,
			} {
				source := fmt.Sprintf(`
f: func(g: %s, x: %s) -> (%s): g(x)
id: func(x: int) -> int: x
`, contract, predicate(domain), predicate(result))
				v := semanticValue(t, source)
				want := domain & ^result == 0
				if err := v.Validate(cue.Concrete(true)); (err == nil) != want {
					t.Fatalf("D=%s R=%s, valid=%v: %v", predicate(domain), predicate(result), want, err)
				}
				if want {
					for value := 0; value < 2; value++ {
						if domain&(1<<value) == 0 {
							continue
						}
						run := semanticValue(t, source+fmt.Sprintf("\nout: f(id, %d)", value))
						semanticJSON(t, run, "out", fmt.Sprint(value))
					}
				}
			}
		}
	}
}

// Membership in a record with a function-valued definition does not supply
// an executable callback. The same rule applies below every data constructor.
func TestQuantifiedBoundaryCallbackEvidence(t *testing.T) {
	for _, label := range []string{"f", "_f", "#F", "_#F", "f?"} {
		for _, tc := range []struct{ domain, packet, path string }{
			{`{%s: func(int) -> int}`, `{%s: func(int) -> int}`, `p.%s`},
			{`{nested: {%s: func(int) -> int}}`, `{nested: {%s: func(int) -> int}}`, `p.nested.%s`},
			{`[{%s: func(int) -> int}]`, `[{%s: func(int) -> int}]`, `p[0].%s`},
		} {
			t.Run(label+"/"+tc.domain, func(t *testing.T) {
				field := label
				if label == "f?" {
					field = "f"
				}
				domain, packet := fmt.Sprintf(tc.domain, label), fmt.Sprintf(tc.packet, label)
				required := label == "f" || label == "_f"
				if required {
					packet = strings.ReplaceAll(packet, "func(int) -> int", "func(x: int) -> int: x")
				}
				source := fmt.Sprintf("f: func(p: %s) -> int: %s(0)\nout: f(%s)", domain, fmt.Sprintf(tc.path, field), packet)
				v := semanticValue(t, source)
				if err := v.Validate(); err != nil {
					t.Fatal(err)
				}
				if err := v.LookupPath(cue.ParsePath("f")).Validate(cue.Concrete(true)); (err == nil) != required {
					t.Fatalf("executable callback=%v: %v", required, err)
				}
				if required {
					semanticJSON(t, v, "out", "0")
				} else if err := v.LookupPath(cue.ParsePath("out")).Validate(cue.Concrete(true)); err == nil {
					t.Fatal("a predicate supplied a callback implementation")
				}
			})
		}
	}
}
