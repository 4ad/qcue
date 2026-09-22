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

package astutil_test

import (
	"fmt"
	"testing"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/ast/astutil"
	"cuelang.org/go/cue/parser"
	"cuelang.org/go/cue/token"
)

func TestQuantifiedScopes(t *testing.T) {
	f, err := parser.ParseFile("test.cue", `
@experiment(quantified)
let A = int
q: forall (A: A, B: A) {
    outer: A
    inner: exists A {a: A, b: B}
}
outside: A
`)
	if err != nil {
		t.Fatal(err)
	}
	astutil.Resolve(f, func(pos token.Pos, msg string, args ...interface{}) {
		t.Errorf("%s: %s", pos, fmt.Sprintf(msg, args...))
	})
	q := f.Decls[2].(*ast.Field).Value.(*ast.Quantifier)
	a, b := q.Params[0], q.Params[1]
	if a.Bound.(*ast.Ident).Node != f.Decls[1] {
		t.Fatal("bound must resolve before its own binder")
	}
	if b.Bound.(*ast.Ident).Node != a {
		t.Fatal("later bound must see earlier binder")
	}
	body := q.Body.(*ast.StructLit)
	if body.Elts[0].(*ast.Field).Value.(*ast.Ident).Node != a {
		t.Fatal("outer binder lost")
	}
	inner := body.Elts[1].(*ast.Field).Value.(*ast.Quantifier)
	innerBody := inner.Body.(*ast.StructLit)
	if innerBody.Elts[0].(*ast.Field).Value.(*ast.Ident).Node != inner.Params[0] {
		t.Fatal("inner binder must shadow")
	}
	if innerBody.Elts[1].(*ast.Field).Value.(*ast.Ident).Node != b {
		t.Fatal("outer binder not visible")
	}
	if f.Decls[3].(*ast.Field).Value.(*ast.Ident).Node != f.Decls[1] {
		t.Fatal("binder escaped scope")
	}
	// Copying a quantified graph must rebind references to the copied binder.
	copied := ast.Clone(q)
	if copied.Params[1].Bound.(*ast.Ident).Node != copied.Params[0] {
		t.Fatal("copied binder not shared")
	}
	if copied.Params[0].Bound.(*ast.Ident).Node != f.Decls[1] {
		t.Fatal("outer reference changed during copy")
	}
	astutil.Apply(copied, nil, nil)
}

func TestQuantifiedDuplicateBinders(t *testing.T) {
	for _, src := range []string{`x: forall (A, A) A`, `x: exists _ int`} {
		f, err := parser.ParseFile("test.cue", "@experiment(quantified)\n"+src)
		if err != nil {
			continue // ParseFile performs resolution as part of parsing.
		}
		var found bool
		astutil.Resolve(f, func(token.Pos, string, ...interface{}) { found = true })
		if !found {
			t.Fatalf("no binding error for %s", src)
		}
	}
}

func TestAliasAndOpenScopes(t *testing.T) {
	f, err := parser.ParseFile("test.cue", `
@experiment(quantified)
let A = int
Box(A, B: A) = {a: A, b: B}
p: seal #Package with (A = A) {value: 1}
x: open p as (A, P) {a: A, p: P}
`)
	if err != nil {
		t.Fatal(err)
	}
	alias := f.Decls[2].(*ast.ParametricAlias)
	if alias.Params[1].Bound.(*ast.Ident).Node != alias.Params[0] {
		t.Fatal("alias bound lost its binder")
	}
	seal := f.Decls[3].(*ast.Field).Value.(*ast.SealExpr)
	if seal.Witnesses[0].Expr.(*ast.Ident).Node != f.Decls[1] {
		t.Fatal("private witness must resolve in declaration environment")
	}
	opened := f.Decls[4].(*ast.Field).Value.(*ast.OpenExpr)
	body := opened.Body.(*ast.StructLit)
	if id := body.Elts[0].(*ast.Field).Value.(*ast.Ident); id.Node != opened.Type || id.Scope != opened {
		t.Fatal("opened type not bound in opening scope")
	}
	if body.Elts[1].(*ast.Field).Value.(*ast.Ident).Node != opened.View {
		t.Fatal("opened view not shared")
	}
	clone := ast.Clone(opened)
	if clone.Body.(*ast.StructLit).Elts[0].(*ast.Field).Value.(*ast.Ident).Node != clone.Type {
		t.Fatal("copy did not preserve opening scope")
	}
}
