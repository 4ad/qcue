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
