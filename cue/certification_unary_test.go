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

func TestQuantifiedUnaryCertification(t *testing.T) {
	for _, tt := range []struct {
		domain, result, body, input, want string
	}{
		{"bool", "bool", "!x", "true", "false"},
		{"false", "true", "!x", "false", "true"},
		{"int", "int", "+x", "2", "2"},
		{"int & >=0", "int & <=0", "-x", "0", "0"},
		{"int & >0", "int & <0", "-x", "2", "-2"},
		{"number & !=0", "number & !=0", "-x", "2.5", "-2.5"},
		{"1 | 2", "-1 | -2", "-x", "2", "-2"},
	} {
		t.Run(tt.domain+tt.body, func(t *testing.T) {
			src := fmt.Sprintf("f: func(x: %s) -> (%s): %s", tt.domain, tt.result, tt.body)
			v := cuecontext.New().CompileString(src)
			if err := v.Validate(cue.Concrete(true)); err != nil {
				t.Fatal(err)
			}
			v = cuecontext.New().CompileString(src + "\nout: f(" + tt.input + ")")
			if got, err := v.LookupPath(cue.ParsePath("out")).MarshalJSON(); err != nil || string(got) != tt.want {
				t.Fatalf("got %s, %v; want %s", got, err, tt.want)
			}
		})
	}
	for _, src := range []string{
		"func(x: bool) -> true: !x",
		"func(x: int) -> (int & >=0): -x",
		"func(x: int & >=0) -> (int & <0): -x",
		"func(x: int) -> bool: !x",
	} {
		if err := cuecontext.New().CompileString("f: " + src).Validate(cue.Concrete(true)); err == nil {
			t.Errorf("certified invalid contract: %s", src)
		}
	}
}
