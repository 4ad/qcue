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
)

func TestQuantifiedTransportCertificates(t *testing.T) {
	for _, tt := range []struct {
		name, source, want string
		valid              bool
	}{
		{"empty_tail", `#I:exists A {f:func()->[..._|_]}
p:seal #I with (A=int) {f:func()->[]:[]}
out:(open p as (A,P) {r:P.f()}).r`, `[]`, true},
		{"empty_tail_wrong_result", `#I:exists A {f:func()->[..._|_]}
p:seal #I with (A=int) {f:func()->[...int]:[1]}
out:(open p as (A,P) {r:P.f()}).r`, "", false},
		{"round_trip_extra_contract", `#I:exists A {id:func(func(int)->int)->(func(int)->int)}
p:seal #I with (A=int) {id:func(f:func(int)->int)->(func(int)->int):f}
f:(func(x:int,y:int)->int:x+y)(1,...)
out:(open p as (A,P) {r:(P.id(f)&f&(func(int)->0))(2)}).r`, "", false},
		{"invalid_private_universal", `#I:exists S {f:forall A func(A)->int}
p:seal #I with (S=int) {f:forall A func(x:A)->A:1}
out:(open p as (S,P) {r:P.f[1](1)}).r`, "", false},
		{"label_capture", `labels:{[string]~(L,_):func()->string:L}
labels:{word:_}
out:labels.word()`, `"word"`, true},
		{"wrong_label_capture", `labels:{[string]~(L,_):func()->int:L}
labels:{word:_}
out:labels.word()`, "", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := semanticValue(t, tt.source)
			out := root.LookupPath(cue.ParsePath("out"))
			if err := out.Validate(); (err == nil) != tt.valid {
				t.Fatalf("valid=%v: %v", tt.valid, err)
			}
			if tt.valid {
				semanticJSON(t, root, "out", tt.want)
			} else if _, err := out.MarshalJSON(); err == nil {
				t.Fatal("executed an uncertified transport")
			}
		})
	}
}
