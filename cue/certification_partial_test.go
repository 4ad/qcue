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

// A callback can implement its own declared type without belonging to the
// stronger domain required by the closure that saves it. Check certification
// before executing a call, so execution cannot supply the missing evidence.
func TestQuantifiedSavedPacketMembership(t *testing.T) {
	for _, shape := range []struct{ domain, value string }{
		{"func(int) -> 1", "cb"},
		{"{f: func(int) -> 1}", "{f: cb}"},
		{"[func(int) -> 1]", "[cb]"},
	} {
		for _, binding := range []string{"%s, ...", "h: %s, ..."} {
			for _, body := range []string{"{v: x}.v", "1"} {
				for _, contract := range []string{"", " & (func(y: int) -> int)"} {
					t.Run(shape.domain+binding+body+contract, func(t *testing.T) {
						good := body == "1"
						src := fmt.Sprintf(`
f: func(h: %s, y: int) -> int: 0
cb: func(x: int) -> int: %s
p: f(%s)%s
g: func(y: int) -> int: p(y)
`, shape.domain, body, fmt.Sprintf(binding, shape.value), contract)
						v := cuecontext.New().CompileString(src)
						for _, path := range []string{"cb", "p", "g", ""} {
							x := v.LookupPath(cue.ParsePath(path))
							if err := x.Validate(); err != nil {
								t.Fatalf("%s: incomplete membership became conflict: %v", path, err)
							}
							if err := x.Validate(cue.Concrete(true)); (err == nil) != (good || path == "cb") {
								t.Fatalf("%s: conformance %v, good=%v", path, err, good)
							}
						}
						called := cuecontext.New().CompileString(src + "\nout: g(2)")
						data, err := called.LookupPath(cue.ParsePath("out")).MarshalJSON()
						if (err == nil) != good || good && string(data) != "0" {
							t.Fatalf("call: %s, %v", data, err)
						}
					})
				}
			}
		}
	}
}
