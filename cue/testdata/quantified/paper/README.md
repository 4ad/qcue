# Paper examples

All 111 listings from [the paper](../../../../doc/paper.tex), in source
order. Each archive preserves the exact listing in `paper.cue.txt`; executable
`in.cue` includes any required surrounding declarations and explicit assertions.
`TestQuantifiedPaperIndex` detects missing or changed listings.

Keep `paper.cue.txt` byte-for-byte identical to the listing, including its
shorthand declarations, comments, and whitespace. All archives use `#noformat`
to protect the original source during bulk fixture formatting. Supporting
declarations, assertions, and executable adaptations belong in `in.cue`.
Additional examples may be added in [the examples directory](../examples/).
[Historical paper examples](../paper_history/) preserve earlier listings and
their regression assertions when the current paper changes or removes them.

The 88 executable examples run in `TestEvalV3/quantified/paper`. Two syntax
templates are parsed by the index test. Twenty D examples and one pseudocode
listing are retained with explicit exclusion reasons. Expected contradictions
and blocked static judgments have explicit assertions. Failed computations
are also observed as bottom to distinguish them from blocked judgments.

| Listing | Topic | Coverage |
| --- | --- | --- |
| 001 | [Descriptions and completions](001-descriptions-and-completions.txtar) | Evaluation |
| 002 | [Independent instantiations of a description](002-independent-instantiations-of-a-description.txtar) | Evaluation |
| 003 | [One subject at every instance](003-one-subject-at-every-instance.txtar) | Evaluation |
| 004 | [Refining a polymorphic record across declarations](004-refining-a-polymorphic-record-across-declarations.txtar) | Evaluation |
| 005 | [Two bounds do not become one upper bound](005-two-bounds-do-not-become-one-upper-bound.txtar) | Evaluation |
| 006 | [An ordinary field floats as a constant obligation](006-an-ordinary-field-floats-as-a-constant-obligation.txtar) | Evaluation |
| 007 | [Binder scope across a union](007-binder-scope-across-a-union.txtar) | D (out of scope) |
| 008 | [A particular client and a universally capable producer](008-a-particular-client-and-a-universally-capable-producer.txtar) | Evaluation |
| 009 | [A correlation-preserving overloaded contract](009-a-correlation-preserving-overloaded-contract.txtar) | Evaluation |
| 010 | [Capabilities and one-call constraints](010-capabilities-and-one-call-constraints.txtar) | Evaluation |
| 011 | [Universal instances and explicit relevant observations](011-universal-instances-and-explicit-relevant-observations.txtar) | Evaluation |
| 012 | [An empty value hypothesis](012-an-empty-value-hypothesis.txtar) | Evaluation |
| 013 | [A language operator with a polymorphic type](013-a-language-operator-with-a-polymorphic-type.txtar) | Evaluation |
| 014 | [Quantifier expressions](014-quantifier-expressions.txtar) | Syntax template (parsed) |
| 015 | [Quantified field declarations](015-quantified-field-declarations.txtar) | Evaluation |
| 016 | [Explicit quantifier elaboration](016-explicit-quantifier-elaboration.txtar) | Evaluation |
| 017 | [Quantified record fields](017-quantified-record-fields.txtar) | Evaluation |
| 018 | [Quantifier blocks](018-quantifier-blocks.txtar) | Evaluation |
| 019 | [Quantifier block elaboration](019-quantifier-block-elaboration.txtar) | Evaluation |
| 020 | [Field quantifier and existential block](020-field-quantifier-and-existential-block.txtar) | Evaluation |
| 021 | [Function-local generics and parametric aliases](021-function-local-generics-and-parametric-aliases.txtar) | Evaluation |
| 022 | [Parametric aliases](022-parametric-aliases.txtar) | Evaluation |
| 023 | [A conservative embedding](023-a-conservative-embedding.txtar) | Evaluation |
| 024 | [Full symmetric, higher-rank CUE: S_H](024-full-symmetric-higher-rank-cue-s-h.txtar) | Evaluation |
| 025 | [An explicitly impredicative instance](025-an-explicitly-impredicative-instance.txtar) | Evaluation |
| 026 | [Symmetric, rank-1 CUE: S_1](026-symmetric-rank-1-cue-s-1.txtar) | Evaluation |
| 027 | [Universal-only, rank-1 CUE: U_1](027-universal-only-rank-1-cue-u-1.txtar) | Evaluation |
| 028 | [Identity at unrelated instances](028-identity-at-unrelated-instances.txtar) | Evaluation |
| 029 | [Pairing preserves an inferred common refinement](029-pairing-preserves-an-inferred-common-refinement.txtar) | Evaluation |
| 030 | [Selecting a value preserves its subtype](030-selecting-a-value-preserves-its-subtype.txtar) | Evaluation |
| 031 | [Two type parameters preserve asymmetric information](031-two-type-parameters-preserve-asymmetric-information.txtar) | Evaluation |
| 032 | [Map with a single implementation](032-map-with-a-single-implementation.txtar) | Evaluation |
| 033 | [Filtering and partitioning preserve elements](033-filtering-and-partitioning-preserve-elements.txtar) | Evaluation |
| 034 | [Flattening a generic transformation](034-flattening-a-generic-transformation.txtar) | Evaluation |
| 035 | [Mapping named record coordinates](035-mapping-named-record-coordinates.txtar) | Evaluation |
| 036 | [Composition is rank-1](036-composition-is-rank-1.txtar) | Evaluation |
| 037 | [A value-dependent closure](037-a-value-dependent-closure.txtar) | Evaluation |
| 038 | [A polymorphic callback](038-a-polymorphic-callback.txtar) | Evaluation |
| 039 | [A nested universal result](039-a-nested-universal-result.txtar) | Evaluation |
| 040 | [A reusable polymorphic utility record](040-a-reusable-polymorphic-utility-record.txtar) | Evaluation |
| 041 | [A cross-file behavioral strengthening](041-a-cross-file-behavioral-strengthening.txtar) | Evaluation |
| 042 | [An intersection of domain-specific guarantees](042-an-intersection-of-domain-specific-guarantees.txtar) | Evaluation |
| 043 | [Unresolved alternatives and defaults](043-unresolved-alternatives-and-defaults.txtar) | Evaluation |
| 044 | [Positional, named, and mixed calls](044-positional-named-and-mixed-calls.txtar) | Evaluation |
| 045 | [Name-only required and optional parameters](045-name-only-required-and-optional-parameters.txtar) | Evaluation |
| 046 | [Positional-only aliases and anonymous parameters](046-positional-only-aliases-and-anonymous-parameters.txtar) | Evaluation and expected formation error |
| 047 | [Hidden labels and attributes](047-hidden-labels-and-attributes.txtar) | Evaluation |
| 048 | [Errors distinguish presence from values](048-errors-distinguish-presence-from-values.txtar) | Evaluation |
| 049 | [Omission defaults and caller preferences](049-omission-defaults-and-caller-preferences.txtar) | Evaluation |
| 050 | [A defaulted name-only argument](050-a-defaulted-name-only-argument.txtar) | Evaluation |
| 051 | [A configurable default is an explicit adapter](051-a-configurable-default-is-an-explicit-adapter.txtar) | Evaluation |
| 052 | [An open interface completed by an implementation](052-an-open-interface-completed-by-an-implementation.txtar) | Evaluation |
| 053 | [Chained partial application](053-chained-partial-application.txtar) | Evaluation |
| 054 | [A residual contract on a partial closure](054-a-residual-contract-on-a-partial-closure.txtar) | Evaluation |
| 055 | [Self-unification after copying and embedding](055-self-unification-after-copying-and-embedding.txtar) | Evaluation |
| 056 | [Equal source, unequal captures](056-equal-source-unequal-captures.txtar) | Evaluation |
| 057 | [Combining result descriptions explicitly](057-combining-result-descriptions-explicitly.txtar) | Evaluation |
| 058 | [A predicate used as an ordinary constraint](058-a-predicate-used-as-an-ordinary-constraint.txtar) | Evaluation |
| 059 | [Portable foreign contract](059-portable-foreign-contract.txtar) | Evaluation |
| 060 | [Nested calls remain ordinary evaluation](060-nested-calls-remain-ordinary-evaluation.txtar) | Evaluation |
| 061 | [A structurally recursive fold](061-a-structurally-recursive-fold.txtar) | Evaluation |
| 062 | [A directly dependent result](062-a-directly-dependent-result.txtar) | D (out of scope) |
| 063 | [A dependent parameter constraint](063-a-dependent-parameter-constraint.txtar) | D (out of scope) |
| 064 | [Indexed finite sequences](064-indexed-finite-sequences.txtar) | D (out of scope) |
| 065 | [Length-preserving map](065-length-preserving-map.txtar) | D (out of scope) |
| 066 | [Safe indexing, including the zero-length case](066-safe-indexing-including-the-zero-length-case.txtar) | D (out of scope) |
| 067 | [Head with an explicit nonempty precondition](067-head-with-an-explicit-nonempty-precondition.txtar) | D (out of scope) |
| 068 | [Appending adds lengths](068-appending-adds-lengths.txtar) | D (out of scope) |
| 069 | [Zip rejects unequal lengths](069-zip-rejects-unequal-lengths.txtar) | D (out of scope) |
| 070 | [Split at a bounded position](070-split-at-a-bounded-position.txtar) | D (out of scope) |
| 071 | [Replication reifies its count](071-replication-reifies-its-count.txtar) | D (out of scope) |
| 072 | [A dependent result with a hidden length](072-a-dependent-result-with-a-hidden-length.txtar) | D (out of scope) |
| 073 | [Matrix dimensions, including empty matrices](073-matrix-dimensions-including-empty-matrices.txtar) | D (out of scope) |
| 074 | [Transpose with a precise shape](074-transpose-with-a-precise-shape.txtar) | D (out of scope) |
| 075 | [Dot product with matched dimensions](075-dot-product-with-matched-dimensions.txtar) | D (out of scope) |
| 076 | [Dimension-safe matrix multiplication](076-dimension-safe-matrix-multiplication.txtar) | D (out of scope) |
| 077 | [A request-indexed response](077-a-request-indexed-response.txtar) | D (out of scope) |
| 078 | [A relation between input and output records](078-a-relation-between-input-and-output-records.txtar) | D (out of scope) |
| 079 | [Clamping to an argument-dependent interval](079-clamping-to-an-argument-dependent-interval.txtar) | D (out of scope) |
| 080 | [A shared hidden representation](080-a-shared-hidden-representation.txtar) | Evaluation |
| 081 | [Sealing and opening](081-sealing-and-opening.txtar) | Syntax template (parsed) |
| 082 | [An integer representation](082-an-integer-representation.txtar) | Evaluation |
| 083 | [A record representation](083-a-record-representation.txtar) | Evaluation |
| 084 | [A client checked once](084-a-client-checked-once.txtar) | Evaluation |
| 085 | [A module law strengthened by universals](085-a-module-law-strengthened-by-universals.txtar) | D (out of scope) |
| 086 | [Heterogeneous values with their own operations](086-heterogeneous-values-with-their-own-operations.txtar) | Evaluation |
| 087 | [Eliminating a first-class package](087-eliminating-a-first-class-package.txtar) | Evaluation |
| 088 | [Sharing two values under one witness](088-sharing-two-values-under-one-witness.txtar) | Evaluation |
| 089 | [An abstract codec interface](089-an-abstract-codec-interface.txtar) | Evaluation |
| 090 | [Mixed prefixes and generic modules](090-mixed-prefixes-and-generic-modules.txtar) | Evaluation |
| 091 | [Closed static field inventory](091-closed-static-field-inventory.txtar) | Evaluation |
| 092 | [Annotations assert properties of the body](092-annotations-assert-properties-of-the-body.txtar) | Evaluation |
| 093 | [Cheap contradictions and unresolved arithmetic](093-cheap-contradictions-and-unresolved-arithmetic.txtar) | Evaluation |
| 094 | [Refuted explicit observations](094-refuted-explicit-observations.txtar) | Evaluation |
| 095 | [Nonconflicting overlap and disjoint cases](095-nonconflicting-overlap-and-disjoint-cases.txtar) | Evaluation |
| 096 | [A generic computation and an explicit specialization](096-a-generic-computation-and-an-explicit-specialization.txtar) | Evaluation |
| 097 | [A universal obligation at an explicit sibling domain](097-a-universal-obligation-at-an-explicit-sibling-domain.txtar) | Evaluation |
| 098 | [Separate alternatives and nested callback contracts](098-separate-alternatives-and-nested-callback-contracts.txtar) | Evaluation |
| 099 | [An annotation and an explicit assertion](099-an-annotation-and-an-explicit-assertion.txtar) | Evaluation |
| 100 | [Explicit kind refinement](100-explicit-kind-refinement.txtar) | Evaluation |
| 101 | [Five presentations of one implementation constraint](101-five-presentations-of-one-implementation-constraint.txtar) | Evaluation |
| 102 | [Five presentations of one implementation constraint](102-five-presentations-of-one-implementation-constraint.txtar) | Evaluation |
| 103 | [Five presentations of one implementation constraint](103-five-presentations-of-one-implementation-constraint.txtar) | Evaluation |
| 104 | [Five presentations of one implementation constraint](104-five-presentations-of-one-implementation-constraint.txtar) | Evaluation |
| 105 | [Five presentations of one implementation constraint](105-five-presentations-of-one-implementation-constraint.txtar) | Evaluation |
| 106 | [Invalid interfaces and scoped computation failure](106-invalid-interfaces-and-scoped-computation-failure.txtar) | Evaluation |
| 107 | [Higher-order obligations are never runtime residuals](107-higher-order-obligations-are-never-runtime-residuals.txtar) | Evaluation |
| 108 | [A bounded implementation](108-a-bounded-implementation.txtar) | Implementation pseudocode |
| 109 | [Supplying a label](109-supplying-a-label.txtar) | Evaluation |
| 110 | [Restricting public calls across files](110-restricting-public-calls-across-files.txtar) | Evaluation |
| 111 | [A first-order dependent result check](111-a-first-order-dependent-result-check.txtar) | Evaluation |
