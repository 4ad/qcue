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

package compile

import (
	"cuelang.org/go/internal/core/adt"
	"cuelang.org/go/internal/core/walk"
)

// Type parameters may occur in checked signatures, type selection, and seal
// witnesses. They cannot supply runtime results, arguments, or defaults. In
// particular, a bound of int does not turn an erased type into an integer
// argument. Finite value binders are different: closure conversion captures
// their selected values as part of the runtime descriptor.
func (c *compiler) checkFunctionErasure(fn *adt.Function) {
	seen := make(map[adt.Node]bool)
	var w walk.Visitor
	w.Before = func(n adt.Node) bool {
		if n == nil || seen[n] {
			return false
		}
		seen[n] = true
		switch x := n.(type) {
		case *adt.TypeReference:
			if x.Param.ValueRange == nil {
				c.errf(x.Src, "erased type parameter %s cannot be used as a runtime value", x.Param.Src.Name.Name)
			}
			return false
		case *adt.Function:
			// Each nested implementation checks its own code. Its annotations
			// are erased even when its descriptor is constructed by this body.
			return false
		case *adt.Quantified:
			w.Elem(x.Body)
			return false
		case *adt.AliasApplication:
			// Alias arguments are predicate substitutions. Inspect their
			// uses in the template, rather than treating the substitutions
			// themselves as runtime call arguments. Nested lambdas have
			// already checked their bodies independently.
			for _, arg := range x.Args {
				if hasErasedParameter(arg) {
					w.Elem(x.Template.Body)
					break
				}
			}
			return false
		case *adt.LetReference:
			w.Elem(x.X)
			return false
		case *adt.IndexExpr:
			if x.Quantified && hasErasedParameter(x.Index) {
				// Indexing is overloaded. Preserve type application and reject
				// fallback to ordinary data indexing when the subject resolves.
				x.ErasedIndex = true
				w.Elem(x.X)
				return false
			}
		case *adt.PackageSeal:
			w.Elem(x.Body)
			return false
		}
		return true
	}
	for _, p := range fn.Params {
		w.Elem(p.Default)
	}
	w.Elem(fn.Body)
}

func hasErasedParameter(expr adt.Expr) bool {
	seen := make(map[adt.Node]bool)
	found := false
	var w walk.Visitor
	w.Before = func(n adt.Node) bool {
		if n == nil || seen[n] || found {
			return false
		}
		seen[n] = true
		switch x := n.(type) {
		case *adt.TypeReference:
			found = x.Param.ValueRange == nil
		case *adt.LetReference:
			w.Elem(x.X)
		}
		return true
	}
	w.Elem(expr)
	return found
}
