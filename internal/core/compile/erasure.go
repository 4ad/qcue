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

type erasureScope struct {
	up   *erasureScope
	args map[*adt.TypeParameter]adt.Expr
}

type erasureVisit struct {
	n adt.Node
	s *erasureScope
}

func (s *erasureScope) bind(x *adt.AliasApplication) *erasureScope {
	next := &erasureScope{up: s, args: make(map[*adt.TypeParameter]adt.Expr)}
	for i, arg := range x.Args {
		next.args[x.Template.Params[i]] = arg
	}
	return next
}

// Type parameters may occur in checked signatures, type selection, and seal
// witnesses. They cannot supply runtime results, arguments, or defaults. In
// particular, a bound of int does not turn an erased type into an integer
// argument. Finite value binders are different: closure conversion captures
// their selected values as part of the runtime descriptor.
func (c *compiler) checkFunctionErasure(fn *adt.Function) {
	// Each alias argument is interpreted in its caller's substitution scope.
	// A template can be visited with several substitutions in the same body.
	var current *erasureScope
	seen := make(map[erasureVisit]bool)
	var w walk.Visitor
	w.Before = func(n adt.Node) bool {
		key := erasureVisit{n, current}
		if n == nil || seen[key] {
			return false
		}
		seen[key] = true
		switch x := n.(type) {
		case *adt.TypeReference:
			for s := current; s != nil; s = s.up {
				if arg, ok := s.args[x.Param]; ok {
					saved := current
					current = s.up
					w.Elem(arg)
					current = saved
					return false
				}
			}
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
			saved := current
			current = current.bind(x)
			w.Elem(x.Template.Body)
			current = saved
			return false
		case *adt.LetReference:
			w.Elem(x.X)
			return false
		case *adt.IndexExpr:
			if x.Quantified && hasErasedParameter(x.Index, current) {
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

func hasErasedParameter(expr adt.Expr, current *erasureScope) bool {
	seen := make(map[erasureVisit]bool)
	found := false
	var w walk.Visitor
	w.Before = func(n adt.Node) bool {
		key := erasureVisit{n, current}
		if n == nil || seen[key] || found {
			return false
		}
		seen[key] = true
		switch x := n.(type) {
		case *adt.TypeReference:
			for s := current; s != nil; s = s.up {
				if arg, ok := s.args[x.Param]; ok {
					saved := current
					current = s.up
					w.Elem(arg)
					current = saved
					return false
				}
			}
			found = x.Param.ValueRange == nil
		case *adt.AliasApplication:
			saved := current
			current = current.bind(x)
			w.Elem(x.Template.Body)
			current = saved
			return false
		case *adt.LetReference:
			w.Elem(x.X)
		}
		return true
	}
	w.Elem(expr)
	return found
}
