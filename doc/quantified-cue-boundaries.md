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
