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

package adt

import (
	"slices"

	"cuelang.org/go/cue/ast"
)

// Existential retains a scoped predicate until a witness is supplied. It is
// a validator rather than an approximation of its bound: forgetting the
// predicate would discard the sharing between its representation fields.
type Existential struct {
	Template *Quantified
	Env      *Environment
}

func (e *Existential) Source() ast.Node         { return e.Template.Source() }
func (*Existential) node()                      {}
func (*Existential) expr()                      {}
func (*Existential) declNode()                  {}
func (*Existential) elemNode()                  {}
func (*Existential) Concreteness() Concreteness { return Constraint }
func (e *Existential) Kind() Kind {
	switch e.Template.Body.(type) {
	case *StructLit:
		return StructKind
	case *ListLit:
		return ListKind
	case *Function:
		return FuncKind
	}
	return TopKind
}

func (e *Existential) validate(c *OpContext, value Value) *Bottom {
	if v, ok := value.(*Vertex); ok {
		v = v.DerefValue()
		if p := v.sealed; p != nil && p.interfaceType.Template == e.Template {
			if sameTypeEnvironment(c, p.interfaceType.Env, e.Env) {
				return nil
			}
			// Reusing a template does not identify its captured predicates.
			// Check this instance using the package's existing shared public
			// witness, rather than forgetting the new environment or choosing
			// an unrelated representation for each field.
			args := make(map[*TypeParameter]Value, len(p.carriers))
			for param, carrier := range p.carriers {
				args[param] = &OpaqueType{carrier: carrier}
			}
			return e.validateWitness(c, quantifiedEnvironment(c, e, args), value)
		}
	}
	if covariantData(e.Template.Body) {
		// For a covariant data predicate and a covariant upper-bounded
		// telescope, the existential join is attained at the largest
		// admissible types. Keep the template for explicit sealing: its
		// logical simplification does not erase an abstraction boundary.
		for _, param := range e.Template.Params {
			if !covariantData(param.Bound) {
				return e.unresolved(c)
			}
		}
		env := quantifiedEnvironment(c, e, nil)
		for _, param := range e.Template.Params {
			var bound Value = &Top{}
			if param.Bound != nil {
				var complete bool
				bound, complete = c.Evaluate(env, param.Bound)
				if !complete {
					return e.unresolved(c)
				}
			}
			if param.ExplicitLevel {
				level, known := universeOf(c, bound, make(map[Expr]bool))
				if !known || level > param.Level {
					return e.unresolved(c)
				}
			}
			env = instantiateEnvironment(env, map[*TypeParameter]Value{param: bound})
		}
		return e.validateWitness(c, env, value)
	}
	return e.unresolved(c)
}

// Type frames contain immutable predicate arguments and no runtime fields.
// Outside them only shared value cells establish environment identity: equal
// upper approximations of distinct witnesses are not equal assignments.
func sameTypeEnvironment(c *OpContext, a, b *Environment) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil || !sameTypeEnvironment(c, a.Up, b.Up) {
		return false
	}
	if a.types == nil || b.types == nil {
		return a.types == b.types && a.DerefVertex(c) == b.DerefVertex(c)
	}
	if a.types.quantifier != b.types.quantifier || len(a.types.arguments) != len(b.types.arguments) {
		return false
	}
	for param, value := range a.types.arguments {
		other, ok := b.types.arguments[param]
		// Data equality omits patterns, optional fields and preferences.
		// Only shared predicates or this exact scalar vocabulary justify
		// the shortcut; structural arguments go through membership below.
		if !ok || value != other && (!fixedCapabilityExpr(value) || !fixedCapabilityExpr(other) ||
			!Equal(c, value, other, CheckStructural)) {
			return false
		}
	}
	return true
}

func (e *Existential) validateWitness(c *OpContext, env *Environment, value Value) *Bottom {
	subject := value
	if vertex, ok := value.(*Vertex); ok {
		// Membership concerns the data witness, not another evaluation of
		// this validator. Other obligations remain on the original vertex.
		subject = vertex.ToDataAll(c)
	}
	v := c.newInlineVertex(nil, nil, MakeRootConjunct(env, e.Template.Body), MakeRootConjunct(nil, subject))
	v.Finalize(c)
	if b := v.Bottom(); b != nil {
		// This child belongs to the private membership check. Report it at
		// the validator boundary so recursive validation cannot skip it.
		copy := *b
		copy.ChildError, copy.HasRecursive = false, false
		return &copy
	}
	cfg := &ValidateConfig{Concrete: true}
	if !covariantData(e.Template.Body) {
		// Conjoining an arrow is not evidence that the existing operation
		// satisfies it. Keep the membership obligation pending until its
		// conformance can be established independently.
		cfg.CheckFunction = func(*OpContext, *FuncValue) *Bottom { return e.unresolved(c) }
	}
	return Validate(c, v, cfg)
}

func (e *Existential) unresolved(c *OpContext) *Bottom {
	return &Bottom{Src: e.Source(), Code: IncompleteError,
		Err: c.Newf("existential witness remains unresolved")}
}

type sealKey struct {
	expr *PackageSeal
	env  *Environment
}

type sealedPackage struct {
	interfaceType *Existential
	privateEnv    *Environment
	publicEnv     *Environment
	carriers      map[*TypeParameter]*opaqueCarrier
	operations    []opaqueAdapter
}

type opaqueCarrier struct {
	owner          *sealedPackage
	parameter      *TypeParameter
	representation Value
}

// OpaqueType denotes the complete abstract carrier, not its private bound.
type OpaqueType struct{ carrier *opaqueCarrier }

func (x *OpaqueType) Source() ast.Node         { return x.carrier.parameter.Src }
func (*OpaqueType) node()                      {}
func (*OpaqueType) expr()                      {}
func (*OpaqueType) declNode()                  {}
func (*OpaqueType) elemNode()                  {}
func (*OpaqueType) Kind() Kind                 { return OpaqueKind }
func (*OpaqueType) Concreteness() Concreteness { return Constraint }
func (x *OpaqueType) validate(c *OpContext, v Value) *Bottom {
	if y, ok := Unwrap(v).(*OpaqueValue); ok && y.carrier == x.carrier {
		return nil
	}
	if !IsConcrete(v) {
		return &Bottom{Code: IncompleteError, Err: c.Newf("incomplete abstract value")}
	}
	return c.NewErrf("value does not belong to this abstract type")
}

// OpaqueValue exposes only carrier identity and equality. Its representation
// is accessible exclusively to the adapters created at the sealing boundary.
type OpaqueValue struct {
	carrier *opaqueCarrier
	private Value
}

func (*OpaqueValue) Source() ast.Node           { return nil }
func (*OpaqueValue) node()                      {}
func (*OpaqueValue) expr()                      {}
func (*OpaqueValue) declNode()                  {}
func (*OpaqueValue) elemNode()                  {}
func (*OpaqueValue) Kind() Kind                 { return OpaqueKind }
func (*OpaqueValue) Concreteness() Concreteness { return Concrete }

type PackageSeal struct {
	Src       *ast.SealExpr
	Interface Expr
	Names     []string
	Witnesses []Expr
	Body      Expr
}

func (s *PackageSeal) Source() ast.Node { return s.Src }
func (*PackageSeal) node()              {}
func (*PackageSeal) expr()              {}
func (*PackageSeal) declNode()          {}
func (*PackageSeal) elemNode()          {}

func (s *PackageSeal) evaluate(c *OpContext, state Flags) Value {
	key := sealKey{s, c.Env(0)}
	if view := c.sealedViews[key]; view != nil {
		return view
	}
	value, _ := c.Evaluate(c.Env(0), s.Interface)
	e := existentialOf(value)
	if e == nil {
		return c.NewErrf("seal requires an existential interface")
	}
	q := e.Template
	if len(s.Names) != len(q.Params) {
		return c.NewErrf("seal requires one witness for each representation type")
	}
	p := &sealedPackage{interfaceType: e,
		carriers: make(map[*TypeParameter]*opaqueCarrier)}
	private := make(map[*TypeParameter]Value)
	public := make(map[*TypeParameter]Value)
	for _, param := range q.Params {
		if param.Bound != nil {
			return c.NewErrf("opaque sealing requires an unbounded representation type")
		}
		i := slices.Index(s.Names, param.Src.Name.Name)
		if i < 0 {
			return c.NewErrf("missing representation witness %s", param.Src.Name.Name)
		}
		v, _ := c.Evaluate(c.Env(0), s.Witnesses[i])
		if v == nil {
			return nil
		}
		if b := checkTypeUniverse(c, param, v); b != nil {
			return b
		}
		carrier := &opaqueCarrier{owner: p, parameter: param, representation: v}
		p.carriers[param] = carrier
		private[param], public[param] = v, &OpaqueType{carrier: carrier}
	}
	p.privateEnv = quantifiedEnvironment(c, e, private)
	p.publicEnv = quantifiedEnvironment(c, e, public)
	implementation := c.newInlineVertex(nil, nil,
		MakeRootConjunct(c.Env(0), s.Body), MakeRootConjunct(p.privateEnv, q.Body))
	implementation.Finalize(c)
	if b := Validate(c, implementation, &ValidateConfig{Concrete: true}); b != nil {
		return b
	}
	view := p.transport(c, p.publicEnv, q.Body, implementation, true)
	if v, ok := view.(*Vertex); ok {
		v.sealed = p
		if c.sealedViews == nil {
			c.sealedViews = make(map[sealKey]*Vertex)
		}
		c.sealedViews[key] = v
	}
	return view
}

func existentialOf(value Value) *Existential {
	switch v := Unwrap(value).(type) {
	case *Existential:
		return v
	case *Conjunction:
		for _, term := range v.Values {
			if e := existentialOf(term); e != nil {
				return e
			}
		}
	}
	return nil
}

func quantifiedEnvironment(c *OpContext, e *Existential, args map[*TypeParameter]Value) *Environment {
	return &Environment{Up: e.Env, Vertex: c.newInlineVertex(nil, &StructMarker{}),
		types: &typeScope{quantifier: e.Template, arguments: args}}
}

// transport follows the interface shape. Private record fields are never
// copied into the public view simply because the representation has them.
func (p *sealedPackage) transport(c *OpContext, env *Environment, schema Expr, value Value, outward bool) Value {
	if b, ok := Unwrap(value).(*Bottom); ok {
		return b
	}
	if r, ok := schema.(*TypeReference); ok {
		carrier := p.carriers[r.Param]
		if carrier == nil {
			v, _ := c.Evaluate(env, r)
			carrier = opaqueTypeOf(v)
		}
		if carrier != nil && carrier.owner == p {
			if !outward {
				if v, ok := Unwrap(value).(*OpaqueValue); ok && v.carrier == carrier {
					return v.private
				}
				return c.NewErrf("argument belongs to a different abstract type")
			}
			if !concreteCapture(c, value) {
				return &Bottom{Code: IncompleteError, Err: c.Newf("incomplete private representation")}
			}
			return &OpaqueValue{carrier: carrier, private: value}
		}
	}
	switch x := schema.(type) {
	case *StructLit:
		for _, d := range x.Decls {
			if _, ok := d.(*Field); !ok {
				template, _ := c.Evaluate(env, schema)
				return p.transportResolved(c, template, value, outward)
			}
		}
		v, ok := value.(*Vertex)
		if !ok {
			return c.NewErrf("interface requires a record")
		}
		v.Finalize(c)
		template, _ := c.Evaluate(env, schema)
		scope, ok := template.(*Vertex)
		if !ok {
			return template
		}
		fieldEnv := &Environment{Up: env, Vertex: scope}
		out := &StructLit{}
		for _, decl := range x.Decls {
			field, ok := decl.(*Field)
			if !ok {
				continue
			}
			var arc *Vertex
			for _, a := range v.Arcs {
				if a.Label == field.Label && a.ArcType == ArcMember {
					arc = a
					break
				}
			}
			if arc == nil {
				if field.ArcType == ArcOptional {
					continue
				}
				return c.NewErrf("missing interface field %s", field.Label.SelectorString(c))
			}
			result := p.transport(c, fieldEnv, field.Value, arc, outward)
			if b, ok := result.(*Bottom); ok {
				return b
			}
			out.Decls = append(out.Decls, &Field{Label: field.Label, Value: result})
		}
		result := c.newInlineVertex(nil, nil, MakeRootConjunct(env, out))
		result.Finalize(c)
		return result
	case *Function:
		f, ok := Unwrap(value).(*FuncValue)
		if !ok {
			return c.NewErrf("interface requires a function implementation")
		}
		for _, a := range p.operations {
			if a.schema == x && a.env == env && a.outward == outward &&
				equalFuncTypes(a.target.Types, f.Types) && closureIdentity(c, a.target, f) == proofEstablished {
				return a.value
			}
		}
		fn := *x
		fn.Params = slices.Clone(x.Params)
		for i := range fn.Params {
			if fn.Params[i].Default != nil {
				// The public contract admits omission; it does not supply
				// the private closure's default. Leave the slot absent so
				// the adapter forwards omission through the boundary.
				fn.Params[i].Default = nil
				fn.Params[i].ArcType = ArcOptional
			}
		}
		fn.Captures = nil
		fn.Body = &OpaqueCall{owner: p, private: f, signature: x, env: env, outward: outward}
		fnEnv := env
		if !outward {
			args := make(map[*TypeParameter]Value, len(p.carriers))
			for param, carrier := range p.carriers {
				args[param] = carrier.representation
			}
			fnEnv = instantiateEnvironment(env, args)
		}
		adapter := &FuncValue{Src: x.Src, Fn: &fn, Env: fnEnv}
		p.operations = append(p.operations, opaqueAdapter{x, env, f, outward, adapter})
		return adapter
	case *ListLit:
		v, ok := value.(*Vertex)
		if !ok || !v.IsList() {
			return c.NewErrf("interface requires a list")
		}
		v.Finalize(c)
		out := &ListLit{}
		i := 0
		for a := range v.Elems() {
			if len(x.Elems) == 0 {
				return c.NewErrf("unexpected interface list element")
			}
			schema, ok := x.Elems[min(i, len(x.Elems)-1)].(Expr)
			if rest, restOK := x.Elems[min(i, len(x.Elems)-1)].(*Ellipsis); restOK {
				schema = rest.Value
			} else if !ok {
				return c.NewErrf("unsupported interface list element")
			}
			out.Elems = append(out.Elems, p.transport(c, env, schema, a, outward))
			i++
		}
		result := c.newInlineVertex(nil, nil, MakeRootConjunct(env, out))
		result.Finalize(c)
		return result
	}
	if schema != nil {
		typ, _ := c.Evaluate(env, schema)
		return p.transportResolved(c, typ, value, outward)
	}
	return value
}

func opaqueTypeOf(value Value) *opaqueCarrier {
	switch v := Unwrap(value).(type) {
	case *OpaqueType:
		return v.carrier
	case *OpaqueValue:
		// A generic argument can be inferred as this value's singleton.
		// Transport still crosses the same abstract carrier boundary.
		return v.carrier
	case *Conjunction:
		for _, term := range v.Values {
			if carrier := opaqueTypeOf(term); carrier != nil {
				return carrier
			}
		}
	}
	return nil
}

func (p *sealedPackage) transportResolved(c *OpContext, schema, value Value, outward bool) Value {
	if b, ok := Unwrap(value).(*Bottom); ok {
		return b
	}
	if union, ok := Unwrap(schema).(*Disjunction); ok {
		return p.transportUnion(c, union, value, outward)
	}
	if carrier := opaqueTypeOf(schema); carrier != nil && carrier.owner == p {
		if !outward {
			if v, ok := Unwrap(value).(*OpaqueValue); ok && v.carrier == carrier {
				return v.private
			}
			return c.NewErrf("argument belongs to a different abstract type")
		}
		if !concreteCapture(c, value) {
			return &Bottom{Code: IncompleteError, Err: c.Newf("incomplete private representation")}
		}
		return &OpaqueValue{carrier: carrier, private: value}
	}
	if f, ok := Unwrap(schema).(*FuncValue); ok {
		return p.transport(c, f.Env, f.Fn, value, outward)
	}
	typ, ok := schema.(*Vertex)
	if !ok || typ.Kind()&(StructKind|ListKind) == 0 {
		return value
	}
	v, ok := value.(*Vertex)
	if !ok {
		return c.NewErrf("interface requires a composite value")
	}
	typ.Finalize(c)
	v.Finalize(c)
	typ, v = typ.DerefValue(), v.DerefValue()
	if typ.IsList() {
		out := &ListLit{}
		for a := range v.Elems() {
			element := typ.LookupRaw(a.Label)
			if element == nil {
				element = c.newInlineVertex(nil, nil)
				element.Label = a.Label
				typ.MatchAndInsert(c, element)
				element.Finalize(c)
			}
			out.Elems = append(out.Elems, p.transportResolved(c, element, a, outward))
		}
		result := c.newInlineVertex(nil, nil, MakeRootConjunct(nil, out))
		result.Finalize(c)
		return result
	}
	out := &StructLit{}
	for _, field := range typ.Arcs {
		if field.Label.IsLet() {
			continue
		}
		a := v.LookupRaw(field.Label)
		if a == nil || a.ArcType != ArcMember {
			if field.ArcType == ArcOptional {
				continue
			}
			return c.NewErrf("missing interface field %s", field.Label.SelectorString(c))
		}
		out.Decls = append(out.Decls, &Field{Label: field.Label,
			Value: p.transportResolved(c, field, a, outward)})
	}
	for _, a := range v.Arcs {
		if a.ArcType != ArcMember || !a.Label.IsRegular() || typ.LookupRaw(a.Label) != nil {
			continue
		}
		field := c.newInlineVertex(nil, nil)
		field.Label = a.Label
		typ.MatchAndInsert(c, field)
		if len(field.Conjuncts) == 0 {
			continue
		}
		field.Finalize(c)
		out.Decls = append(out.Decls, &Field{Label: a.Label,
			Value: p.transportResolved(c, field, a, outward)})
	}
	result := c.newInlineVertex(nil, nil, MakeRootConjunct(nil, out))
	result.Finalize(c)
	return result
}

// OpaqueCall is an authorized adapter. Traversals intentionally cannot visit
// its private implementation or capture environment.
type opaqueAdapter struct {
	schema  *Function
	env     *Environment
	target  *FuncValue
	outward bool
	value   *FuncValue
}

type OpaqueCall struct {
	owner     *sealedPackage
	private   *FuncValue
	signature *Function
	env       *Environment
	outward   bool
}

func (*OpaqueCall) Source() ast.Node { return nil }
func (*OpaqueCall) node()            {}
func (*OpaqueCall) expr()            {}
func (*OpaqueCall) declNode()        {}
func (*OpaqueCall) elemNode()        {}

func (s *OpaqueCall) evaluate(c *OpContext, state Flags) Value {
	bindings := make(map[*TypeParameter]Value)
	for e := c.Env(0); e != nil; e = e.Up {
		if e.types != nil {
			for p, v := range e.types.arguments {
				bindings[p] = v
			}
		}
	}
	// The adapter's code is shared across all erased instances, but its
	// input and output transport must use this call's selected predicates.
	env := instantiateEnvironment(s.env, bindings)
	private := s.private
	publicParams, privateParams := typeParameters(s.env), typeParameters(private.Env)
	if len(publicParams) == len(privateParams) && len(publicParams) != 0 {
		args := make(map[*TypeParameter]Value)
		for i, p := range publicParams {
			v := bindings[p]
			if s.outward {
				switch x := Unwrap(v).(type) {
				case *OpaqueType:
					if x.carrier.owner == s.owner {
						v = x.carrier.representation
					}
				case *OpaqueValue:
					if x.carrier.owner == s.owner {
						v = x.private
					}
				default:
					if abstractEscapes(c, v, s.owner, make(map[Value]bool)) {
						if concreteCapture(c, v) {
							v = s.owner.transportResolved(c, v, v, false)
						} else if len(private.Fn.Params) != 0 {
							// Infer a private instance from the transported
							// inputs when a composite abstract predicate
							// has no direct private representation.
							v = nil
						} else {
							return &Bottom{Code: IncompleteError, Err: c.Newf("abstract type argument transport remains unresolved")}
						}
					}
				}
			}
			args[privateParams[i]] = v
		}
		var b *Bottom
		private, b = private.instantiate(c, args)
		if b != nil {
			return b
		}
	}
	protocol, _ := private.residualSignature()
	matches := matchFuncParams(s.signature, protocol, false)
	arguments := make(map[int]Value)
	for i, p := range s.signature.Params {
		label := p.Local
		if label == InvalidLabel {
			label = anonParamLabel(c, i)
		}
		var value Value
		for _, a := range c.Env(0).Vertex.Arcs {
			if a.Label == label && a.ArcType == ArcMember {
				a.Finalize(c)
				value = a
				break
			}
		}
		if value == nil {
			continue
		}
		v := s.owner.transport(c, env, p.Value, value, !s.outward)
		if b, ok := v.(*Bottom); ok {
			return b
		}
		if matches[i] < 0 {
			return c.NewErrf("private operation does not accept an interface argument")
		}
		arguments[matches[i]] = v
	}
	// Preserve original slots when an earlier positional parameter was
	// omitted. Later supplied arguments must use their labels in that case;
	// compacting them into positional slots would change the call packet.
	call := &CallExpr{}
	positional := true
	for i, p := range protocol.Params {
		v := arguments[i]
		if v == nil {
			if p.Positional {
				positional = false
			}
			continue
		}
		label := p.Label
		if p.Positional && positional {
			label = InvalidLabel
		} else if label == InvalidLabel {
			return c.NewErrf("private operation cannot preserve an omitted positional argument")
		}
		call.Args = append(call.Args, v)
		call.ArgLabels = append(call.ArgLabels, label)
	}
	v := private.call(c, call, state)
	if b, ok := Unwrap(v).(*Bottom); ok {
		return b
	}
	return s.owner.transport(c, env, s.signature.Ret, v, s.outward)
}

type PackageOpen struct {
	Src   *ast.OpenExpr
	Value Expr
	Type  Feature
	View  Feature
	Body  Expr
}

func (o *PackageOpen) Source() ast.Node { return o.Src }
func (*PackageOpen) node()              {}
func (*PackageOpen) expr()              {}
func (*PackageOpen) declNode()          {}
func (*PackageOpen) elemNode()          {}

func (o *PackageOpen) evaluate(c *OpContext, state Flags) Value {
	v, _ := c.Evaluate(c.Env(0), o.Value)
	view, ok := v.(*Vertex)
	if !ok || view.DerefValue().sealed == nil {
		return &Bottom{Src: o.Src, Code: IncompleteError, Err: c.Newf("package witness is not available for opening")}
	}
	view = view.DerefValue()
	p := view.sealed
	if len(p.carriers) != 1 {
		return c.NewErrf("this opening requires exactly one representation type")
	}
	var typ Value
	for _, carrier := range p.carriers {
		typ = &OpaqueType{carrier: carrier}
	}
	// The opened view is a fresh lexical permission, with the same carrier
	// and capability identities. It does not mutate the sealed package.
	fields := &StructLit{}
	for _, a := range view.Arcs {
		fields.Decls = append(fields.Decls, &Field{Label: a.Label, ArcType: a.ArcType, Value: a})
	}
	opened := c.newInlineVertex(nil, nil, MakeRootConjunct(c.Env(0), fields))
	opened.Finalize(c)
	opened.sealed, opened.sealedOpened = p, true
	scope := c.newInlineVertex(nil, nil, MakeRootConjunct(c.Env(0), &StructLit{Decls: []Decl{
		&Field{Label: o.Type, Value: typ}, &Field{Label: o.View, Value: opened},
	}}))
	scope.Finalize(c)
	env := &Environment{Up: c.Env(0), Vertex: scope}
	result, _ := c.Evaluate(env, o.Body)
	if abstractEscapes(c, result, p, make(map[Value]bool)) {
		return c.NewErrf("abstract type escapes its opening scope")
	}
	return protectAbstractScope(c, result, p, make(map[Value]bool))
}

func abstractEscapes(c *OpContext, value Value, owner *sealedPackage, seen map[Value]bool) bool {
	if value == nil || seen[value] {
		return false
	}
	seen[value] = true
	switch v := value.(type) {
	case *OpaqueValue:
		return v.carrier.owner == owner
	case *OpaqueType:
		return v.carrier.owner == owner
	case *Vertex:
		v = v.DerefValue()
		if v.sealed != nil {
			// A closed package binds its own witness. The opened structural
			// view still mentions the local rigid type and cannot escape.
			return v.sealed == owner && v.sealedOpened
		}
		v.Finalize(c)
		for _, a := range v.Arcs {
			if !a.Label.IsLet() && abstractEscapes(c, a, owner, seen) {
				return true
			}
		}
		if value := Unwrap(v); value != v {
			return abstractEscapes(c, value, owner, seen)
		}
	case *Conjunction:
		for _, term := range v.Values {
			if abstractEscapes(c, term, owner, seen) {
				return true
			}
		}
	case *Disjunction:
		for _, term := range v.Values {
			if abstractEscapes(c, term, owner, seen) {
				return true
			}
		}
	case *FuncValue:
		for _, param := range v.Fn.Params {
			if param.Value != nil {
				x, _ := c.Evaluate(v.Env, param.Value)
				if abstractEscapes(c, x, owner, seen) {
					return true
				}
			}
		}
		if v.Fn.Ret != nil {
			x, _ := c.Evaluate(v.Env, v.Fn.Ret)
			return abstractEscapes(c, x, owner, seen)
		}
	}
	return false
}

func (f *FuncValue) checkResultScopes(c *OpContext, value Value) Value {
	for _, owner := range f.scopes {
		if abstractEscapes(c, value, owner, make(map[Value]bool)) {
			return c.NewErrf("abstract type escapes its opening scope through a returned function")
		}
		value = protectAbstractScope(c, value, owner, make(map[Value]bool))
	}
	return value
}

// OpaqueScope protects values contributed by a pattern or an open list tail
// when a later refinement materializes them. The original subject supplies
// the constraints in their lexical environments, including the matched label.
type OpaqueScope struct {
	subject *Vertex
	owner   *sealedPackage
}

func (*OpaqueScope) Source() ast.Node { return nil }
func (*OpaqueScope) node()            {}
func (*OpaqueScope) expr()            {}
func (*OpaqueScope) declNode()        {}
func (*OpaqueScope) elemNode()        {}

func (s *OpaqueScope) evaluate(c *OpContext, state Flags) Value {
	label := c.Env(0).DynamicLabel
	if label == InvalidLabel {
		return &Bottom{Code: IncompleteError, Err: c.Newf("abstract scope awaits a concrete field")}
	}
	field := c.newInlineVertex(nil, nil)
	field.Label = label
	s.subject.MatchAndInsert(c, field)
	field.Finalize(c)
	if abstractEscapes(c, field, s.owner, make(map[Value]bool)) {
		return c.NewErrf("abstract type escapes its opening scope through a field constraint")
	}
	return protectAbstractScope(c, field, s.owner, make(map[Value]bool))
}

// Retain non-escape obligations on returned closures, including closures
// nested in records and lists. The extra view is conjoined with the original
// vertex, preserving its presence, pattern and validation constraints.
// Neither the original value nor any shared closure is mutated.
func protectAbstractScope(c *OpContext, value Value, owner *sealedPackage, seen map[Value]bool) Value {
	if value == nil || seen[value] {
		return value
	}
	seen[value] = true
	defer delete(seen, value)
	if v, ok := value.(*Vertex); ok {
		// Optional fields may not have been demanded by the enclosing
		// record. Their closures still need the deferred escape check.
		v.Finalize(c)
	}
	if d, ok := Unwrap(value).(*Disjunction); ok {
		copy := *d
		copy.Values = slices.Clone(d.Values)
		changed := false
		for i, term := range d.Values {
			copy.Values[i] = protectAbstractScope(c, term, owner, seen)
			changed = changed || copy.Values[i] != term
		}
		if !changed {
			return value
		}
		// Keep branch guards and preference information on the original
		// subject as well as on the protected alternatives.
		result := c.newInlineVertex(nil, nil, MakeRootConjunct(nil, &copy), MakeRootConjunct(nil, value))
		result.Finalize(c)
		return result
	}
	if f, ok := Unwrap(value).(*FuncValue); ok {
		if slices.Contains(f.scopes, owner) {
			return value
		}
		copy := *f
		copy.scopes = append(slices.Clone(f.scopes), owner)
		return &copy
	}
	v, ok := value.(*Vertex)
	if !ok {
		return value
	}
	v = v.DerefValue()
	if v.sealed != nil || v.Kind()&(StructKind|ListKind) == 0 {
		return value
	}
	var extra Expr
	if v.IsList() {
		list := &ListLit{}
		changed := !v.IsClosedList() && v.PatternConstraints != nil
		for a := range v.Elems() {
			x := protectAbstractScope(c, a, owner, seen)
			changed = changed || x != a
			list.Elems = append(list.Elems, x)
		}
		if !changed {
			return value
		}
		if !v.IsClosedList() {
			list.Elems = append(list.Elems, &Ellipsis{Value: &OpaqueScope{subject: v, owner: owner}})
		}
		extra = list
	} else {
		record := &StructLit{}
		for _, a := range v.Arcs {
			if a.Label.IsLet() {
				continue
			}
			if x := protectAbstractScope(c, a, owner, seen); x != a {
				record.Decls = append(record.Decls, &Field{Label: a.Label, ArcType: a.ArcType, Value: x})
			}
		}
		if pcs := v.PatternConstraints; pcs != nil {
			for _, pc := range pcs.Pairs {
				record.Decls = append(record.Decls, &BulkOptionalField{
					Filter: pc.Pattern, Value: &OpaqueScope{subject: v, owner: owner},
				})
			}
		}
		if len(record.Decls) == 0 {
			return value
		}
		extra = record
	}
	result := c.newInlineVertex(nil, nil, MakeRootConjunct(nil, value), MakeRootConjunct(nil, extra))
	result.Finalize(c)
	return result
}
