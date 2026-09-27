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

package subsume_test

import (
	"testing"

	"cuelang.org/go/internal/core/adt"
	"cuelang.org/go/internal/core/eval"
	"cuelang.org/go/internal/core/runtime"
	"cuelang.org/go/internal/core/subsume"
)

func TestRelevanceSourceScopes(t *testing.T) {
	ctx := eval.NewContext(runtime.New(), nil)
	root := parse(t, ctx, `
meet(A, B): func(x: A, y: B) -> (A & B): x & y
selected: meet[int, bool]
partial: meet(1, ...)
combined: meet[int, bool] & meet[string, int]
explicit: func(int, bool) -> (int & bool)
Template(A, B) = func(A, B) -> (A & B)
aliased: Template(int, bool)
module(A, B): {meet: func(x: A, y: B) -> (A & B): x & y}
projected: module[int, bool].meet
`)
	seen := make(map[string]bool)
	for _, arc := range root.Arcs {
		name := arc.Label.SelectorString(ctx)
		if name == "module" || arc.Label.IsLet() {
			continue
		}
		f, ok := adt.Unwrap(arc).(*adt.FuncValue)
		if !ok {
			t.Fatalf("%s: missing function", name)
		}
		seen[name] = true
		clauses := f.ExplicitClauses()
		if len(clauses) == 0 {
			t.Fatalf("%s: no source clauses", name)
		}
		// Composite overlays can retain independently re-scoped copies.
		// Every copy must retain the original universal introduction.
		for _, clause := range clauses {
			var args []adt.Value
			for _, param := range adt.FunctionTypeParameters(clause) {
				args = append(args, &adt.RigidType{Param: param, Bound: &adt.Top{}})
			}
			if name == "explicit" || name == "aliased" {
				if len(args) != 0 {
					t.Fatalf("%s: alias did not substitute its explicit arguments", name)
				}
			} else if len(args) != 2 {
				t.Fatalf("%s: lost declaration telescope: %d parameters", name, len(args))
			}
			clause = adt.BindFunctionTypes(clause, args)
			err := subsume.CheckArrowRelevance(ctx, []adt.FuncType{clause})
			if (err != nil) != (name == "explicit" || name == "aliased") {
				t.Fatalf("%s: %v", name, err)
			}
		}
	}
	for _, name := range []string{"meet", "selected", "partial", "combined", "explicit", "aliased", "projected"} {
		if !seen[name] {
			t.Fatalf("missing test subject %s", name)
		}
	}
}

func TestRelevanceAccumulatesSourceDeclarations(t *testing.T) {
	ctx := eval.NewContext(runtime.New(), nil)
	root := parse(t, ctx, `
id(A): func(x: A) -> A: x
inline: id & (func(int) -> int)
separate: id
separate: func(int) -> int
selected: separate[int]
repeated: separate & separate
`)
	for _, arc := range root.Arcs {
		name := arc.Label.SelectorString(ctx)
		f, ok := adt.Unwrap(arc).(*adt.FuncValue)
		if !ok {
			t.Fatalf("%s: missing function", name)
		}
		want := 2
		if name == "id" {
			want = 1
		}
		if got := len(f.ExplicitClauses()); got != want {
			t.Fatalf("%s: %d source clauses, want %d", name, got, want)
		}
		if name == "selected" && len(f.Obligations()) <= want {
			t.Fatal("selection did not retain its derived obligations separately")
		}
	}
}

func TestRelevanceNewAnnotationAfterSelection(t *testing.T) {
	ctx := eval.NewContext(runtime.New(), nil)
	root := parse(t, ctx, `
meet(A, B): func(x: A, y: B) -> (A & B): x & y
f: meet[int, bool] & (func(int, bool) -> (int & bool))
`)
	for _, arc := range root.Arcs {
		if arc.Label.SelectorString(ctx) != "f" {
			continue
		}
		f, ok := adt.Unwrap(arc).(*adt.FuncValue)
		if !ok {
			t.Fatal("missing annotated subject")
		}
		clauses := f.ExplicitClauses()
		if len(clauses) != 2 {
			t.Fatalf("%d source clauses, want generic origin and new annotation", len(clauses))
		}
		ground := 0
		for _, clause := range clauses {
			if len(adt.FunctionTypeParameters(clause)) != 0 {
				continue
			}
			ground++
			if err := subsume.CheckArrowRelevance(ctx, []adt.FuncType{clause}); err == nil || err.Limit {
				t.Fatalf("explicit specialization must block independently: %v", err)
			}
		}
		if ground != 1 {
			t.Fatalf("%d explicit specializations, want 1", ground)
		}
		return
	}
	t.Fatal("missing test subject")
}
