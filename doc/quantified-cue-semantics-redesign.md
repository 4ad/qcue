# Quantified CUE semantic preservation

The redesign separates operations that previously shared an evaluator result
without sharing its meaning. A successful meet establishes compatibility; it
does not, by itself, establish membership, implementation conformance, packet
domain disjointness, or runtime identity.

## Required boundaries

1. **Membership.** Keep the supplied subject separate from the scoped predicate
   and the temporary meet used to check it. Preserve package-qualified hidden
   fields, definitions, presence, optional constraints, patterns and closedness.
   A meet may refine a candidate, but cannot manufacture evidence that the
   original packet belonged to a guard. Checking a validator must not replay
   that same validator, nor project the subject to JSON data to avoid replay.
2. **Observation.** Schema validation and validation of a supplied runtime
   packet have different field policies. Hidden runtime fields carry the same
   conformance obligations as ordinary fields. Definitions remain predicates;
   they are not required to materialize merely because their containing record
   is used as an argument. Missing optional fields remain absent.
3. **Quantification.** A subject's runtime kind does not determine whether it
   retains a universal introduction. Formation and source export must retain
   the telescope even when evaluation reduces its data approximation to a
   scalar. A selected view retains original obligations separately from the
   remaining telescope available to subsequent elimination.
4. **Packet domains.** Parameter alignment, protocol inclusion and intersection
   of packet domains are separate judgments. Disjointness must account for
   positional and labeled alternatives, omission and defaults. Unknown overlap
   cannot certify incompatible transports.
5. **Opaque identity.** Public operation identity follows the interface export
   and alias graph. It is independent of signature AST sharing and private
   closure sharing. Copies, selected views, partial applications and inverse
   callback transports must preserve the appropriate identity.
6. **Pending computations.** Graph deduplication compares expressions together
   with their lexical environments. The same syntax under different binder
   assignments is not the same pending obligation. Environments also retain
   dynamic pattern labels. Equal runtime data alone does not justify discarding
   a different universe obligation or selection telescope.

## Implemented representation and judgment boundaries

- [membership.go](../internal/core/adt/membership.go) keeps `subject`,
  `scopedPredicate`, and the compatibility `meet` in separate slots.
  `evaluatedSubject` is a private evaluator input for checking the evaluated
  root without replaying its validator. Unlike data projection, its insertion
  preserves all field classes, presence, patterns, list tails and closedness.
  It is never returned as a replacement subject. Packet membership additionally
  checks the original subject's completeness, absence observations and callable
  inclusion. Existential checks retain the original graph and its other
  obligations, while using the same compatibility and shape boundaries.
- [validate.go](../internal/core/adt/validate.go) gives supplied runtime packets,
  captured environments and package implementations a field policy separate
  from ordinary host schema validation. Hidden runtime callables require their
  own conformance proofs; definitions and absent optionals remain schemas.
- [subject.go](../internal/core/adt/subject.go) owns universal introduction and
  elimination metadata independently of the data kind. Graph sharing and
  structural equality check that metadata. A callable's `frontier` is its
  remaining selection telescope; `Types` remains the complete obligation set.
  Explicit selection and composite projection have separate export provenance,
  including later refinements. Universe checks inspect both the original
  introduction and its selected view before unwrapping scalar data.
- [function_views.go](../internal/core/adt/function_views.go) keeps a conjunction
  of checked instance views separate from their shared closure and original
  universal obligations. For example, both orders of `id[int] & id[string]`
  admit integer and string calls. Admission selects a proof view, executes one
  body, and retains all applicable result predicates. Explicit instance checks
  still apply to `id[int]` alone. Partial application, remaining type selection,
  call memoization and source export retain the combined views.
- [packet_domain.go](../internal/core/adt/packet_domain.go) computes families of
  common packet shapes for closed rows. Optional labels factor independently;
  no exponential subset enumeration is needed in the implementation. Opaque
  coherence checks transport agreement for every potentially overlapping
  family. A failed alignment or failed overlap proof is not disjointness.
- [opaque_identity.go](../internal/core/adt/opaque_identity.go) builds the public
  export and alias graph before transport. Signature abbreviations and private
  closure sharing do not identify its nodes. Selection, nested projections,
  copies and explicit aliases retain their public identities. Higher-order
  transport still preserves supplied callback identities and inverse transport.
- [disjunct2.go](../internal/core/adt/disjunct2.go) includes lexical environments
  in pending-task equality. This fixes a pre-existing counterexample discovered
  by the independent model: `forall x exists y {ok: true & (x != y)}` over
  `{0,1}` was incorrectly refuted because distinct assignments were merged.

The host evaluator still uses `Vertex` for scheduling, storage and ordinary CUE
unification. This change makes its semantic uses explicit at the quantified
boundaries; it does not introduce another evaluator. Inference may construct a
wider candidate predicate, but it must still independently establish admission
of every original supplied argument. A candidate approximation is not evidence.

The supported fragment remains the one described in
[the implementation guide](quantified-cue-implementation.md). General dependent
profile D, unrestricted quantifier proof search, general termination proofs and
unsupported opaque transports are separate extensions. Unknown judgments remain
incomplete rather than being accepted or refuted by approximation.

## Verification

The seven counterexamples in the audit are regression requirements, not the
completion criterion. Generated checks must additionally compare equivalent
programs across field classes and nesting, declaration and conjunction order,
alias expansion, private implementation sharing, explicit selection, API
refinement and source export/reimport. A small independent finite semantic
model supplies expected membership and packet-intersection results; successful
examples alone cannot certify a universal property.

Ordinary validation, concrete certification and selected-value observations
are checked separately. Refutation and incompleteness are separate outcomes.
Unsupported proof search may remain incomplete, but cannot manufacture a
contradiction or discard a required obligation.

This document records the implementation requirements. Completion is established
by the implementation and tests, not by this checklist or the historical audits.

The executable checks are
[quantified_semantics_test.go](../cue/quantified_semantics_test.go) and
[packet_domain_test.go](../internal/core/adt/packet_domain_test.go):

- All seven findings, with positive controls and field/nesting/order variants.
- A finite record-membership oracle across regular, hidden, definition and
  hidden-definition labels; required/optional presence; two values; both meet
  orders; ordinary and concrete validation.
- A finite quantifier oracle for all 16 binary relations on `{0,1}`, all four
  ordered pairs of universal/existential quantifiers, and all 16 pairs of
  subdomains, including the empty domain: 1,024 cases.
- An independent packet-admission oracle compared with symbolic intersections
  for all 28,561 generated ordered pairs of rows of up to two parameters and
  all 12 packet shapes in that finite universe: 342,732 comparisons.
- API `Unify`/`FillPath` refinement, ordinary/final source export and reimport,
  alpha renaming, consumed-binder rejection, retained universe formation,
  private sharing and alias expansion, nested public aliases, qualified hidden
  labels from distinct packages, patterns, closedness, open list tails, and
  runtime conformance boundaries. Checked-view conjunctions additionally cover
  commutativity, associativity, partial application, remaining selection,
  source round trips, and overlapping result obligations.

These are executable preservation checks for a stated finite vocabulary and
supported transformations, not a proof of the full implementation.

## Expanded verification

The [oracle guide](quantified-cue-oracles.md) records the larger finite universes,
dependent domains and subtype chains, generated feature combinations and
transformations, automatic shrinking, and periodic execution. It separates exact
finite-model results from the opaque-transport cases that remain unproved.
The larger randomized model also found exponential expansion of equivalent
finite alternatives; normalization now collapses proved-equal ground results
without discarding residual predicates or mutable lexical dependencies.
