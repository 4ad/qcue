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

func TestQuantifiedPackageRefinementIdentity(t *testing.T) {
	// A copy can be refined in a different lexical environment or API
	// operation. Its source contains a seal, but the copy is already a
	// constructed package and must not evaluate that constructor again.
	ctx := cuecontext.New()
	v := ctx.CompileString(`
#I: exists A {tag: 1, optional?: int}
p: {
	result: implementation
	implementation: seal #I with (A = int) {tag: 1}
}.result & {[=~"^x"]: >=0}
copy: p & {extra: 2}
same: p & copy
out: (open same as (A, P) {result: P.extra}).result
`)
	if got, err := v.LookupPath(cue.ParsePath("out")).Int64(); err != nil || got != 2 {
		t.Fatalf("copy reconstructed its seal: got %d, %v", got, err)
	}
	p := v.LookupPath(cue.ParsePath("p"))
	if err := p.Validate(cue.Concrete(true)); err != nil {
		t.Fatal(err)
	}
	if err := p.Unify(ctx.CompileString("{extra: 2}")).Unify(p).Validate(); err != nil {
		t.Fatalf("later refinement reconstructed its seal: %v", err)
	}
	for _, bad := range []string{`{optional: "bad"}`, `{x: -1}`} {
		if err := p.Unify(ctx.CompileString(bad)).Validate(); err == nil {
			t.Fatalf("copy lost its public constraints: %s", bad)
		}
	}
}
