// Copyright 2020 CUE Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
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
	"cuelang.org/go/pkg/internal/builtintest"
	"cuelang.org/go/pkg/list"
)

func TestBuiltin(t *testing.T) {
	builtintest.Run("list", t)
}

func TestContainsGoPredicate(t *testing.T) {
	ctx := cuecontext.New()
	if list.Contains(nil, cue.Value{}) || list.Contains([]cue.Value{{}}, ctx.CompileString("1")) ||
		list.Contains([]cue.Value{ctx.CompileString("1")}, cue.Value{}) {
		t.Fatal("an absent Go value was considered equal to a list element")
	}
	for _, tc := range []struct {
		name, item, target string
		want               bool
	}{
		{"equal", "1", "1", true},
		{"different", "1", "2", false},
		{"incomplete", "int", "1", false},
		{"equal_descriptions", "int", "int", false},
		{"incomplete_record", "{a:int}", "{a:1}", false},
		{"invalid", "{a:_|_}", "{a:1}", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := list.Contains([]cue.Value{ctx.CompileString(tc.item)}, ctx.CompileString(tc.target))
			if got != tc.want {
				t.Fatalf("Contains = %v; want %v", got, tc.want)
			}
		})
	}
}
