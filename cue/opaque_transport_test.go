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
	"cuelang.org/go/cue/format"
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
			if err != nil {
				t.Fatal(err)
			}
			rebuilt := cuecontext.New().CompileString("p: " + string(src) + `
tag: (open p as (B, P) {result: P.tag}).result
`)
			if got, err := rebuilt.LookupPath(cue.ParsePath("tag")).Int64(); err != nil || got != 1 {
				t.Fatalf("%s lost its package boundary: %s\n%v", path, src, err)
			}
		}
	}
}

func TestQuantifiedDependentPackageTransport(t *testing.T) {
	for _, body := range []string{"{#T: T}", "{value?: T}", "{[string]: T}"} {
		t.Run(body, func(t *testing.T) {
			v := cuecontext.New().CompileString(`
Inner(T) = exists B ` + body + `
#Outer: exists A {inner: Inner(A)}
private: seal Inner(int) with (B = bool) {}
p: seal #Outer with (A = int) {inner: private}
q: (open p as (A, P) {result: P.inner}).result
`)
			if err := v.LookupPath(cue.ParsePath("p")).Validate(cue.Concrete(true)); err == nil ||
				!strings.Contains(err.Error(), "dependent package transport remains unresolved") {
				t.Fatalf("dependent public interface must remain incomplete: %v", err)
			}
		})
	}
	v := cuecontext.New().CompileString(`
Inner(T) = exists B {#T: T}
#Outer: exists A {inner: Inner(A)}
private: seal Inner(int) with (B = bool) {}
p: seal #Outer with (A = int) {inner: private}
q: (open p as (A, P) {result: P.inner}).result
exposed: (open q as (B, Q) {result: Q.#T}).result
out: exposed & 1
`)
	if got, err := v.LookupPath(cue.ParsePath("out")).MarshalJSON(); err == nil {
		t.Fatalf("nested package exposed the outer representation: %s", got)
	}
}

func TestQuantifiedPackageFreeAbstractDependencies(t *testing.T) {
	for _, body := range []string{
		`forall B (func(A) -> B | func(B) -> A)`,
		`seal (exists B {#T: A}) with (B = bool) {}`,
		`seal ((exists B {}) & {#T: A}) with (B = bool) {}`,
		`seal ((exists B {}) & {[string]: A}) with (B = bool) {}`,
	} {
		t.Run(body, func(t *testing.T) {
			v := cuecontext.New().CompileString(`
#M: exists A {value: A}
p: seal #M with (A = int) {value: 1}
out: (open p as (A, P) {result: ` + body + `}).result
`)
			if err := v.LookupPath(cue.ParsePath("out")).Validate(); err == nil ||
				!strings.Contains(err.Error(), "abstract type escapes its opening scope") {
				t.Fatalf("free abstract dependency escaped: %v", err)
			}
		})
	}

	// An inner seal may bind the outer representation as its private
	// witness when its public interface is independent of that witness.
	v := cuecontext.New().CompileString(`
#M: exists A {value: A}
p: seal #M with (A = int) {value: 1}
out: (open p as (A, P) {
	result: seal (exists B {value: B}) with (B = A) {value: P.value}
}).result
`)
	if err := v.LookupPath(cue.ParsePath("out")).Validate(cue.Concrete(true)); err != nil {
		t.Fatal(err)
	}
}
