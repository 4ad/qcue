// Copyright 2026 The CUE Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Command gentypes updates the standard-library catalogue in doc/types.md.
// Run it from the repository root; -check verifies without writing.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"strings"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/format"
	"cuelang.org/go/cue/parser"
	"cuelang.org/go/cue/token"
	"cuelang.org/go/pkg"
)

const (
	startMarker = "<!-- BEGIN GENERATED PACKAGE TYPES -->"
	endMarker   = "<!-- END GENERATED PACKAGE TYPES -->"
)

func main() {
	check := flag.Bool("check", false, "check that the catalogue is current without writing")
	flag.Parse()
	if err := run("doc/types.md", *check); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(filename string, check bool) error {
	src, err := os.ReadFile(filename)
	if err != nil {
		return err
	}
	want, err := update(src)
	if err != nil {
		return err
	}
	if bytes.Equal(src, want) {
		return nil
	}
	if check {
		return fmt.Errorf("%s is stale; run go run ./internal/cmd/gentypes from the repository root", filename)
	}
	return os.WriteFile(filename, want, 0o644)
}

func update(src []byte) ([]byte, error) {
	if bytes.Count(src, []byte(startMarker)) != 1 || bytes.Count(src, []byte(endMarker)) != 1 {
		return nil, fmt.Errorf("expected exactly one pair of package catalogue markers")
	}
	start := bytes.Index(src, []byte(startMarker)) + len(startMarker)
	end := bytes.Index(src, []byte(endMarker))
	if start > end {
		return nil, fmt.Errorf("package catalogue markers are out of order")
	}
	var out bytes.Buffer
	out.Write(src[:start])
	out.WriteString("\n\n")
	paths := pkg.ImportPaths()
	fmt.Fprintf(&out, "This catalogue covers all %d package interfaces.\n\n", len(paths))
	for _, ip := range paths {
		fmt.Fprintf(&out, "- [`%s`](#package-%s)\n", ip, strings.ReplaceAll(ip, "/", "-"))
	}
	for _, ip := range paths {
		source, ok := pkg.Source(ip)
		if !ok {
			return nil, fmt.Errorf("missing package source for %q", ip)
		}
		file, err := parser.ParseFile(ip+"/pkg.cue", source)
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(&out, "\n<a id=\"package-%s\"></a>\n\n### `%s`\n\n", strings.ReplaceAll(ip, "/", "-"), ip)
		fmt.Fprintf(&out, "[Source](../pkg/%s/pkg.cue)\n\n", ip)
		// Put new function contracts first. Keep supporting declarations
		// available without burying the signatures beneath constants and
		// task schemas. Package attributes stay in the linked source.
		var signatures, support []ast.Decl
		for _, decl := range file.Decls {
			switch x := decl.(type) {
			case *ast.Package, *ast.Attribute:
				continue
			case *ast.Field:
				if hasFunctionType(x.Value) {
					signatures = append(signatures, decl)
					continue
				}
			}
			support = append(support, decl)
		}
		if err := writeDecls(&out, signatures); err != nil {
			return nil, err
		}
		if len(support) != 0 {
			if len(signatures) == 0 {
				out.WriteString("No function declarations.\n\n")
			}
			out.WriteString("\n<details>\n<summary>Supporting schemas and constants</summary>\n\n")
			if err := writeDecls(&out, support); err != nil {
				return nil, err
			}
			out.WriteString("\n</details>\n")
		}
	}
	out.WriteByte('\n')
	out.Write(src[end:])
	return out.Bytes(), nil
}

func hasFunctionType(expr ast.Expr) bool {
	switch x := expr.(type) {
	case *ast.Func:
		return true
	case *ast.ParenExpr:
		return hasFunctionType(x.X)
	case *ast.Quantifier:
		return hasFunctionType(x.Body)
	case *ast.BinaryExpr:
		return hasFunctionType(x.X) || hasFunctionType(x.Y)
	}
	return false
}

func writeDecls(out *bytes.Buffer, decls []ast.Decl) error {
	if len(decls) == 0 {
		return nil
	}
	file := &ast.File{Decls: decls}
	for _, decl := range decls {
		ast.SetRelPos(decl, token.Newline)
	}
	wrapTypes(file)
	formatted, err := format.Node(file, format.Simplify())
	if err != nil {
		return err
	}
	out.WriteString("```cue\n")
	out.Write(formatted)
	out.WriteString("```\n")
	return nil
}

// Insert layout hints at syntax boundaries, leaving the declarations intact.
// In particular, fixed-size address tuples and unions of call forms should
// not require hundreds of columns to read in a Markdown code block.
func wrapTypes(file *ast.File) {
	ast.Walk(file, nil, func(n ast.Node) {
		switch n.(type) {
		case *ast.BinaryExpr, *ast.Func, *ast.ListLit, *ast.StructLit:
		default:
			return
		}
		b, err := format.Node(n)
		if err != nil || bytes.Contains(bytes.TrimSpace(b), []byte{'\n'}) || len(bytes.TrimSpace(b)) <= 80 {
			return
		}
		switch x := n.(type) {
		case *ast.BinaryExpr:
			ast.SetRelPos(x.Y, token.Newline)
		case *ast.Func:
			for _, p := range x.Params {
				ast.SetRelPos(p, token.Newline)
			}
			x.Rparen = x.Rparen.WithRel(token.Newline)
		case *ast.ListLit:
			for _, elem := range x.Elts {
				ast.SetRelPos(elem, token.Newline)
			}
			x.Rbrack = x.Rbrack.WithRel(token.Newline)
		case *ast.StructLit:
			for _, elem := range x.Elts {
				ast.SetRelPos(elem, token.Newline)
			}
			x.Rbrace = x.Rbrace.WithRel(token.Newline)
		}
	})
}
