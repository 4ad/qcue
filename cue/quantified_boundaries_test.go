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
	"cuelang.org/go/internal/core/adt"
	"cuelang.org/go/internal/value"
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

// A bodyless hypothesis supplies a result consequence under its constrained
// packet. Missing fields can acquire the parameter constraint, but no
// execution witness is manufactured by that successful logical propagation.
func TestQuantifiedBoundaryAbstractAdmission(t *testing.T) {
	for _, label := range []string{"a", "_a"} {
		for _, context := range []string{`%s`, `{nested: %s}`, `[%s]`} {
			for _, tc := range []struct {
				packet string
				member bool
			}{
				{`{}`, true},
				{fmt.Sprintf(`{%s?: 1}`, label), true},
				{fmt.Sprintf(`{%s: 2}`, label), false},
				{fmt.Sprintf(`{%s: 1}`, label), true},
				{fmt.Sprintf(`{%s: 1, extra: 2}`, label), true},
			} {
				t.Run(label+"/"+context+"/"+tc.packet, func(t *testing.T) {
					guard := fmt.Sprintf(context, fmt.Sprintf(`{%s: 1}`, label))
					packet := fmt.Sprintf(context, tc.packet)
					v := semanticValue(t, fmt.Sprintf("f: func(%s) -> string\nout: f(%s) & 0\ncall: f(%s)", guard, packet, packet))
					out := v.LookupPath(cue.ParsePath("out"))
					if out.Validate() == nil {
						t.Fatal("invalid application or conflicting result was accepted")
					}
					_, vertex := value.ToInternal(out)
					bottom := adt.CombineErrors(nil, vertex.Bottom(), vertex.ChildErrors)
					if bottom == nil || bottom.IsIncomplete() {
						t.Fatalf("admitted=%v: unexpected diagnostic: %v", tc.member, bottom)
					}
					if err := v.LookupPath(cue.ParsePath("call")).Validate(); (err == nil) != tc.member {
						t.Fatalf("admitted=%v: call checking: %v", tc.member, err)
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
		`(func({a: 1}) -> string) & (func({}) -> (int|string))`,
		`(func({}) -> (int|string)) & (func({a: 1}) -> string)`,
	} {
		v := semanticValue(t, "f: "+contract+"\nout: f({}) & 0")
		if err := v.Validate(); err != nil {
			t.Fatalf("inapplicable clause constrained abstract result: %v", err)
		}
	}
	// The earlier version allowed this explicit interface. Its observations
	// conflict at {a: 1}, even though this particular call uses {}. Keep the
	// old program as a rejection case, and test abstract admission above with
	// compatible result clauses.
	v := semanticValue(t, "f: (func({a: 1})->string) & (func({})->int)\nout: f({}) & 0")
	if err := v.Validate(); err == nil {
		t.Fatal("an unobserved explicit overlap escaped relevance checking")
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
	if err := v.Validate(); err == nil {
		t.Fatal("call outside the declared domain was not blocked")
	}
	// The supplied body explicitly declares a wider domain. Recheck the
	// blocked judgment with that evidence, both by refinement and rebuilding.
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
out: bad()
`)
	if err := v.LookupPath(cue.ParsePath("out")).Validate(); err == nil {
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
				if err := v.Validate(); (err == nil) != required {
					t.Fatalf("definition check: executable callback=%v: %v", required, err)
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

// Memoizing a successful call may detach ground data from its activation, but
// cannot erase a subject's remaining type-selection obligations. Quantified
// arguments belong to the same type sort as ordinary data predicates.
func TestQuantifiedBoundaryCallSubjectPreservation(t *testing.T) {
	for _, subject := range []string{"1", "[1]", "{v: 1}"} {
		for _, wrap := range []string{"%s", "[%s]"} {
			for _, opaque := range []bool{false} {
				t.Run(fmt.Sprintf("%s/%s/opaque=%v", subject, wrap, opaque), func(t *testing.T) {
					call := "id(" + fmt.Sprintf(wrap, "c") + ")"
					selectSubject := "copied"
					if wrap != "%s" {
						selectSubject += "[0]"
					}
					v := semanticValue(t, fmt.Sprintf(`
c(A): %s
id: func(x: _) -> _: x
copied: %s
selected: %s[int]
polymorphic: %s[forall (X) func(X) -> X]
consumed: %s[int][bool]
`, subject, call, selectSubject, selectSubject, selectSubject))
					for _, path := range []string{"copied", "selected", "polymorphic"} {
						if err := v.LookupPath(cue.ParsePath(path)).Validate(cue.Concrete(true)); err != nil {
							t.Fatalf("%s: %v", path, err)
						}
					}
					if err := v.LookupPath(cue.ParsePath("consumed")).Validate(); err == nil {
						t.Fatal("a consumed binder became available for selection again")
					}
				})
			}
		}
	}
}
