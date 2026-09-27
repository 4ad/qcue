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
	"cuelang.org/go/internal/core/adt"
	"cuelang.org/go/internal/value"
)

func TestQuantifiedCallRequiresCertificate(t *testing.T) {
	for _, expr := range []string{
		`bad(1)`,
		`bad(1, ...)`,
		`bad(1) | 0`,
		`*0 | bad(1)`,
		`*bad(1) | 0`,
		`bad(1) == _|_`,
		`bad(1) != _|_`,
		`{field: bad(1)} == _|_`,
		`{field: bad(1)} != _|_`,
	} {
		t.Run(expr, func(t *testing.T) {
			v := cuecontext.New().CompileString(`
bad: func(x:int)->int: {value:"wrong"}.value
out: ` + expr)
			out := v.LookupPath(cue.ParsePath("out"))
			if err := out.Validate(); err == nil {
				t.Fatal("ordinary validation accepted an uncertified call")
			}
			if _, err := out.MarshalJSON(); err == nil {
				t.Fatal("an uncertified call produced a successful observation")
			}
			_, vertex := value.ToInternal(out)
			b := adt.CombineErrors(nil, vertex.Bottom(), vertex.ChildErrors)
			if b == nil || b.Code != adt.BlockedError {
				t.Fatalf("missing non-refuting static diagnostic: %v", b)
			}
		})
	}
}
