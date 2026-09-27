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
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
)

func TestQuantifiedIntegerIntervalCertificates(t *testing.T) {
	// The integer endpoints lie strictly inside this finite observation
	// window, so it distinguishes every inclusion between these half-lines.
	// Compare the checker with direct mathematical comparisons, including
	// fractional and negative endpoints and both open and closed bounds.
	compare := func(op string, value, endpoint float64) bool {
		switch op {
		case ">":
			return value > endpoint
		case ">=":
			return value >= endpoint
		case "<":
			return value < endpoint
		default:
			return value <= endpoint
		}
	}
	for _, a := range []float64{-2, -1.5, 0, 1.25, 2} {
		for _, b := range []float64{-2, -1.5, 0, 1.25, 2} {
			for _, input := range []string{">", ">=", "<", "<="} {
				for _, result := range []string{">", ">=", "<", "<="} {
					want := true
					for n := -4; n <= 4; n++ {
						want = want && (!compare(input, float64(n), a) || compare(result, float64(n), b))
					}
					source := fmt.Sprintf("f: func(x: int & %s (%v)) -> (int & %s (%v)): x", input, a, result, b)
					v := cuecontext.New().CompileString(source).LookupPath(cue.ParsePath("f"))
					if err := v.Validate(cue.Concrete(true)); (err == nil) != want {
						t.Fatalf("%s: inclusion=%v: %v", source, want, err)
					}
				}
			}
		}
	}
	for _, tt := range []struct {
		source string
		valid  bool
	}{
		{`f:func(x:int&>0)->(int&>=0):x-1`, true},
		{`f:func(x:int&<0)->(int&<=0):x+1`, true},
		{`f:func(x:int&>0)->(int&>0):x-1`, false},
		{`f:func(x:number&>0)->(number&>=0):x-1`, false},
		{`f:func(x:int&>99999999999999999999999999999999999)->(int&>=100000000000000000000000000000000000):x`, true},
	} {
		v := cuecontext.New().CompileString(tt.source).LookupPath(cue.ParsePath("f"))
		if err := v.Validate(cue.Concrete(true)); (err == nil) != tt.valid {
			t.Errorf("%s: valid=%v: %v", tt.source, tt.valid, err)
		}
	}
}
