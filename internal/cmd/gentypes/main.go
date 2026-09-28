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
		// Preserve every declaration, import, attribute, and default. The
		// linked source carries the detailed API comments; omit them here
		// to keep the catalogue focused on types.
		file, err := parser.ParseFile(ip+"/pkg.cue", source)
		if err != nil {
			return nil, err
		}
		wrapTypes(file)
		formatted, err := format.Node(file, format.Simplify())
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(&out, "\n<a id=\"package-%s\"></a>\n\n### `%s`\n\n", strings.ReplaceAll(ip, "/", "-"), ip)
		fmt.Fprintf(&out, "[API declarations and comments](../pkg/%s/pkg.cue)\n\n", ip)
		out.WriteString("```cue\n")
		out.Write(formatted)
		out.WriteString("```\n")
	}
	out.WriteByte('\n')
	out.Write(src[end:])
	return out.Bytes(), nil
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
