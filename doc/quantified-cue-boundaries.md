# Quantified CUE boundary architecture

This work implements the boundary requirements of the supported fragments in
`quantified-cue.tex` and addresses the audit of revision `64815d1e6`. The existing constraint lattice remains the
semantic carrier. Compatibility, membership, executable witnesses, and proofs
of successful execution are different judgments over that carrier.

## Implemented boundaries

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

## Acceptance criteria

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

The implementation evidence, consumer review, verification results, and
remaining supported-profile limits are recorded below.

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
  definition projection; the full core/public API suite passes. Unchanged
  pattern regions retain their label aliases and public lexical dependencies.
  Partitioning uses the same child plans as execution, and complement checks
  use the checking operand to avoid replaying their own validator.
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

- Call-result normalization retains universal subject introductions and seals.
  A scalar or closed-list approximation cannot erase subsequent selection,
  consumed-binder checks, or universe bounds. Tests cover direct and nested
  subjects through ordinary identity calls and opaque operations.

## Audit resolution and consumer review

| Original finding | Shared boundary and evidence |
| --- | --- |
| 1. Enriched abstract-call admission | `callPacket.admit`; abstract, selected, opaque, builtin, and guarded-result consumers retain original packet evidence. Presence and API-refinement tests distinguish unknown from refuted. |
| 2. Contract-sensitive nested equality | `compareRuntimeValues`; ordinary data observations, closure captures, saved arguments, singleton membership, and opaque inhabitants use recursive runtime identity. |
| 3. Lost record predicates | Identity transport retains the graph; changing transport retains its inverse-image predicate and projectable predicates. Optional, closed, hidden, nested, pattern, definition, API-refinement, and callback cases have positive and negative controls. |
| 4. Unexecutable existential elimination | Covariant membership and opening share `existentialDataWitness`; certification also requires total construction of the public view. |
| 5. Unsorted seal witnesses | Selection and sealing share `TypeParameter.checkWitness`; mixed telescope orders preserve value witnesses and reject invalid ranges. |
| 6. Mis-scoped list transport | Execution and `OpaqueCall.ProofTypes` consume the same `operationTransport`; children and tails are resolved in their declaration scopes. |
| 7. Untranslated definitions | `transportPlan.predicate` transforms definition and optional predicates without demanding runtime witnesses; nested projection tests retain narrower constraints. |
| 8. Head-clause-only certification | `Obligations`, `CallClausesFor`, and `ResultClausesFor` preserve intersections, selection, partial coordinates, and guarded consequences. A finite identity model compares certification with execution. |

The consumer review also checked the following points:

- The only constructor of successful `packetAdmission` is the shared admission
  check. Execution activations do not replace its original packet.
- Graph `Equal`, environment sharing, and call-cache equality remain stricter
  than runtime identity. They must retain contracts and formation metadata;
  replacing them wholesale with runtime equality would introduce new losses.
- The former transport AST executor and independent totality walker are gone.
  Callable inverse checking recovers the original descriptor only as a checking
  operand; actual adapters retain their protocols and contract obligations.
- Callback hypotheses originate from admitted runtime components. Definitions,
  absent optional fields, and constructed bodyless schemas do not supply them.
- Transport preserves both predicate denotation and visible dependencies.
  Pattern regions with unchanged representation retain their original scopes;
  changed regions remain checked against the original graph. Missing codecs
  cause incomplete source export rather than private representation disclosure.
- Formation and escape walkers account for retained transport predicates.
  Runtime call-result detachment cannot erase a universal introduction or seal.
- Membership and pattern complement checks do not replay their own validators.
  Recursive transport checks and unresolved proofs remain incomplete.

## Verification results

Verified on 24 September 2026 with Go 1.27.1 on darwin/arm64:

| Gate | Result |
| --- | --- |
| `go test ./cue ./internal/core/...` | Passed. |
| `go test ./...` | Passed with localhost access for registry and language-server test servers. |
| `tools/test-quantified-oracles.sh fast` | Passed, including all boundary tests. |
| `tools/test-quantified-oracles.sh extended` | Passed, followed by all five 30-second native fuzz runs. |
| Extended feature-combination model with its residual allowance removed | All 2,432 valid cases established; all 3,840 invalid promises rejected; zero residual cases. |
| `go test -race ./cue -run '^(TestQuantifiedBoundary|TestQuantifiedSemantic)'` | Passed. |
| `go test -race ./internal/core/adt -run '^TestEvalV3$/quantified'` | Passed. |
| `go test -race ./internal/core/subsume` | Passed. |
| `git diff --check` | Passed. |

The extended run includes 262,144 exhaustive finite-quantifier cases, 122,880
finite dependent-domain cases, 25,088 record-membership cases, 5,120 generated
preservation transformations, 512 dependent subtype chains, and 540,021,248
packet-domain comparisons. The new independent transport model adds 576 cases.
The feature model's previous allowance for 448 incomplete positive cases has
been removed: every valid case in that supported vocabulary must now certify
and execute. General proof limits remain tested separately.

The first sandboxed repository run could not bind local test-server ports.
Rerunning with localhost access passed; no test expectation was relaxed to
work around that restriction.

## Remaining supported-profile limits

This is not an implementation of the entire denotational reference model.
The [implementation guide](quantified-cue-implementation.md) remains the supported
feature contract. General dependent profile D, unrestricted Boolean quantified
inclusion, arbitrary termination proofs, and effect/extern implementation proofs
remain unsupported. Opening is limited to record-shaped interfaces with one
unbounded representation carrier; constructive transparent opening uses the
covariant membership rule. Overlapping or recursive transports without a
sufficient totality proof remain incomplete. Opaque source export requires an
interface codec. These limits do not justify certifying an operation whose
admitted execution fails, or turning an unresolved judgment into a refutation.

The finite models and transformations provide independent executable evidence
for their stated vocabularies. They are not a formal proof of all possible CUE
programs or a guarantee that no future interaction defect can exist.
