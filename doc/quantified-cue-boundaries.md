# Quantified CUE boundary architecture

This work implements the requirements in `quantified-cue.tex` and addresses
the audit of revision `64815d1e6`. The existing constraint lattice remains the
semantic carrier. Compatibility, membership, executable witnesses, and proofs
of successful execution are different judgments over that carrier.

## Required implementation changes

1. Runtime observations use one recursive inhabitant comparison. Constraint
   graph equality remains separate. Contracts and erased selections cannot
   affect inhabitant identity, including inside containers and captures.
2. Abstract application, selected views, opaque dispatch, inference, and
   guarded result propagation establish admission from the original packet.
   Binding and compatibility cannot themselves discharge that judgment.
3. Transport elaborates scoped schemas once into the representation used by
   both the proof and execution paths. Syntax-specific execution paths must
   not independently interpret lexical levels. Ordinary records retain their
   predicates, optional presence, patterns, and closedness; module projection
   remains an explicit operation. Definitions undergo predicate substitution,
   not runtime materialization.
4. Existential elimination uses an available executable witness, or a witness
   construction justified by the same membership rule. An existential
   proposition alone cannot silently supply a sealed runtime object.
5. Seal witness formation respects binder sorts and dependent ranges. Value
   binders cannot become opaque type carriers.
6. Call certification obtains the whole callable contract through a shared
   interface, including intersections and selected or partial views.

## Evidence required before completion

- Regressions for all eight findings, with concrete validation, ordinary
  validation, and selected observations distinguished.
- Generated semantic preservation checks across containers, field classes,
  aliases, conjunction order, partial calls, and explicit type selection.
- Checks relating certification and execution on independently enumerated
  admitted inputs, preserving unknown versus refuted judgments.
- Tests that exercise the shared boundaries through different public routes,
  including API refinement and source export where supported.
- Existing evaluator, subsumption, public API, and full repository tests;
  the extended finite-model suite and relevant fuzz/race checks.
- A final review of every boundary consumer and each audit finding against
  the implemented design. Green example regressions alone are insufficient.

This document is a work plan, not a claim that these requirements have been
implemented or verified. Implementation evidence and remaining limitations
will be recorded as the work proceeds.

## Implementation evidence

- Runtime observations now share `compareRuntimeValues`: data equality uses
  the ordinary visible-field policy, while opaque inhabitants and captures
  retain hidden runtime fields. Definitions remain predicates. Recursive or
  incomplete comparisons remain unknown. `Equal` remains constraint-graph
  equality and is not used for callable leaves of runtime observations.
  `TestQuantifiedBoundaryRuntimeEquality` exercises origin, captures, saved
  packets, redundant contracts, erased selections, builtins, and nesting;
  `TestQuantifiedBoundaryOpaqueEquality` checks the same opaque comparison
  directly and through containers. `go test ./cue ./internal/core/...` passes.
- `callPacket` retains normalized supplied arguments and their lexical
  environments separately from execution activations. Abstract calls,
  selected views, opaque overload dispatch, builtin contracts, and ordinary
  guarded results use its shared admission operation. An abstract call now
  propagates all proved result clauses without executing a synthetic body.
  Inference uses original-packet membership for concrete candidates and
  predicate inclusion for symbolic proof inputs. Presence/nesting matrices
  and API/source refinement regressions pass, as does the public API and
  complete core test suite.
- `transportPlan` resolves schemas in their declaration environments and
  supplies both execution and totality certification. `operationTransport`
  gives opaque calls and their proofs the same input/result interpretation.
  The separate AST executor and totality walker have been removed. Ordinary
  identity branches retain the original graph; list unions and inward
  definitions use the resolved scoped schema. New regressions and existing
  opaque transport tests pass with the full core/public API suite. Nonidentity
  composite plans now retain an inverse-image predicate over the original
  graph, so later refinements recheck closedness, patterns, and correlations.
  Definitions and absent optional fields retain transported predicates,
  including narrower constraints observed by projection. Inverse checking
  recovers an original callable descriptor rather than substituting the
  narrower protocol of an inverse adapter. New positive/negative matrices
  cover direct and callback transport, source and API refinement, and nested
  definition projection; the full core/public API suite passes. The final
  preservation and boundary audit remains pending.
- `TypeParameter.checkWitness` is shared by type application and sealing.
  It dispatches by the declared binder sort, checks value-range membership
  or type-universe/bound inclusion, and retains unknown formation obligations.
  Mixed finite-value/type seals preserve value witnesses as values and create
  carriers only for type witnesses. Tests vary telescope order and valid,
  out-of-range, wrong-kind, and incomplete witnesses; core/API suites pass.
- Covariant existential membership now constructs an `existentialDataWitness`.
  Opening uses that same construction for admitted transparent records and
  retains a shared subject's witness across repeated elimination and copying.
  Proof-level opening checks that this construction has a total public
  transport; ambiguous union transport remains an explicit proof obligation.
  Existing sealed packages retain their original witness. Regressions cover
  certified clients on ordinary records, nested data, refinements, shared
  projections, and copied subjects. Core/public API suites pass.
- Call contracts expose obligations, available call clauses, and guarded
  result clauses separately. Certification and abstract application use this
  interface, including intersections, selected views, partial coordinates,
  and inference from saved plus remaining arguments. Finite union packets
  are checked branch by branch under the proof budget. Constructed proof
  records/lists preserve projected callback evidence instead of relying on
  the identity of re-evaluated schema approximations. A finite identity model
  checks overload coverage and result inclusion against execution; additional
  tests cover clause order, nesting, partial application, and rejected
  attempts to manufacture evidence or reopen selected domains.

- The final consumer audit found and repaired another predicate/witness mixup:
  callback hypotheses could be introduced from definition fields. Admission
  now introduces hypotheses only from present runtime fields. Tests vary
  regular, hidden, definition, hidden-definition, and optional fields through
  direct records, nested records, and lists, and compare certification with
  actual application. The full core/public API suite passes.
