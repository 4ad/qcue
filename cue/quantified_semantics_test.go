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
	"cuelang.org/go/cue/build"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/format"
)

func semanticValue(t *testing.T, source string) cue.Value {
	t.Helper()
	return cuecontext.New().CompileString("@experiment(quantified)\n" + source)
}

func semanticJSON(t *testing.T, v cue.Value, path, want string) {
	t.Helper()
	v = v.LookupPath(cue.ParsePath(path))
	got, err := v.MarshalJSON()
	if err != nil || string(got) != want {
		t.Fatalf("%s: got %s, %v; want %s", path, got, err, want)
	}
}

// Checked selections retain separate call views of one operational closure.
// Combining those views must preserve both admitted domains, independently of
// conjunction order, without reopening the consumed selection telescope.
func TestQuantifiedSemanticSelectedMeet(t *testing.T) {
	for _, expr := range []string{
		`id[int] & id[string]`,
		`id[string] & id[int]`,
		`(id[int] & id[string]) & id[int]`,
		`id[int] & (id[string] & id[int])`,
	} {
		t.Run(expr, func(t *testing.T) {
			v := semanticValue(t, `id(A): func(x: A) -> A: x
f: `+expr+`
out: [f(1), f("s"), f("s", ...)()]
bad: f(true)
extra: f[bool]
singleBad: id[int]("s")`)
			semanticJSON(t, v, "out", `[1,"s","s"]`)
			for _, path := range []string{"bad", "extra", "singleBad"} {
				if v.LookupPath(cue.ParsePath(path)).Validate() == nil {
					t.Fatalf("%s lost its checked instance boundary", path)
				}
			}
			for _, options := range [][]cue.Option{nil, {cue.Final()}} {
				source, err := format.Node(v.LookupPath(cue.ParsePath("f")).Syntax(options...))
				if err != nil {
					t.Fatal(err)
				}
				rebuilt := semanticValue(t, "f: "+string(source)+"\nout: [f(1), f(\"s\")]\nbad: f(true)\nextra: f[bool]")
				semanticJSON(t, rebuilt, "out", `[1,"s"]`)
				for _, path := range []string{"bad", "extra"} {
					if rebuilt.LookupPath(cue.ParsePath(path)).Validate() == nil {
						t.Fatalf("export lost %s: %s", path, source)
					}
				}
			}
		})
	}
	for _, expr := range []string{`pair[int] & pair[string]`, `pair[string] & pair[int]`} {
		v := semanticValue(t, `pair(A, B): func(x: A, y: B) -> [A, B]: [x, y]
f: (`+expr+`)[bool]
out: [f(1, true), f("s", false), f("s", ...)(true)]
extra: f[int]`)
		semanticJSON(t, v, "out", `[[1,true],["s",false],["s",true]]`)
		if v.LookupPath(cue.ParsePath("extra")).Validate() == nil {
			t.Fatal("merged selection reopened a consumed binder")
		}
	}
	// On overlapping domains, every admitted result predicate is required,
	// even when the first checked view alone would permit the returned value.
	for _, expr := range []string{`bad[0|1] & bad[0]`, `bad[0] & bad[0|1]`} {
		v := semanticValue(t, `bad(A): func(x: A) -> A: 1
f: `+expr+`
out: f(0)`)
		if v.LookupPath(cue.ParsePath("out")).Validate() == nil {
			t.Fatal("overlapping selected result obligation was dropped")
		}
	}
}

// Conformance of a supplied callback cannot depend on whether its field is
// emitted by JSON, or on whether the callee happens to invoke it once.
func TestQuantifiedSemanticCallbackObservation(t *testing.T) {
	for _, label := range []string{"cb", "_cb"} {
		for _, nested := range []bool{false, true} {
			for _, invoke := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/nested=%v/invoke=%v", label, nested, invoke), func(t *testing.T) {
					predicate := fmt.Sprintf("{%s: func(int) -> 1}", label)
					packet := fmt.Sprintf("{%s: cb}", label)
					access := "arg."
					if nested {
						predicate, packet, access = "{n: "+predicate+"}", "{n: "+packet+"}", "arg.n."
					}
					body := "0"
					if invoke {
						body = access + label + "(1)"
					}
					v := semanticValue(t, fmt.Sprintf(`
cb: func(x: int) -> int: {v: x}.v
f: func(arg: %s) -> int: %s
out: f(%s)
`, predicate, body, packet))
					if err := v.Validate(cue.Concrete(true)); err == nil {
						t.Fatal("certified invalid callback promise")
					}
					if _, err := v.LookupPath(cue.ParsePath("out")).MarshalJSON(); err == nil {
						t.Fatal("call result lost the callback obligation")
					}
				})
			}
		}
	}
	t.Run("builtin", func(t *testing.T) {
		v := semanticValue(t, `import "strings"
f: func(arg: {_cb: func(string) -> "yes"}) -> int: 0
out: f({_cb: strings.ToUpper})`)
		if err := v.Validate(cue.Concrete(true)); err == nil {
			t.Fatal("certified invalid builtin promise")
		}
	})
}

func TestQuantifiedSemanticExistentialFields(t *testing.T) {
	for _, label := range []string{"x", "_x", "#X", "_#X"} {
		for _, nested := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/nested=%v", label, nested), func(t *testing.T) {
				predicate, subject := fmt.Sprintf("{%s: 1}", label), fmt.Sprintf("{%s: 2}", label)
				if nested {
					predicate, subject = "{n:"+predicate+"}", "{n:"+subject+"}"
				}
				v := semanticValue(t, fmt.Sprintf("v: (exists A %s) & %s", predicate, subject))
				if err := v.Validate(); err == nil {
					t.Fatal("constant existential discarded a conflicting field")
				}
			})
		}
	}
}

func TestQuantifiedSemanticGuardPresence(t *testing.T) {
	for _, label := range []string{"required", "_required", "#Required", "_#Required"} {
		t.Run(label, func(t *testing.T) {
			v := semanticValue(t, fmt.Sprintf(`
f: (func(x: {}) -> int: {
    if x.%[1]s == _|_ {v: 0}
    if x.%[1]s != _|_ {v: 1}
}.v) & (func({%[1]s: 1}) -> 1)
out: f({})
`, label))
			semanticJSON(t, v, "out", "0")
		})
	}
}

func TestQuantifiedSemanticOpaqueOverlap(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		t.Run(fmt.Sprint(reverse), func(t *testing.T) {
			a, b := "func(x!: int) -> int", "func(x: int) -> A"
			if reverse {
				a, b = b, a
			}
			v := semanticValue(t, fmt.Sprintf(`
#M: exists A {f: (%s) & (%s)}
p: seal #M with (A = int) {f: func(x: int) -> int: x}
`, a, b))
			if err := v.Validate(cue.Concrete(true)); err == nil {
				t.Fatal("certified incompatible transports on packet (x: 1)")
			}
		})
	}
}

func TestQuantifiedSemanticOpaqueExportIdentity(t *testing.T) {
	for _, inline := range []bool{false, true} {
		for _, shared := range []bool{false, true} {
			for _, alias := range []bool{false, true} {
				t.Run(fmt.Sprintf("inline=%v/shared=%v/alias=%v", inline, shared, alias), func(t *testing.T) {
					f := "#F"
					if inline {
						f = "func(int) -> int"
					}
					g := f
					if alias {
						g = "f"
					}
					implementation := "{f: h, g: h}"
					if !shared {
						implementation = "{f: func(x: int) -> int: x, g: func(x: int) -> int: x}"
					}
					v := semanticValue(t, fmt.Sprintf(`
#F: func(int) -> int
#M: exists A {f: %s, g: %s}
h: func(x: int) -> int: x
p: seal #M with (A = int) %s
out: (open p as (A, P) {result: (P.f & P.g)(1)}).result
`, f, g, implementation))
					if alias && !shared {
						if err := v.Validate(); err == nil {
							t.Fatal("distinct implementations satisfied a declared alias")
						}
					} else if alias {
						semanticJSON(t, v, "out", "1")
					} else if err := v.LookupPath(cue.ParsePath("out")).Validate(); err == nil {
						t.Fatal("independent public exports acquired equal identity")
					}
				})
			}
		}
	}
}

func TestQuantifiedSemanticScalarSelection(t *testing.T) {
	for _, body := range []string{"1", `"x"`, "true", "null", "{n: 1}", "[1]"} {
		t.Run(body, func(t *testing.T) {
			v := semanticValue(t, fmt.Sprintf("c(A): %s\nout: c[int]", body))
			want, err := semanticValue(t, "out: "+body).LookupPath(cue.ParsePath("out")).MarshalJSON()
			if err != nil {
				t.Fatal(err)
			}
			semanticJSON(t, v, "out", string(want))
			v = semanticValue(t, fmt.Sprintf(`
poly: forall (A in Type(0)) %s
f(T in Type(0)): func(x: T) -> T: x
out: f[poly](%s)
`, body, body))
			if err := v.LookupPath(cue.ParsePath("out")).Validate(); err == nil {
				t.Fatal("normalization lowered a quantified argument to Type(0)")
			}
		})
	}
}

func TestQuantifiedSemanticNestedSelection(t *testing.T) {
	for _, names := range [][2]string{{"A", "B"}, {"Outer", "Inner"}} {
		for _, reverse := range []bool{false, true} {
			t.Run(fmt.Sprintf("%v/reverse=%v", names, reverse), func(t *testing.T) {
				a := fmt.Sprintf("r(%s): {f(%s): func(%s) -> %s}", names[0], names[1], names[0], names[0])
				b := "r: {f: func(x: _) -> _: x}"
				if reverse {
					a, b = b, a
				}
				v := semanticValue(t, a+"\n"+b+"\nout: r[int].f[string](1)")
				semanticJSON(t, v, "out", "1")
			})
		}
	}
}

// Selecting or normalizing a value does not erase its binder domain. The
// exporter must preserve this under its ordinary and final output profiles.
func TestQuantifiedSemanticSelectionRoundTrip(t *testing.T) {
	for _, declaration := range []string{
		"c(A): 1",
		`c(A): "x"`,
		"c(A): true",
		"c(A): null",
		"c(A): 1 | 2",
		"c(A): {n: 1}",
		"c(A): [1]",
		"c: forall (a: 1 | 2) 1",
	} {
		for _, final := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/final=%v", declaration, final), func(t *testing.T) {
				v := semanticValue(t, declaration)
				var opts []cue.Option
				if final {
					opts = append(opts, cue.Final())
				}
				text, err := format.Node(v.LookupPath(cue.ParsePath("c")).Syntax(opts...))
				if err != nil {
					t.Fatal(err)
				}
				arg := "int"
				if declaration == "c: forall (a: 1 | 2) 1" {
					arg = "1"
				}
				selected := "c[" + arg + "]"
				if declaration == "c(A): 1 | 2" {
					selected = "(" + selected + " & 1)"
				}
				rebuilt := semanticValue(t, "c: "+string(text)+"\nselected: "+selected+"\nbad: "+selected+"[bool]")
				if err := rebuilt.LookupPath(cue.ParsePath("selected")).Validate(); err != nil {
					t.Fatalf("export %s lost elimination: %v", text, err)
				}
				if err := rebuilt.LookupPath(cue.ParsePath("bad")).Validate(); err == nil {
					t.Fatalf("export %s restarted a consumed binder", text)
				}
			})
		}
	}
	for _, container := range []string{"record", "list"} {
		for _, final := range []bool{false, true} {
			t.Run(fmt.Sprintf("method/%s/final=%v", container, final), func(t *testing.T) {
				source := "r(A): {f(B): func(A) -> A}\nr: {f: func(x: _) -> _: x}\nselected: r[int].f"
				if container == "list" {
					source = "r(A): [forall B func(A) -> A]\nr: [func(x: _) -> _: x]\nselected: r[int][0]"
				}
				v := semanticValue(t, source)
				var opts []cue.Option
				if final {
					opts = append(opts, cue.Final())
				}
				text, err := format.Node(v.LookupPath(cue.ParsePath("selected")).Syntax(opts...))
				if err != nil {
					t.Fatal(err)
				}
				rebuilt := semanticValue(t, "f: "+string(text)+"\nout: f[string](1)\nbad: f[string][bool]")
				t.Logf("exported method: %s", text)
				semanticJSON(t, rebuilt, "out", "1")
				if err := rebuilt.LookupPath(cue.ParsePath("bad")).Validate(); err == nil {
					t.Fatalf("export %s restarted a consumed method binder", text)
				}
			})
		}
	}
}

// This finite oracle describes records as two independent presence bits and
// values in {0,1}, or {0,1,2} in extended mode. Predicate masks are sets of
// permitted values; an absent required field is unresolved in an open refinable
// subject. It does not use
// the evaluator's unifier, shape checker, or validator to derive expectations.
// The same denotation is then checked through each CUE field class and both
// conjunction orders.
func TestQuantifiedSemanticMembershipModel(t *testing.T) {
	width := 2
	if extendedQuantifiedOracle(t) {
		width = 3
	}
	cases := 0
	for _, label := range []string{"x", "_x", "#X", "_#X"} {
		t.Run(label, func(t *testing.T) {
			ctx := cuecontext.New()
			for maskX := 1; maskX < 1<<width; maskX++ {
				for maskY := 1; maskY < 1<<width; maskY++ {
					for optional := range 4 {
						fields := []string{label, "y"}
						masks := []int{maskX, maskY}
						pred := make([]string, 2)
						for i, field := range fields {
							mark := ""
							if optional&(1<<i) != 0 {
								mark = "?"
							}
							values := []string{}
							for value := range width {
								if masks[i]&(1<<value) != 0 {
									values = append(values, fmt.Sprint(value))
								}
							}
							pred[i] = field + mark + ": " + strings.Join(values, " | ")
						}
						predicate := "(exists A {" + strings.Join(pred, ", ") + "})"
						for x := -1; x < width; x++ {
							for y := -1; y < width; y++ {
								conflict, complete := false, true
								var entries []string
								for i, value := range []int{x, y} {
									if value < 0 {
										complete = complete && optional&(1<<i) != 0
									} else {
										conflict = conflict || masks[i]&(1<<value) == 0
										entries = append(entries, fmt.Sprintf("%s: %d", fields[i], value))
									}
								}
								subject := "{" + strings.Join(entries, ", ") + "}"
								for _, reverse := range []bool{false, true} {
									a, b := predicate, subject
									if reverse {
										a, b = b, a
									}
									source := "@experiment(quantified)\nv: " + a + " & " + b
									if cases%256 == 0 {
										ctx = cuecontext.New()
									}
									cases++
									v := ctx.CompileString(source)
									if err := v.Validate(); (err != nil) != conflict {
										t.Fatalf("%s: conflict=%v; got %v", source, conflict, err)
									}
									if err := v.Validate(cue.Concrete(true)); (err == nil) != (complete && !conflict) {
										t.Fatalf("%s: complete=%v conflict=%v; got %v", source, complete, conflict, err)
									}
								}
							}
						}
					}
				}
			}
		})
	}
	t.Logf("checked %d membership cases over %d values", cases, width)
}

func TestQuantifiedSemanticMembershipRefinement(t *testing.T) {
	for _, label := range []string{"x", "_x", "#X", "_#X"} {
		for _, nested := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/nested=%v", label, nested), func(t *testing.T) {
				ctx := cuecontext.New()
				record := func(value string) string {
					x := "{" + label + ": " + value + "}"
					if nested {
						x = "{n: " + x + "}"
					}
					return x
				}
				v := ctx.CompileString("@experiment(quantified)\nv: (exists A " + record("1") + ") & " + record("int"))
				if err := v.Validate(); err != nil {
					t.Fatal(err)
				}
				for _, good := range []bool{false, true} {
					value := "2"
					if good {
						value = "1"
					}
					for _, fill := range []bool{false, true} {
						var result cue.Value
						if fill {
							result = v.FillPath(cue.ParsePath("v"), ctx.CompileString(record(value)))
						} else {
							result = v.Unify(ctx.CompileString("v: " + record(value)))
						}
						if err := result.Validate(cue.Concrete(true)); (err == nil) != good {
							t.Fatalf("good=%v FillPath=%v: %v", good, fill, err)
						}
					}
				}
			})
		}
	}
}

func TestQuantifiedSemanticOpaqueGraph(t *testing.T) {
	for _, shared := range []bool{false, true} {
		for _, reverse := range []bool{false, true} {
			t.Run(fmt.Sprintf("shared=%v/reverse=%v", shared, reverse), func(t *testing.T) {
				privateB := "h"
				if !shared {
					privateB = "func(x: int) -> int: x"
				}
				fields := "a: #N, b: #N, copy: a, alias: a.f"
				if reverse {
					fields = "alias: a.f, copy: a, b: #N, a: #N"
				}
				source := fmt.Sprintf(`
#F: func(int) -> int
#N: {f: #F, g: f}
#M: exists A {%s}
h: func(x: int) -> int: x
p: seal #M with (A = int) {
    a: {f: h, g: h}
    b: {f: %s, g: f}
    copy: a
    alias: a.f
}
good: (open p as (A, P) {out: [(P.a.f & P.a.g)(1), (P.a.f & P.copy.f)(2), (P.a.f & P.alias)(3)]}).out
bad: (open p as (A, P) {out: (P.a.f & P.b.f)(1)}).out
`, fields, privateB)
				v := semanticValue(t, source)
				semanticJSON(t, v, "good", "[1,2,3]")
				if err := v.LookupPath(cue.ParsePath("bad")).Validate(); err == nil {
					t.Fatal("reusing a record predicate merged distinct export paths")
				}
			})
		}
	}
}

func TestQuantifiedSemanticRuntimeBoundaries(t *testing.T) {
	for _, good := range []bool{false, true} {
		for _, boundary := range []string{"packet", "capture", "package"} {
			t.Run(fmt.Sprintf("%s/good=%v", boundary, good), func(t *testing.T) {
				ret := "1"
				if good {
					ret = "int"
				}
				var source string
				switch boundary {
				case "packet":
					source = fmt.Sprintf("cb: func(x: int) -> int: {v: x}.v\nf: func(arg: {_cb: func(int) -> %s}) -> int: 0\nout: f({_cb: cb})", ret)
				case "capture":
					source = fmt.Sprintf("obj: {_cb: (func(x: int) -> int: {v: x}.v) & func(int) -> %s}\nf: func() -> int: obj._cb(1)", ret)
				case "package":
					source = fmt.Sprintf("#M: exists A {_f: func(int) -> %s}\np: seal #M with (A = int) {_f: func(x: int) -> int: {v: x}.v}", ret)
				}
				v := semanticValue(t, source)
				if err := v.Validate(cue.Concrete(true)); (err == nil) != good {
					t.Fatalf("boundary conformance good=%v: %v", good, err)
				}
			})
		}
	}
}

// Exhaust the truth tables of binary relations on {0,1}, both quantifier
// prefixes and all subdomains (including empty domains). This oracle evaluates
// finite meets and joins with ordinary Go booleans, independently of ADT
// normalization, environments and proof search.
func TestQuantifiedSemanticFiniteQuantifierModel(t *testing.T) {
	domain := func(mask int) string {
		var values []string
		for i := range 2 {
			if mask&(1<<i) != 0 {
				values = append(values, fmt.Sprint(i))
			}
		}
		if len(values) == 0 {
			return "_|_"
		}
		return strings.Join(values, " | ")
	}
	quantify := func(universal bool, mask int, body func(int) bool) bool {
		result := universal
		for i := range 2 {
			if mask&(1<<i) != 0 {
				if universal {
					result = result && body(i)
				} else {
					result = result || body(i)
				}
			}
		}
		return result
	}
	ctx := cuecontext.New()
	for relation := range 16 {
		var terms []string
		for x := range 2 {
			for y := range 2 {
				if relation&(1<<(2*x+y)) != 0 {
					terms = append(terms, fmt.Sprintf("(x == %d && y == %d)", x, y))
				}
			}
		}
		body := "false"
		if len(terms) > 0 {
			body = strings.Join(terms, " || ")
		}
		for outer := range 4 {
			for inner := range 4 {
				for _, qx := range []string{"forall", "exists"} {
					for _, qy := range []string{"forall", "exists"} {
						want := quantify(qx == "forall", outer, func(x int) bool {
							return quantify(qy == "forall", inner, func(y int) bool {
								return relation&(1<<(2*x+y)) != 0
							})
						})
						source := fmt.Sprintf("@experiment(quantified)\nv: (%s (x in %s) %s (y in %s) {ok: true & (%s)}) & {ok: true}", qx, domain(outer), qy, domain(inner), body)
						v := ctx.CompileString(source)
						if err := v.Validate(); (err == nil) != want {
							t.Fatalf("finite denotation=%v: %s\n%v", want, source, err)
						}
						if err := v.Validate(cue.Concrete(true)); (err == nil) != want {
							t.Fatalf("finite observation=%v: %s\n%v", want, source, err)
						}
					}
				}
			}
		}
	}
}

func TestQuantifiedSemanticFormationPreservation(t *testing.T) {
	for _, expression := range []string{
		"poly", "poly & 1", "1 & poly", "poly | 1", "1 | poly",
		"({p: poly}).p", "[poly][0]", "poly[int]", "(poly & 1)[int]",
	} {
		t.Run(expression, func(t *testing.T) {
			v := semanticValue(t, fmt.Sprintf(`
poly: forall (A in Type(0)) 1
f(T in Type(0)): func(x: T) -> T: x
out: f[%s](1)
`, expression))
			if err := v.LookupPath(cue.ParsePath("out")).Validate(); err == nil {
				t.Fatal("copying or normalizing a subject erased its universe obligation")
			}
		})
	}
}

func TestQuantifiedSemanticMembershipConstraints(t *testing.T) {
	for _, source := range []string{
		`v: (exists A {x: 1}) & close({})`,
		`v: (exists A {x: 1}) & {[string]: string}`,
		`v: (exists A {x: 1}) & close({[=~"^y"]: int})`,
		`v: (exists A [int, string]) & [1, ...int]`,
		`v: (exists A [int, string]) & [1]`,
	} {
		t.Run(source, func(t *testing.T) {
			if err := semanticValue(t, source).Validate(); err == nil {
				t.Fatal("membership lost a pattern, list tail, or closedness constraint")
			}
		})
	}
	ctx := cuecontext.New()
	v := ctx.CompileString("@experiment(quantified)\nv: (exists A {x?: 1}) & {x?: 2}")
	if err := v.Validate(cue.Concrete(true)); err != nil {
		t.Fatalf("absent optional field: %v", err)
	}
	if err := v.FillPath(cue.ParsePath("v.x"), 1).Validate(); err == nil {
		t.Fatal("later presence discarded the original optional constraint")
	}
	v = ctx.CompileString("@experiment(quantified)\nv: (exists A [int, string]) & [1, ...]")
	if err := v.Validate(); err != nil {
		t.Fatalf("refinable list: %v", err)
	}
	if err := v.Validate(cue.Concrete(true)); err == nil {
		t.Fatal("membership manufactured a missing list element")
	}
	if err := v.Unify(ctx.CompileString(`v: [1, "x"]`)).Validate(cue.Concrete(true)); err != nil {
		t.Fatalf("refined list: %v", err)
	}
}

func TestQuantifiedSemanticQualifiedLabels(t *testing.T) {
	ctx := cuecontext.New()
	compile := func(path, name, source string) cue.Value {
		instance := &build.Instance{ImportPath: path, PkgName: name}
		if err := instance.AddFile(name+".cue", "@experiment(quantified)\npackage "+name+"\n"+source); err != nil {
			t.Fatal(err)
		}
		return ctx.BuildInstance(instance)
	}
	p := compile("first.test/p", "p", "constraint: exists A {_x: 1}\ngood: {_x: 1}")
	q := compile("second.test/q", "q", "data: {_x: 2}")
	v := p.LookupPath(cue.ParsePath("constraint")).Unify(q.LookupPath(cue.ParsePath("data")))
	if err := v.Validate(); err != nil {
		t.Fatalf("distinct qualified hidden labels conflicted: %v", err)
	}
	if err := v.Validate(cue.Concrete(true)); err == nil {
		t.Fatal("a different package's hidden field supplied the witness")
	}
	v = v.Unify(p.LookupPath(cue.ParsePath("good")))
	if err := v.Validate(cue.Concrete(true)); err != nil {
		t.Fatalf("qualified witness: %v", err)
	}
	for path, want := range map[string]int64{"first.test/p": 1, "second.test/q": 2} {
		got, err := v.LookupPath(cue.MakePath(cue.Hid("_x", path))).Int64()
		if err != nil || got != want {
			t.Fatalf("hidden field of %s: %d, %v", path, got, err)
		}
	}
}
