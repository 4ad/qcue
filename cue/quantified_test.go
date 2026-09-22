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

	"golang.org/x/tools/txtar"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/build"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/format"
)

func TestQuantifiedDataMeet(t *testing.T) {
	ctx := cuecontext.New()
	v := ctx.CompileString(quantifiedAPIText(t, "data_meet", "case01.cue"))
	for _, path := range []string{"impossible", "impossibleRecord"} {
		if err := v.LookupPath(cue.ParsePath(path)).Validate(); err == nil {
			t.Errorf("%s: missing empty-instance refutation", path)
		}
	}
	for path, want := range map[string]string{"empty": quantifiedAPIText(t, "data_meet", "case02.cue"), "optional": quantifiedAPIText(t, "data_meet", "case03.cue")} {
		got, err := v.LookupPath(cue.ParsePath(path)).MarshalJSON()
		if err != nil || string(got) != want {
			t.Errorf("%s: got %s, %v; want %s", path, got, err, want)
		}
	}
	if err := v.LookupPath(cue.ParsePath("union")).Unify(ctx.CompileString(quantifiedAPIText(t, "data_meet", "case04.cue"))).Err(); err == nil {
		t.Fatal("universal union lost its empty instance")
	}
}

func TestQuantifiedAbstractCall(t *testing.T) {
	ctx := cuecontext.New()
	v := ctx.CompileString(quantifiedAPIText(t, "abstract_call", "case01.cue"))
	if err := v.LookupPath(cue.ParsePath("bad")).Err(); err == nil {
		t.Fatal("symbolic call failed to propagate its result constraint")
	}
	if _, err := v.LookupPath(cue.ParsePath("r")).MarshalJSON(); err == nil {
		t.Fatal("an arrow hypothesis materialized a result without execution")
	}
	v = v.Unify(ctx.CompileString(quantifiedAPIText(t, "abstract_call", "case02.cue")))
	if got, err := v.LookupPath(cue.ParsePath("r")).Int64(); err != nil || got != 3 {
		t.Fatalf("supplied implementation: %d, %v", got, err)
	}
}

// The paper's self-unification examples exercise descriptor identity across
// copied environments, not just references to the same evaluator pointer.

func TestQuantifiedCaptureRefinement(t *testing.T) {
	ctx := cuecontext.New()
	v := ctx.CompileString(quantifiedAPIText(t, "capture_refinement", "case01.cue"))
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
		{quantifiedAPIText(t, "capture_refinement", "case02.cue"), false},
		{quantifiedAPIText(t, "capture_refinement", "case03.cue"), true},
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

func TestQuantifiedCapabilityGuardRefinement(t *testing.T) {
	ctx := cuecontext.New()
	v := ctx.CompileString(quantifiedAPIText(t, "capability_guard_refinement", "case01.cue"))
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

func TestQuantifiedExport(t *testing.T) {
	for _, tt := range []struct{ src, field, call, want string }{
		{quantifiedAPIText(t, "export", "case01.cue"), "id", quantifiedAPIText(t, "export", "case02.cue"), quantifiedAPIText(t, "export", "case03.cue")},
		{quantifiedAPIText(t, "export", "case04.cue"), "specialized", quantifiedAPIText(t, "export", "case05.cue"), quantifiedAPIText(t, "export", "case06.cue")},
		{quantifiedAPIText(t, "export", "case07.cue"), "id", quantifiedAPIText(t, "export", "case08.cue"), quantifiedAPIText(t, "export", "case09.cue")},
		{quantifiedAPIText(t, "export", "case10.cue"), "constant", quantifiedAPIText(t, "export", "case11.cue"), quantifiedAPIText(t, "export", "case12.cue")},
		{quantifiedAPIText(t, "export", "case13.cue"), "f", quantifiedAPIText(t, "export", "case14.cue"), quantifiedAPIText(t, "export", "case15.cue")},
		{quantifiedAPIText(t, "export", "case16.cue"), "f", quantifiedAPIText(t, "export", "case17.cue"), quantifiedAPIText(t, "export", "case18.cue")},
		{quantifiedAPIText(t, "export", "case19.cue"), "f", quantifiedAPIText(t, "export", "case20.cue"), quantifiedAPIText(t, "export", "case21.cue")},
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

func TestQuantifiedWitnessCorrelation(t *testing.T) {
	for _, tt := range []struct {
		name, src string
		good, bad int
		want      string
	}{
		{"guard", quantifiedAPIText(t, "witness_correlation", "case01.cue"), 0, 1, quantifiedAPIText(t, "witness_correlation", "case02.cue")},
		{"selector result", quantifiedAPIText(t, "witness_correlation", "case03.cue"), 1, 2, quantifiedAPIText(t, "witness_correlation", "case04.cue")},
		{"alias result", quantifiedAPIText(t, "witness_correlation", "case05.cue"), 1, 2, quantifiedAPIText(t, "witness_correlation", "case06.cue")},
		{"indexed result", quantifiedAPIText(t, "witness_correlation", "case07.cue"), 1, 2, quantifiedAPIText(t, "witness_correlation", "case08.cue")},
		{"explicit type argument", quantifiedAPIText(t, "witness_correlation", "case09.cue"), 1, 2, quantifiedAPIText(t, "witness_correlation", "case10.cue")},
		{"result", quantifiedAPIText(t, "witness_correlation", "case11.cue"), 1, 2, quantifiedAPIText(t, "witness_correlation", "case12.cue")},
		{"protocol", quantifiedAPIText(t, "witness_correlation", "case13.cue"), 1, 2, quantifiedAPIText(t, "witness_correlation", "case14.cue")},
		{"type bound", quantifiedAPIText(t, "witness_correlation", "case15.cue"), 1, 2, quantifiedAPIText(t, "witness_correlation", "case16.cue")},
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
	v := cuecontext.New().CompileString(quantifiedAPIText(t, "witness_correlation", "case17.cue"))
	if err := v.LookupPath(cue.ParsePath("f")).Validate(cue.VerifyFunctions(true)); err == nil {
		t.Fatal("a result singleton was certified from its upper approximation")
	}
}

func TestQuantifiedFileOrder(t *testing.T) {
	cases := [][]string{
		{quantifiedAPIText(t, "file_order", "case01.cue"),
			quantifiedAPIText(t, "file_order", "case02.cue"),
			quantifiedAPIText(t, "file_order", "case03.cue")},
		{quantifiedAPIText(t, "file_order", "case04.cue"), quantifiedAPIText(t, "file_order", "case05.cue"), quantifiedAPIText(t, "file_order", "case06.cue")},
	}
	want := []string{quantifiedAPIText(t, "file_order", "case07.cue"), quantifiedAPIText(t, "file_order", "case08.cue")}
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
		{"body capture", quantifiedAPIText(t, "closure_completeness", "case01.cue"), false},
		{"default capture", quantifiedAPIText(t, "closure_completeness", "case02.cue"), false},
		{"erased schema", quantifiedAPIText(t, "closure_completeness", "case03.cue"), true},
		{"erased schema alias", quantifiedAPIText(t, "closure_completeness", "case04.cue"), true},
		{"recursive descriptor", quantifiedAPIText(t, "closure_completeness", "case05.cue"), true},
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

func TestQuantifiedCompositeInstanceObligations(t *testing.T) {
	for _, src := range []string{
		quantifiedAPIText(t, "composite_instance_obligations", "case01.cue"),
		quantifiedAPIText(t, "composite_instance_obligations", "case02.cue"),
		quantifiedAPIText(t, "composite_instance_obligations", "case03.cue"),
		quantifiedAPIText(t, "composite_instance_obligations", "case04.cue"),
	} {
		t.Run(src, func(t *testing.T) {
			v := cuecontext.New().CompileString("@experiment(quantified)\n" + src)
			if err := v.LookupPath(cue.ParsePath("out")).Validate(); err == nil {
				t.Fatal("selection lost its subject or binder obligations")
			}
		})
	}
	v := cuecontext.New().CompileString(quantifiedAPIText(t, "composite_instance_obligations", "case05.cue"))
	out := v.LookupPath(cue.ParsePath("out"))
	if err := out.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := out.Validate(cue.Concrete(true)); err == nil {
		t.Fatal("selection chose an independent data witness")
	}
	got, err := v.FillPath(cue.ParsePath("module.value"), 7).LookupPath(cue.ParsePath("out")).Int64()
	if err != nil || got != 7 {
		t.Fatalf("subject refinement: %d, %v; want 7", got, err)
	}
}

// API-specific operations such as FillPath, Unify, syntax round trips, and
// file ordering remain Go assertions. Their programs live with the rest of
// the quantified corpus, in testdata/quantified/api.
func quantifiedAPIText(t *testing.T, name, section string) string {
	t.Helper()
	a, err := txtar.ParseFile("testdata/quantified/api/" + name + ".txtar")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range a.Files {
		if f.Name == section {
			return strings.TrimSpace(string(f.Data))
		}
	}
	t.Fatalf("missing %s in quantified API fixture %s", section, name)
	return ""
}
