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
	"fmt"
	"strings"
	"testing"

	"cuelang.org/go/internal/core/adt"
	"cuelang.org/go/internal/core/eval"
	"cuelang.org/go/internal/core/runtime"
	"cuelang.org/go/internal/core/subsume"
)

func TestCertificationSharedDependencies(t *testing.T) {
	for _, depth := range []int{5, 20, 60} {
		t.Run(fmt.Sprint(depth), func(t *testing.T) {
			ctx := eval.NewContext(runtime.New(), nil)
			var src strings.Builder
			src.WriteString("f0: func(x: int) -> int: x\n")
			for i := 1; i <= depth; i++ {
				fmt.Fprintf(&src, "f%d: func(x: int) -> int: f%d(f%d(x))\n", i, i-1, i-1)
			}
			root := parse(t, ctx, src.String())
			var f *adt.FuncValue
			for _, a := range root.Arcs {
				if a.Label.SelectorString(ctx) == fmt.Sprint("f", depth) {
					f, _ = adt.Unwrap(a).(*adt.FuncValue)
				}
			}
			if f == nil {
				t.Fatal("missing function")
			}
			budget := 100 * (depth + 1)
			if b, used := subsume.ValidateFunctionBudget(ctx, f, budget); b != nil {
				t.Fatalf("proof used %d of %d steps: %v", used, budget, b)
			}
			// A work limit suspends the proof, and is local to this attempt.
			if b, used := subsume.ValidateFunctionBudget(ctx, f, 10); b == nil || !b.IsIncomplete() || used != 10 {
				t.Fatalf("exhaustion: %v, used %d", b, used)
			}
			if b, _ := subsume.ValidateFunctionBudget(ctx, f, budget); b != nil {
				t.Fatalf("fresh proof after exhaustion: %v", b)
			}
		})
	}
}
