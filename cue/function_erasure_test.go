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
)

// A result annotation is a proposition about the independently evaluated
// body. It cannot supply fields, choose alternatives, or solve body cycles.
func TestQuantifiedResultErasure(t *testing.T) {
	for _, tt := range []struct{ name, source string }{
		{"inferred", `f(A): func(xs: [...A]) -> {b: A}: {a: xs[0], c: self.b}
out: f([1, 2, 3])`},
		{"selected", `f(A): func() -> {b: A}: {c: self.b}
out: f[1]()`},
		{"universal", `f(A): func(x: A) -> {a: 1}: x
out: f({x: "x"})`},
		{"field", `out: (func() -> {a: 1}: {})()`},
		{"nested", `out: (func() -> {n: {a: 1}}: {n: {}})()`},
		{"list", `out: (func() -> [{a: 1}]: [{}])()`},
		{"choice", `out: (func() -> 1: 1 | 2)()`},
		{"default", `out: (func() -> (*1 | int): int)()`},
		{"cycle", `out: (func() -> {a: 1}: {a: self.a})()`},
		{"attached", `f: (func() -> {}: {}) & (func() -> {a: 1})
out: f()`},
		{"partial", `f: (func(x: int) -> {}: {})(1, ...) & (func() -> {a: 1})
out: f()`},
		{"builtin", `import "encoding/json"
f: json.Unmarshal & (func(string) -> {a: 1})
out: f("{}")`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			v := semanticValue(t, tt.source)
			out := v.LookupPath(cue.ParsePath("out"))
			if err := out.Validate(cue.Concrete(true)); err == nil {
				t.Fatal("result annotation constructed a successful result")
			}
			if b, err := out.MarshalJSON(); err == nil {
				t.Fatalf("result annotation materialized %s", b)
			}
		})
	}
}
