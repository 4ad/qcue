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
	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/format"
	"strings"
	"testing"
)

func TestQuantifiedNestedPackageTransport(t *testing.T) {
	for _, element := range []string{"{tag: 1}", "{tag: 1, value: B}"} {
		v := cuecontext.New().CompileString(`
#Inner: exists B ` + element + `
#Outer: exists A {inner: #Inner, list: [...#Inner]}
p: seal #Inner with (B = int) {tag: 1, value: 2}
q: seal #Outer with (A = int) {inner: p, list: [p]}
copy: (open q as (A, Q) {result: Q.inner}).result
listed: (open q as (A, Q) {result: Q.list[0]}).result
`)
		for _, path := range []string{"p", "copy", "listed"} {
			x := v.LookupPath(cue.ParsePath(path))
			if err := x.Validate(cue.Concrete(true)); err != nil {
				t.Fatalf("%s: %v", path, err)
			}
			src, err := format.Node(x.Syntax(cue.Final()))
			if err == nil && !strings.Contains(string(src), "opaque boundaries") {
				t.Fatalf("%s lost its package boundary: %s", path, src)
			}
		}
	}
}
