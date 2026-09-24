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

// existentialDataWitness is the constructive rule behind covariant data
// membership. Its greatest admissible witnesses attain the existential join.
// Keeping this construction explicit lets elimination use the very witness
// justified by membership, instead of assuming all members were sealed.
type existentialDataWitness struct {
	env       *Environment
	arguments map[*TypeParameter]Value
}

func (e *Existential) dataWitness(c *OpContext) (*existentialDataWitness, *Bottom) {
	if !covariantData(e.Template.Body) {
		return nil, e.unresolved(c)
	}
	for _, param := range e.Template.Params {
		if param.ValueRange != nil || !covariantData(param.Bound) {
			return nil, e.unresolved(c)
		}
	}
	witness := &existentialDataWitness{
		env: quantifiedEnvironment(c, e, nil), arguments: make(map[*TypeParameter]Value),
	}
	for _, param := range e.Template.Params {
		var bound Value = &Top{}
		if param.Bound != nil {
			var complete bool
			bound, complete = c.Evaluate(witness.env, param.Bound)
			if !complete {
				return nil, e.unresolved(c)
			}
		}
		if b := param.checkWitness(c, witness.env, bound); b != nil {
			return nil, b
		}
		witness.arguments[param] = bound
		witness.env = instantiateEnvironment(witness.env, map[*TypeParameter]Value{param: bound})
	}
	return witness, nil
}

type dataWitnessKey struct {
	subject  *Vertex
	template *Quantified
	env      *Environment
}

// openDataWitness constructs a view only from the same covariant rule used by
// existential membership. It neither guesses an arbitrary representation nor
// refines the original subject into an inhabitant. Repeated elimination of a
// shared subject preserves its witness; opening itself is not a fresh seal.
func openDataWitness(c *OpContext, subject *Vertex) (*Vertex, *Bottom) {
	e := existentialOf(c, subject, make(map[Expr]bool))
	if e == nil {
		return nil, &Bottom{Code: IncompleteError, Err: c.Newf("package witness is not available for opening")}
	}
	key := dataWitnessKey{subject: subject, template: e.Template, env: e.Env}
	if v := c.dataWitnessViews[key]; v != nil {
		return v, nil
	}
	witness, b := e.dataWitness(c)
	if b != nil {
		return nil, b
	}
	if !concreteCapture(c, subject) {
		return nil, e.unresolved(c)
	}
	if b := e.validateWitness(c, witness.env, subject); b != nil {
		return nil, b
	}
	p := &sealedPackage{interfaceType: e, implementation: subject, privateEnv: witness.env,
		carriers: make(map[*TypeParameter]*opaqueCarrier)}
	public := make(map[*TypeParameter]Value)
	for _, param := range e.Template.Params {
		if param.Bound != nil {
			return nil, &Bottom{Code: IncompleteError,
				Err: c.Newf("opening a bounded transparent witness remains unresolved")}
		}
		representation := witness.arguments[param]
		level, _ := universeOf(c, representation, make(map[Expr]bool))
		carrier := &opaqueCarrier{owner: p, parameter: param, representation: representation,
			level: max(param.Level, level)}
		p.carriers[param] = carrier
		public[param] = &OpaqueType{carrier: carrier}
	}
	p.publicEnv = quantifiedEnvironment(c, e, public)
	p.publicSchema = c.newInlineVertex(nil, nil, MakeRootConjunct(p.publicEnv, e.Template.Body))
	p.publicSchema.Finalize(c)
	plan := p.planTransport(c, scopedPredicate{expr: p.publicSchema}, true)
	view := plan.apply(c, subject, true, publicExportGraph(c, p.publicSchema))
	if b, ok := Unwrap(view).(*Bottom); ok {
		return nil, b
	}
	v, ok := view.(*Vertex)
	if !ok {
		return nil, e.unresolved(c)
	}
	v.sealed = p
	if c.dataWitnessViews == nil {
		c.dataWitnessViews = make(map[dataWitnessKey]*Vertex)
	}
	c.dataWitnessViews[key] = v
	return v, nil
}
