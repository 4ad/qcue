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
	"cuelang.org/go/cue/format"
)

func TestQuantifiedImpredicativeIdentity(t *testing.T) {
	for _, binder := range []string{"A", "A in Type", "A in Type(0)", "A in Type(3)"} {
		for _, instance := range []string{"id[Identity](id)", "id(id)", "id[id](id)"} {
			t.Run(binder+"/"+instance, func(t *testing.T) {
				prefix := ""
				if instance == "id[Identity](id)" {
					prefix = "let Identity = forall A func(A) -> A\n"
				}
				v := semanticValue(t, prefix+`id(`+binder+`): func(x: A) -> A: x
again: `+instance+`
out: [again[int](3), again[string]("three")]
`)
				if err := v.LookupPath(cue.ParsePath("again")).Validate(cue.Concrete(true)); err != nil {
					t.Fatalf("quantified instance did not certify: %v; root: %v", err, v.Err())
				}
				semanticJSON(t, v, "out", `[3,"three"]`)
				for _, options := range [][]cue.Option{nil, {cue.Final()}} {
					source, err := format.Node(v.LookupPath(cue.ParsePath("again")).Syntax(options...))
					if err != nil {
						t.Fatal(err)
					}
					rebuilt := semanticValue(t, "again: "+string(source)+"\nout: [again[int](3), again[string](\"three\")]")
					semanticJSON(t, rebuilt, "out", `[3,"three"]`)
				}
			})
		}
	}
}
