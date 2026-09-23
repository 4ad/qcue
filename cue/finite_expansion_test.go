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

func TestQuantifiedFiniteExpansionBudget(t *testing.T) {
	prefix := func(kind, name string, count int) string {
		params := make([]string, count)
		for i := range params {
			params[i] = fmt.Sprintf("%s%d in 0 | 1", name, i)
		}
		return kind + " (" + strings.Join(params, ", ") + ") "
	}
	for _, source := range []string{
		prefix("exists", "n", 20) + "n0",
		prefix("forall", "n", 20) + "(n0 | 1)",
		prefix("exists", "n", 10) + prefix("exists", "m", 10) + "(n0 | m0)",
		`exists (n in 0 | 1, A) {value: n}`,
	} {
		t.Run(source[:min(len(source), 40)], func(t *testing.T) {
			ctx := cuecontext.New()
			v := ctx.CompileString("large: " + source + "\nsmall: (exists (n in 1 | 2) n) & 2")
			large := v.LookupPath(cue.ParsePath("large"))
			if err := large.Validate(); err != nil {
				t.Fatalf("budget exhaustion became a contradiction: %v", err)
			}
			if err := large.Validate(cue.Concrete(true)); err == nil {
				t.Fatal("incomplete enumeration certified a result")
			}
			if got, err := v.LookupPath(cue.ParsePath("small")).Int64(); err != nil || got != 2 {
				t.Fatalf("unrelated obligation lost its budget: %d, %v", got, err)
			}
			for _, opts := range [][]cue.Option{nil, {cue.Final()}} {
				text, err := format.Node(large.Syntax(opts...))
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(string(text), "exists") && !strings.Contains(string(text), "forall") {
					t.Fatalf("enumeration lost its residual: %s", text)
				}
				rebuilt := ctx.CompileString("large: " + string(text)).LookupPath(cue.ParsePath("large"))
				if err := rebuilt.Validate(); err != nil {
					t.Fatalf("residual round trip: %s: %v", text, err)
				}
				// A branch not reached before exhaustion must remain possible.
				refinement := "1"
				if strings.Contains(source, "{value:") {
					refinement = "{value: 3}"
				}
				r := rebuilt.Unify(ctx.CompileString(refinement))
				if err := r.Validate(); err != nil {
					t.Fatalf("residual spuriously refuted refinement: %v", err)
				}
				if err := r.Validate(cue.Concrete(true)); err == nil {
					t.Fatal("residual accepted a witness without a proof")
				}
			}
		})
	}
	for _, kind := range []string{"exists", "forall"} {
		v := cuecontext.New().CompileString("out: " + prefix(kind, "n", 120) + "1")
		if got, err := v.LookupPath(cue.ParsePath("out")).Int64(); err != nil || got != 1 {
			t.Fatalf("constant %s: %d, %v", kind, got, err)
		}
	}
}
