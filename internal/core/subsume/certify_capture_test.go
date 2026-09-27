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
	"testing"

	"cuelang.org/go/internal/core/adt"
	"cuelang.org/go/internal/core/eval"
	"cuelang.org/go/internal/core/runtime"
	"cuelang.org/go/internal/core/subsume"
)

func TestConditionalCaptureCertificates(t *testing.T) {
	for _, tt := range []struct {
		name, source    string
		valid, concrete bool
	}{
		{"scalar", `n:int
f:func()->int:n+1`, true, false},
		{"record", `r:{a:int}
f:func()->int:r.a`, true, false},
		{"required", `r:{a!:int}
f:func()->int:r.a`, true, false},
		{"list", `xs:[int]
f:func()->int:xs[0]`, true, false},
		{"import", `g:func(int)->int
f:func()->int:g(1)`, true, false},
		{"polymorphic_import", `g:forall A func(A)->A
f:func()->int:g[int](1)`, true, false},
		{"record_import", `r:{g:func(int)->int}
f:func()->int:r.g(1)`, true, false},
		{"list_import", `xs:[func(int)->int]
f:func()->int:xs[0](1)`, true, false},
		{"linked", `n:2
g:func(x:int)->int:x+1
f:func()->int:g(n)`, true, true},
		{"unknown_field", `r:{}
f:func()->int:r.a`, false, false},
		{"bad_scalar", `n:string
f:func()->int:n+1`, false, false},
		{"refuted_data", `n:int&string
f:func()->int:n`, false, false},
		{"bad_import_packet", `g:func(int)->int
f:func()->int:g("bad")`, false, false},
		{"bad_implementation", `g:func(x:int)->int:"bad"
f:func()->int:g(1)`, false, false},
		{"bad_record_implementation", `r:{g:func(x:int)->int:"bad"}
f:func()->int:r.g(1)`, false, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := eval.NewContext(runtime.New(), nil)
			root := parse(t, ctx, tt.source)
			var field *adt.Vertex
			for _, arc := range root.Arcs {
				if arc.Label.SelectorString(ctx) == "f" {
					field = arc
				}
			}
			if field == nil {
				t.Fatal("missing function field")
			}
			f, ok := adt.Unwrap(field).(*adt.FuncValue)
			if !ok {
				t.Fatalf("missing function: %v", field.Bottom())
			}
			err := subsume.ValidateFunction(ctx, f)
			if (err == nil) != tt.valid {
				t.Fatalf("static certificate: valid=%v: %v", tt.valid, err)
			}
			err = adt.Validate(ctx, field, &adt.ValidateConfig{Concrete: true, Runtime: true, CheckFunction: subsume.ValidateFunction, CheckBuiltin: subsume.ValidateBuiltin})
			if (err == nil) != tt.concrete {
				t.Fatalf("concrete closure: valid=%v: %v", tt.concrete, err)
			}
		})
	}
}
