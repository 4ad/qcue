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

package list_test

import (
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/pkg/list"
)

func TestIsSortedGoPredicate(t *testing.T) {
	ctx := cuecontext.New()
	iter, err := ctx.CompileString(`[1,2]`).List()
	if err != nil {
		t.Fatal(err)
	}
	var values []cue.Value
	for iter.Next() {
		values = append(values, iter.Value())
	}
	for _, tc := range []struct {
		name, comparator string
		want             bool
	}{
		{"ascending", `{x:int,y:int,less:x<y}`, true},
		{"descending", `{x:int,y:int,less:x>y}`, false},
		{"incomplete", `{x:int,y:int,less:bool}`, false},
		{"invalid_result", `{x:int,y:int,less:42}`, false},
		{"missing_comparison", `{x:int,y:int}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := list.IsSorted(values, ctx.CompileString(tc.comparator)); got != tc.want {
				t.Fatalf("IsSorted = %v; want %v", got, tc.want)
			}
		})
	}
}
