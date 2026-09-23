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
	"strings"
	"testing"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
)

// Certification must check the same packet protocol as runtime invocation.
func TestQuantifiedCertificationProtocol(t *testing.T) {
	for _, tt := range []struct {
		name, source, call string
		valid              bool
	}{
		{"partial", `p: (func(x: int, y: int) -> int: y)(1, ...)
f: func(x: int) -> int: p(x)`, `f(2)`, true},
		{"partial_arity", `p: (func(x: int, y: int) -> int: y)(1, ...)
f: func(x: int, y: int) -> int: p(x, y)`, `f(2, 3)`, false},
		{"partial_label", `p: (func(x: int, y: int) -> int: y)(x: 1, ...)
f: func(y: int) -> int: p(y: y)`, `f(2)`, true},
		{"partial_bound_label", `p: (func(x: int, y: int) -> int: y)(x: 1, ...)
f: func(x: int, y: int) -> int: p(x: x, y: y)`, `f(2, 3)`, false},
		{"len", `f: func(x: string) -> int: len(x)`, `f("abc")`, true},
		{"len_label", `f: func(x: string) -> int: len(wrong: x)`, `f("abc")`, false},
		{"len_arity", `f: func(x: string) -> int: len(x, x)`, `f("abc")`, false},
		{"primitive", "import \"strings\"\nh: strings.ToUpper\nf: func(x: string) -> string: h(x)", `f("abc")`, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			v := cuecontext.New().CompileString(tt.source + "\nout: " + tt.call)
			f := v.LookupPath(cue.ParsePath("f"))
			if err := f.Validate(cue.Concrete(true)); (err == nil) != tt.valid {
				t.Fatalf("certification: %v; want valid %v", err, tt.valid)
			}
			out := v.LookupPath(cue.ParsePath("out"))
			if err := out.Validate(cue.Concrete(true)); (err == nil) != tt.valid {
				t.Fatalf("execution: %v; want valid %v", err, tt.valid)
			}
		})
	}
}

// Argument obligations survive even when the body ignores its callback or
// successfully calls it at one point in the promised domain.
func TestQuantifiedCallbackPacket(t *testing.T) {
	for _, body := range []string{"5", "cb(1)", "{value: cb(1)}.value"} {
		for _, nested := range []bool{false, true} {
			for _, good := range []bool{false, true} {
				t.Run(body, func(t *testing.T) {
					body := body
					callbackBody := "{value: n}.value"
					if good {
						callbackBody = "1"
					}
					signature, argument := "func(int) -> 1", "func(n: int) -> int: "+callbackBody
					if nested {
						signature = "{f: " + signature + "}"
						argument = "{f: " + argument + "}"
						body = strings.ReplaceAll(body, "cb(", "cb.f(")
					}
					v := cuecontext.New().CompileString("out: (func(cb: " + signature + ") -> int: " + body + ")(" + argument + ")")
					out := v.LookupPath(cue.ParsePath("out"))
					if err := out.Validate(cue.Concrete(true)); (err == nil) != good {
						t.Fatalf("good=%v nested=%v body=%s: %v", good, nested, body, err)
					}
					if _, err := out.MarshalJSON(); (err == nil) != good {
						t.Fatalf("serialization discarded callback proof: %v", err)
					}
					if !good && out.Validate() != nil {
						t.Fatal("unproved conformance became a contradiction")
					}
				})
			}
		}
	}
}

func TestQuantifiedCertificationBounds(t *testing.T) {
	for _, tt := range []struct {
		source string
		valid  bool
	}{
		{`func(x: int & >=0) -> (int & >=1): x + 1`, true},
		{`func(x: int & >=0) -> (int & >=2): x + 1`, false},
		{`func(x: int & <0) -> (int & <1): 1 + x`, true},
		{`func(x: int & <=0) -> (int & <= -1): x - 1`, true},
		{`func(x: int & <=0) -> (int & < -1): x - 1`, false},
		{`func(x: int & >=0) -> (int & >=0): x - 1`, false},
	} {
		v := cuecontext.New().CompileString("f: " + tt.source)
		if err := v.LookupPath(cue.ParsePath("f")).Validate(cue.Concrete(true)); (err == nil) != tt.valid {
			t.Errorf("%s: %v; want valid %v", tt.source, err, tt.valid)
		}
	}
}

func TestQuantifiedCertificationPackageOpening(t *testing.T) {
	for _, escape := range []bool{false, true} {
		body := "P.show(P.value)"
		result := "string"
		if escape {
			body, result = "P.value", "_"
		}
		v := cuecontext.New().CompileString(`
#Showable: exists A {value: A, show: func(A) -> string}
f: func(p: #Showable) -> ` + result + `: (open p as (A, P) {out: ` + body + `}).out
`)
		if err := v.LookupPath(cue.ParsePath("f")).Validate(cue.Concrete(true)); (err == nil) == escape {
			t.Fatalf("escape=%v: %v", escape, err)
		}
	}
}
