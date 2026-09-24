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
	"maps"
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

// SubsumesPackage uses an existing package witness, or the identical scoped
// existential introduction. It never guesses a representation for a record.
func (e *Existential) SubsumesPackage(c *OpContext, value Value) bool {
	if other, ok := Unwrap(value).(*Existential); ok {
		return e.Template == other.Template && e.sameEnvironment(c, other.Env)
	}
	if v, ok := value.(*Vertex); ok && v.DerefValue().sealed != nil {
		return e.validate(c, v) == nil
	}
	return false
}

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
			if e.sameEnvironment(c, p.interfaceType.Env) {
				return nil
			}
			// Reusing a template does not identify its captured predicates.
			// Check this instance using the package's existing shared public
			// witness, rather than forgetting the new environment or choosing
			// an unrelated representation for each field.
			args := make(map[*TypeParameter]Value)
			if p.publicEnv != nil {
				args = maps.Clone(p.publicEnv.types.arguments)
			}
			for param, carrier := range p.carriers {
				args[param] = &OpaqueType{carrier: carrier}
				if p.implementation != nil && v == p.implementation.DerefValue() {
					// A lexical interface wrapper is re-scoped when conjoined
					// with the private implementation. Recheck that view using
					// the seal's supplied representation, before transport.
					args[param] = carrier.representation
				}
			}
			return e.validateWitness(c, quantifiedEnvironment(c, e, args), value)
		}
	}
	if witness, b := e.dataWitness(c); b == nil {
		return e.validateWitness(c, witness.env, value)
	}
	return e.unresolved(c)
}

// Lexical wrappers can move an introduction without changing any of its
// dependencies. Compare those dependencies, not unrelated fields in the
// wrapper. Ordinary incomplete witnesses still require shared value cells;
// equal upper approximations do not establish equal assignments.
func (e *Existential) sameEnvironment(c *OpContext, other *Environment) bool {
	if sameTypeEnvironment(c, e.Env, other) {
		return true
	}
	for _, ref := range e.Template.References {
		x, xok := c.Evaluate(e.Env, ref)
		y, yok := c.Evaluate(other, ref)
		if !xok || !yok || x == nil || y == nil {
			return false
		}
		if a, ok := x.(*Vertex); ok {
			if b, ok := y.(*Vertex); ok && a.DerefValue() == b.DerefValue() {
				continue
			}
		}
		predicate := false
		switch r := ref.(type) {
		case *TypeReference:
			predicate = true
		case *FieldReference:
			predicate = r.Label.IsDef()
		case *LetReference:
			predicate = r.IsPredicate
		}
		if predicate {
			if !c.provesInclusion(x, y) || !c.provesInclusion(y, x) {
				return false
			}
		} else if !concreteCapture(c, x) || !concreteCapture(c, y) || !Equal(c, x, y, CheckStructural) {
			return false
		}
	}
	return true
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
	if !maps.Equal(a.types.erasedIndices, b.types.erasedIndices) {
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
	m := checkMembership(c, value, scopedPredicate{env, e.Template.Body})
	v := m.meet
	if b := v.Bottom(); b != nil {
		// This child belongs to the private membership check. Report it at
		// the validator boundary so recursive validation cannot skip it.
		copy := *b
		copy.ChildError, copy.HasRecursive = false, false
		return &copy
	}
	if !m.preservesShape(c) {
		if impossibleWitnessShape(c, value, v) {
			return c.NewErrf("existential witness requires a field forbidden by the supplied value")
		}
		return e.unresolved(c)
	}
	cfg := &ValidateConfig{Concrete: true, Final: true, Runtime: true}
	if !covariantData(e.Template.Body) {
		// Conjoining an arrow is not evidence that the existing operation
		// satisfies it. Keep the membership obligation pending until its
		// conformance can be established independently.
		cfg.CheckFunction = func(*OpContext, *FuncValue) *Bottom { return e.unresolved(c) }
		cfg.CheckBuiltin = func(*OpContext, *Builtin) *Bottom { return e.unresolved(c) }
	}
	return Validate(c, v, cfg)
}

// An open record can acquire missing fields through later refinement. A
// closed record that forbids such a field already refutes membership. Check
// the original witness: its data projection intentionally omits closedness.
func impossibleWitnessShape(c *OpContext, before, after Value) bool {
	a, aok := Unwrap(before).(*Vertex)
	b, bok := Unwrap(after).(*Vertex)
	if !aok || !bok {
		return false
	}
	for _, field := range b.Arcs {
		if field.Label.IsLet() || field.ArcType == ArcOptional {
			continue
		}
		original := a.LookupRaw(field.Label)
		if original == nil {
			if !a.Accept(c, field.Label) {
				return true
			}
		} else if impossibleWitnessShape(c, original, field) {
			return true
		}
	}
	return false
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
	interfaceType  *Existential
	implementation *Vertex
	publicSchema   *Vertex
	privateEnv     *Environment
	publicEnv      *Environment
	carriers       map[*TypeParameter]*opaqueCarrier
	operations     []opaqueAdapter
}

type opaqueCarrier struct {
	owner          *sealedPackage
	parameter      *TypeParameter
	representation Value
	// level is the representation binder's universe, inferred from the
	// witness when unannotated. Opening must not lower an abstract sort.
	level int
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

// Subsumes compares public carrier identity without inspecting representation.
func (x *OpaqueType) Subsumes(v Value) bool {
	switch v := Unwrap(v).(type) {
	case *OpaqueType:
		return x.carrier == v.carrier
	case *OpaqueValue:
		return x.carrier == v.carrier
	}
	return false
}

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
	e := existentialExprOf(c, c.Env(0), s.Interface, make(map[Expr]bool))
	if e == nil {
		return c.NewErrf("seal requires an existential interface")
	}
	q := e.Template
	if len(s.Names) != len(q.Params) {
		return c.NewErrf("seal requires one witness for each interface binder")
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
		if b := param.checkWitness(c, quantifiedEnvironment(c, e, maps.Clone(private)), v); b != nil {
			return b
		}
		if param.ValueRange != nil {
			// A finite value witness has its declared observable sort. It
			// is not a representation type and cannot acquire an opaque
			// carrier merely because it shares a telescope with one.
			private[param], public[param] = v, v
			continue
		}
		level, _ := universeOf(c, v, make(map[Expr]bool)) // checked above
		carrier := &opaqueCarrier{owner: p, parameter: param, representation: v,
			level: max(param.Level, level)}
		p.carriers[param] = carrier
		private[param], public[param] = v, &OpaqueType{carrier: carrier}
	}
	p.privateEnv = quantifiedEnvironment(c, e, private)
	p.publicEnv = quantifiedEnvironment(c, e, public)
	implementation := c.newInlineVertex(nil, nil,
		MakeRootConjunct(c.Env(0), s.Body), MakeRootConjunct(p.privateEnv, q.Body),
		MakeRootConjunct(c.Env(0), s.Interface))
	// The supplied witness discharges only this existential introduction.
	// Its instantiated body and every other interface conjunct still apply.
	implementation.sealed, implementation.sealedOpened = p, true
	// Transport hides representations, not their membership obligations.
	p.implementation = implementation
	implementation.Finalize(c)
	if b := Validate(c, implementation, &ValidateConfig{Concrete: true}); b != nil {
		return b
	}
	// Refined fields also belong to the public interface. Transport its
	// complete shape, rather than projecting just the existential's body.
	schema := c.newInlineVertex(nil, nil, MakeRootConjunct(p.publicEnv, q.Body),
		MakeRootConjunct(c.Env(0), s.Interface))
	schema.sealed, schema.sealedOpened = p, true
	schema.Finalize(c)
	if b := schema.Bottom(); b != nil {
		return b
	}
	p.publicSchema = schema
	view := p.transportResolvedExport(c, schema, implementation, true, true, publicExportGraph(c, schema))
	if v, ok := view.(*Vertex); ok {
		v.sealed = p
		if c.sealedViews == nil {
			c.sealedViews = make(map[sealKey]*Vertex)
		}
		c.sealedViews[key] = v
	}
	return view
}

// Find a top-level existential introduction without mistaking incomplete
// membership of an interface schema for the absence of an introduction.
// Extraction does not discharge any other conjunct of that interface.
func existentialOf(c *OpContext, value Value, seen map[Expr]bool) *Existential {
	if seen[value] {
		return nil
	}
	seen[value] = true
	if b, ok := value.(*Bottom); ok && b.Node != nil {
		return existentialOf(c, b.Node, seen)
	}
	switch v := Unwrap(value).(type) {
	case *Existential:
		return v
	case *Conjunction:
		for _, term := range v.Values {
			if e := existentialOf(c, term, seen); e != nil {
				return e
			}
		}
	}
	if v, ok := value.(*Vertex); ok {
		for conjunct := range v.LeafConjuncts() {
			env, expr := conjunct.EnvExpr()
			if e := existentialExprOf(c, env, expr, seen); e != nil {
				return e
			}
		}
	}
	return nil
}

func existentialExprOf(c *OpContext, env *Environment, expr Expr, seen map[Expr]bool) *Existential {
	if value, ok := expr.(Value); ok {
		return existentialOf(c, value, seen)
	}
	if seen[expr] {
		return nil
	}
	seen[expr] = true
	if r, ok := expr.(Resolver); ok {
		saved := c.PushState(env, expr.Source())
		v := r.resolve(c, Flags{status: partial, condition: arcTypeKnown, mode: yield})
		c.PopState(saved)
		if v != nil {
			return existentialOf(c, v.DerefValue(), seen)
		}
		return nil
	}
	if x, ok := expr.(*BinaryExpr); ok && x.Op == AndOp {
		if e := existentialExprOf(c, env, x.X, seen); e != nil {
			return e
		}
		return existentialExprOf(c, env, x.Y, seen)
	}
	if x, ok := expr.(*StructLit); ok {
		// A lexical wrapper can embed an interface alongside the local
		// definitions or aliases needed by its predicates. The embedded
		// introduction belongs to that wrapper's scope, not its parent.
		v := c.newInlineVertex(nil, nil, MakeRootConjunct(env, x))
		v.Finalize(c)
		scope := &Environment{Up: env, Vertex: v}
		for _, decl := range x.Decls {
			if embedded, ok := decl.(Expr); ok {
				if e := existentialExprOf(c, scope, embedded, seen); e != nil {
					return e
				}
			}
		}
		return nil
	}
	v, _ := c.Evaluate(env, expr)
	return existentialOf(c, v, seen)
}

func quantifiedEnvironment(c *OpContext, e *Existential, args map[*TypeParameter]Value) *Environment {
	return &Environment{Up: e.Env, Vertex: c.newInlineVertex(nil, &StructMarker{}),
		types: &typeScope{quantifier: e.Template, arguments: args}}
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
	return p.transportResolvedExport(c, schema, value, outward, false, nil)
}

func (p *sealedPackage) transportResolvedExport(c *OpContext, schema, value Value, outward, project bool, export *publicExport) Value {
	return p.planTransport(c, scopedPredicate{expr: schema}, outward).apply(c, value, project, export)
}

// A nested package is an existential value, not a record to reconstruct.
// An independent boundary transports by identity, retaining its witness,
// generativity, and implementation obligations. Borrowed abstract fields
// need a stronger transport rule and must remain incomplete.
func (p *sealedPackage) transportPackage(c *OpContext, env *Environment, schema Expr, value Value) Value {
	v, ok := value.(*Vertex)
	if !ok {
		return nil
	}
	v.Finalize(c)
	v = v.DerefValue()
	if v.sealed == nil || (v.sealed == p && v.sealedOpened) {
		return nil
	}
	if v.sealed != p {
		// The private package can be independent while its public schema
		// borrows this boundary's carrier. Identity transport would then
		// expose the private instantiation of that public dependency.
		var typ Value
		complete := true
		if schema != nil {
			typ, complete = c.Evaluate(env, schema)
		}
		if !complete || abstractEscapes(c, typ, p, make(map[Value]bool)) ||
			abstractEscapes(c, v, p, make(map[Value]bool)) {
			return &Bottom{Code: IncompleteError, Err: c.Newf("dependent package transport remains unresolved")}
		}
	}
	return value
}

// OpaqueCall is an authorized adapter. Traversals intentionally cannot visit
// its private implementation or capture environment.
type opaqueAdapter struct {
	export  *publicExport
	schema  *Function
	types   []FuncType
	env     *Environment
	target  *FuncValue
	outward bool
	value   *FuncValue
}

type OpaqueCall struct {
	export    *publicExport
	owner     *sealedPackage
	private   *FuncValue
	signature *Function
	env       *Environment
	outward   bool

	// All views of an overloaded operation share one boundary identity.
	// A selected execution view disables dispatch while retaining every
	// advertised clause as a result obligation on its FuncValue.
	clauses  []FuncType
	root     *OpaqueCall
	dispatch bool
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
				if s.owner.carriers[p] == nil {
					bindings[p] = v
				}
			}
		}
	}
	// The adapter's code is shared across all erased instances, but its
	// input and output transport must use this call's selected predicates.
	env := instantiateEnvironment(s.env, bindings)
	plan := s.planOperation(c, env)
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
		v := plan.inputs[i].apply(c, value, false, nil)
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
	return plan.result.apply(c, v, false, nil)
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
	if !ok {
		return &Bottom{Src: o.Src, Code: IncompleteError, Err: c.Newf("package witness is not available for opening")}
	}
	view = view.DerefValue()
	if view.sealed == nil {
		var b *Bottom
		view, b = openDataWitness(c, view)
		if b != nil {
			return b
		}
	}
	p := view.sealed
	if len(p.carriers) != 1 {
		return &Bottom{Src: o.Src, Code: IncompleteError,
			Err: c.Newf("opening multiple representation types is not yet supported")}
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
	case *WitnessType:
		// A singleton retains the witness as a predicate dependency even
		// when its current upper approximation looks like ordinary data.
		witness, _ := c.Evaluate(v.Env, v.Ref.X)
		return abstractEscapes(c, v.Upper, owner, seen) ||
			abstractEscapes(c, witness, owner, seen)
	case *Vertex:
		v = v.DerefValue()
		if v.sealed == owner {
			// A closed package binds its own witness. The opened structural
			// view still mentions the local rigid type and cannot escape.
			return v.sealedOpened
		}
		if v.sealed != nil && v.sealed.publicSchema != nil &&
			abstractEscapes(c, v.sealed.publicSchema, owner, seen) {
			// Another seal binds only its own witness, not free abstract
			// dependencies in its public interface. Never inspect its
			// private implementation or representation witnesses here.
			return true
		}
		v.Finalize(c)
		for _, a := range v.Arcs {
			if !a.Label.IsLet() && abstractEscapes(c, a, owner, seen) {
				return true
			}
		}
		if pcs := v.PatternConstraints; pcs != nil {
			for _, pc := range pcs.Pairs {
				if abstractEscapes(c, pc.Pattern, owner, seen) ||
					abstractEscapes(c, pc.Constraint, owner, seen) {
					return true
				}
			}
		}
		if value := Unwrap(v); value != v {
			return abstractEscapes(c, value, owner, seen)
		}
	case *Existential:
		return abstractReferencesEscape(c, v.Template, v.Env, owner, seen)
	case *Universal:
		return abstractReferencesEscape(c, v.Template, v.Env, owner, seen)
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
		for _, t := range v.selectionAndOriginalClauses() {
			if abstractFunctionTypeEscapes(c, t, owner, seen) {
				return true
			}
		}
	case *Builtin:
		for _, t := range v.Types {
			if abstractFunctionTypeEscapes(c, t, owner, seen) {
				return true
			}
		}
	}
	return false
}

// Check public predicates, including retained clauses and telescope bounds.
// Private implementation captures remain permitted: checkResultScopes checks
// what a returned closure exposes when it is eventually called.
func abstractFunctionTypeEscapes(c *OpContext, t FuncType, owner *sealedPackage, seen map[Value]bool) bool {
	for _, param := range t.Fn.Params {
		if abstractPredicateEscapes(c, t.Env, param.Value, owner, seen) {
			return true
		}
	}
	if abstractPredicateEscapes(c, t.Env, t.Fn.Ret, owner, seen) {
		return true
	}
	for e := t.Env; e != nil; e = e.Up {
		if e.types == nil {
			continue
		}
		for _, p := range e.types.quantifier.Params {
			if e.types.arguments[p] == nil &&
				(abstractPredicateEscapes(c, e, p.Bound, owner, seen) ||
					abstractPredicateEscapes(c, e, p.ValueRange, owner, seen)) {
				return true
			}
		}
	}
	return false
}

func abstractPredicateEscapes(c *OpContext, env *Environment, expr Expr, owner *sealedPackage, seen map[Value]bool) bool {
	if expr == nil {
		return false
	}
	// Evaluating a compound predicate with an uninstantiated binder can
	// yield only an incomplete bottom. Inspect its independent premises
	// before evaluation so that this does not hide another free carrier.
	switch x := expr.(type) {
	case *BinaryExpr:
		return abstractPredicateEscapes(c, env, x.X, owner, seen) ||
			abstractPredicateEscapes(c, env, x.Y, owner, seen)
	case *DisjunctionExpr:
		for _, term := range x.Values {
			if abstractPredicateEscapes(c, env, term.Val, owner, seen) {
				return true
			}
		}
	}
	value, _ := c.Evaluate(env, expr)
	return abstractEscapes(c, value, owner, seen)
}

func abstractReferencesEscape(c *OpContext, q *Quantified, env *Environment, owner *sealedPackage, seen map[Value]bool) bool {
	for _, ref := range q.References {
		value, _ := c.Evaluate(env, ref)
		if abstractEscapes(c, value, owner, seen) {
			return true
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
