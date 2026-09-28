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

func TestStdlibErrorResult(t *testing.T) {
	const uuid = `"00000000-0000-0000-0000-000000000000"`
	v := cuecontext.New().CompileString(`import "uuid"
direct:uuid.Valid(` + uuid + `)
constraint:` + uuid + ` & uuid.Valid
f:func(s:string)->true:uuid.Valid(s)
out:f(` + uuid + `)
`)
	if err := v.Validate(); err != nil {
		t.Fatal(err)
	}
	semanticJSON(t, v, "direct", "true")
	semanticJSON(t, v, "out", "true")
	semanticJSON(t, v, "constraint", uuid)
	for _, source := range []string{
		`bad:uuid.Valid("invalid")`,
		`bad:"invalid" & uuid.Valid`,
		`bad:uuid.Valid(1)`,
		`f:func(s:string)->int:uuid.Valid(s)`,
		`f:func(s:string)->false:uuid.Valid(s)`,
	} {
		v := cuecontext.New().CompileString("import \"uuid\"\n" + source)
		if v.Validate() == nil {
			t.Fatalf("accepted %s", source)
		}
	}
	v = cuecontext.New().CompileString(`import "uuid"
constraint: uuid.Valid`)
	if err := v.FillPath(cue.ParsePath("constraint"), "00000000-0000-0000-0000-000000000000").Validate(); err != nil {
		t.Fatal(err)
	}
	if err := v.FillPath(cue.ParsePath("constraint"), "invalid").Validate(); err == nil {
		t.Fatal("validator constraint was lost")
	}
}
