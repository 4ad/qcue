// Copyright 2020 CUE Authors
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

package adt

type ValidateConfig struct {
	// Runtime checks a supplied packet or captured runtime environment. Hidden
	// fields are observable components of it, and must discharge concreteness
	// and callable obligations just like regular fields. Definitions retain
	// their schema interpretation. The default policy is host CUE validation.
	Runtime bool

	// Concrete, if true, requires that all values be concrete.
	Concrete bool

	// Final, if true, checks that there are no required fields left.
	Final bool

	// DisallowCycles indicates that there may not be cycles.
	DisallowCycles bool

	// ReportIncomplete reports an incomplete error even when concrete is not
	// requested.
	ReportIncomplete bool

	// AllErrors continues descending into a Vertex, even if errors are found.
	AllErrors bool

	// CheckFunction validates the contracts of a concrete function value.
	// Public validation supplies the conformance checker here. Internal
	// evaluation can still inspect a closure's concrete representation while
	// retaining its unresolved contract, without claiming conformance.
	CheckFunction func(*OpContext, *FuncValue) *Bottom

	// CheckBuiltin independently proves client-added primitive contracts.
	CheckBuiltin func(*OpContext, *Builtin) *Bottom

	// TODO: omitOptional, if this is becomes relevant.
}

// Validate checks that a value has certain properties. The value must have
// been evaluated.
func Validate(ctx *OpContext, v *Vertex, cfg *ValidateConfig) *Bottom {
	if cfg == nil {
		cfg = &ValidateConfig{}
	}
	x := validator{ValidateConfig: *cfg, ctx: ctx}
	x.validate(v)
	if b := ctx.Cancelled(); b != nil {
		return b
	}
	return x.err
}

// validateValue checks that a value has certain properties. The value must have
// been evaluated.
func validateValue(ctx *OpContext, v Value, cfg *ValidateConfig) *Bottom {
	if cfg == nil {
		cfg = &ValidateConfig{}
	}

	if v.Concreteness() > Concrete {
		return &Bottom{
			Code: IncompleteError,
			Err:  ctx.Newf("non-concrete value '%v'", v),
			Node: ctx.vertex,
		}
	}

	if x, ok := v.(*Vertex); ok {
		if v.Kind()&(StructKind|ListKind) != 0 {
			x.Finalize(ctx)
		}
		return Validate(ctx, x, cfg)
	}

	return nil
}

type validator struct {
	ValidateConfig
	ctx          *OpContext
	err          *Bottom
	inDefinition int

	sharedPositions []Node

	// shared vertices should be visited at least once if referenced by
	// a non-definition.
	// TODO: we could also keep track of the number of references to a
	// shared vertex. This would allow us to report more than a single error
	// per shared vertex.
	visited  map[*Vertex]bool
	packages map[*sealedPackage]bool
}

func (v *validator) validatePackage(p *sealedPackage) {
	if p == nil || !v.checkConcrete() || v.packages[p] {
		return
	}
	if v.packages == nil {
		v.packages = make(map[*sealedPackage]bool)
	}
	v.packages[p] = true
	saved := v.Runtime
	v.Runtime = true
	v.validate(p.implementation)
	v.Runtime = saved
}

func (v *validator) addPositions(err *ValueError) {
	for _, p := range v.sharedPositions {
		err.AddPosition(p)
	}
}

func (v *validator) checkConcrete() bool {
	return v.Concrete && v.inDefinition == 0
}

func (v *validator) checkFinal() bool {
	return (v.Concrete || v.Final) && v.inDefinition == 0
}

func (v *validator) add(b *Bottom) {
	if !v.AllErrors {
		v.err = CombineErrors(nil, v.err, b)
		return
	}
	if !b.ChildError {
		v.err = CombineErrors(nil, v.err, b)
	}
}

func (v *validator) validate(x *Vertex) {
	if b := v.ctx.Cancelled(); b != nil {
		v.err = b
		return
	}
	defer v.ctx.PopArcAndLabel(v.ctx.PushArcAndLabel(x))

	y := x

	if x.IsShared {
		saved := v.sharedPositions
		// assume there is always a single conjunct: multiple references either
		// result in the same shared value, or no sharing. And there has to be
		// at least one to be able to share in the first place.
		c, n := x.SingleConjunct()
		if n >= 1 {
			v.sharedPositions = append(v.sharedPositions, c.Elem())
		}
		defer func() { v.sharedPositions = saved }()
	}
	// Dereference values, but only those that are not shared. This includes let
	// values. This prevents us from processing structure-shared nodes more than
	// once and prevents potential cycles.
	x = x.DerefValue()
	if y != x {
		// Ensure that each structure shared node is processed at least once
		// in a position that is not a definition.
		if v.inDefinition > 0 {
			return
		}
		if v.visited == nil {
			v.visited = make(map[*Vertex]bool)
		}
		if v.visited[x] {
			return
		}
		v.visited[x] = true
	}

	if b := x.Bottom(); b != nil {
		switch b.Code {
		case CycleError:
			if v.checkFinal() || v.DisallowCycles {
				v.add(b)
			}

		case IncompleteError:
			if v.ReportIncomplete || v.checkConcrete() {
				v.add(b)
			}

		default:
			v.add(b)
		}
		if !b.HasRecursive {
			return
		}

	} else if v.checkConcrete() {
		x = x.Default()
		if !IsConcrete(x) {
			err := v.ctx.Newf("incomplete value %v", x.Value())
			for c := range x.LeafConjuncts() {
				err.AddPosition(c.Elem())
			}
			v.addPositions(err)
			v.add(&Bottom{
				Code: IncompleteError,
				Err:  err,
			})
		}
	}

	if f, ok := x.BaseValue.(*FuncValue); ok && len(f.identities) > 0 {
		if b := f.checkIdentities(v.ctx); b != nil {
			if !b.IsIncomplete() || v.ReportIncomplete || v.checkConcrete() {
				v.add(b)
			}
		}
	}
	v.validatePackage(x.sealed)
	if b, ok := x.BaseValue.(*Builtin); ok && b.capabilityMode() && v.checkConcrete() && v.CheckBuiltin != nil {
		if err := v.CheckBuiltin(v.ctx, b); err != nil {
			v.add(err)
		}
	}
	if opaque, ok := x.BaseValue.(*OpaqueValue); ok {
		v.validatePackage(opaque.carrier.owner)
		// An opaque meet retains obligations from both representation
		// graphs. They may include contracts acquired after the seal was
		// created, so checking the original package alone is insufficient.
		private, ok := opaque.private.(*Vertex)
		if !ok {
			private = v.ctx.newInlineVertex(nil, nil, MakeRootConjunct(nil, opaque.private))
			private.Finalize(v.ctx)
		}
		v.validate(private)
	}
	if f, ok := x.BaseValue.(*FuncValue); ok && capabilityMode(f.Fn, f.Types) && v.checkConcrete() {
		if boundary, ok := f.Fn.Body.(*OpaqueCall); ok {
			v.validatePackage(boundary.owner)
		}
		if !concreteCapture(v.ctx, f) {
			v.add(&Bottom{Src: f.Source(), Code: IncompleteError,
				Err: v.ctx.Newf("function implementation or captured values remain unresolved")})
		} else if v.CheckFunction != nil {
			if b := v.CheckFunction(v.ctx, f); b != nil {
				v.add(b)
			}
		}
	}

	for _, a := range x.Arcs {
		if a.ArcType == ArcRequired && v.Final && v.inDefinition == 0 {
			v.ctx.PushArcAndLabel(a)
			v.add(NewRequiredNotPresentError(v.ctx, a, v.sharedPositions...))
			v.ctx.PopArcAndLabel(a)
			continue
		}

		if a.Label.IsLet() || !a.IsDefined(v.ctx) {
			continue
		}
		if !v.AllErrors && v.err != nil {
			break
		}
		if a.Label.IsRegular() || v.Runtime && !a.Label.IsDef() {
			v.validate(a)
		} else {
			v.inDefinition++
			v.validate(a)
			v.inDefinition--
		}
	}
}
