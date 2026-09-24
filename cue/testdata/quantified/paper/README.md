# Paper examples

All 101 listings from [the paper](../../../../doc/paper.tex), in source
order. Each archive preserves the exact listing in `paper.cue.txt`; executable
`in.cue` includes any required surrounding declarations and explicit assertions.
`TestQuantifiedPaperIndex` detects missing or changed listings.

Keep `paper.cue.txt` byte-for-byte identical to the listing, including its
shorthand declarations, comments, and whitespace. All archives use `#noformat`
to protect the original source during bulk fixture formatting. Supporting
declarations, assertions, and executable adaptations belong in `in.cue`.
Additional examples may be added in [the examples directory](../examples/).

The 78 executable examples run in `TestEvalV3/quantified/paper`. Two syntax
templates are parsed by the index test. Twenty D examples and one pseudocode
listing are retained with explicit exclusion reasons. Expected contradictions
and residual obligations are passing assertions, not skipped examples.

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
| 011 | [A scheme and an ordinary arrow unify normally](011-a-scheme-and-an-ordinary-arrow-unify-normally.txtar) | Evaluation |
| 012 | [Quantifier expressions](012-quantifier-expressions.txtar) | Syntax template (parsed) |
| 013 | [Quantified field declarations](013-quantified-field-declarations.txtar) | Evaluation |
| 014 | [Explicit quantifier elaboration](014-explicit-quantifier-elaboration.txtar) | Evaluation |
| 015 | [Quantified record fields](015-quantified-record-fields.txtar) | Evaluation |
| 016 | [Quantifier blocks](016-quantifier-blocks.txtar) | Evaluation |
| 017 | [Quantifier block elaboration](017-quantifier-block-elaboration.txtar) | Evaluation |
| 018 | [Field quantifier and existential block](018-field-quantifier-and-existential-block.txtar) | Evaluation |
| 019 | [Function-local generics and parametric aliases](019-function-local-generics-and-parametric-aliases.txtar) | Evaluation |
| 020 | [Parametric aliases](020-parametric-aliases.txtar) | Evaluation |
| 021 | [A conservative embedding](021-a-conservative-embedding.txtar) | Evaluation |
| 022 | [Full symmetric, higher-rank CUE: S_H](022-full-symmetric-higher-rank-cue-s-h.txtar) | Evaluation |
| 023 | [Symmetric, rank-1 CUE: S_1](023-symmetric-rank-1-cue-s-1.txtar) | Evaluation |
| 024 | [Universal-only, rank-1 CUE: U_1](024-universal-only-rank-1-cue-u-1.txtar) | Evaluation |
| 025 | [Identity at unrelated instances](025-identity-at-unrelated-instances.txtar) | Evaluation |
| 026 | [Pairing preserves an inferred common refinement](026-pairing-preserves-an-inferred-common-refinement.txtar) | Evaluation |
| 027 | [Selecting a value preserves its subtype](027-selecting-a-value-preserves-its-subtype.txtar) | Evaluation |
| 028 | [Two type parameters preserve asymmetric information](028-two-type-parameters-preserve-asymmetric-information.txtar) | Evaluation |
| 029 | [Map with a single implementation](029-map-with-a-single-implementation.txtar) | Evaluation |
| 030 | [Filtering and partitioning preserve elements](030-filtering-and-partitioning-preserve-elements.txtar) | Evaluation |
| 031 | [Flattening a generic transformation](031-flattening-a-generic-transformation.txtar) | Evaluation |
| 032 | [Mapping named record coordinates](032-mapping-named-record-coordinates.txtar) | Evaluation |
| 033 | [Composition is rank-1](033-composition-is-rank-1.txtar) | Evaluation |
| 034 | [A value-dependent closure](034-a-value-dependent-closure.txtar) | Evaluation |
| 035 | [A polymorphic callback](035-a-polymorphic-callback.txtar) | Evaluation |
| 036 | [A nested universal result](036-a-nested-universal-result.txtar) | Evaluation |
| 037 | [A reusable polymorphic utility record](037-a-reusable-polymorphic-utility-record.txtar) | Evaluation |
| 038 | [A cross-file behavioral strengthening](038-a-cross-file-behavioral-strengthening.txtar) | Evaluation |
| 039 | [An intersection of domain-specific guarantees](039-an-intersection-of-domain-specific-guarantees.txtar) | Evaluation |
| 040 | [Unresolved alternatives and defaults](040-unresolved-alternatives-and-defaults.txtar) | Evaluation |
| 041 | [Positional, named, and mixed calls](041-positional-named-and-mixed-calls.txtar) | Evaluation |
| 042 | [Name-only required and optional parameters](042-name-only-required-and-optional-parameters.txtar) | Evaluation |
| 043 | [Positional-only aliases and anonymous parameters](043-positional-only-aliases-and-anonymous-parameters.txtar) | Evaluation and expected formation error |
| 044 | [Hidden labels and attributes](044-hidden-labels-and-attributes.txtar) | Evaluation |
| 045 | [Errors distinguish presence from values](045-errors-distinguish-presence-from-values.txtar) | Evaluation |
| 046 | [Omission defaults and caller preferences](046-omission-defaults-and-caller-preferences.txtar) | Evaluation |
| 047 | [A defaulted name-only argument](047-a-defaulted-name-only-argument.txtar) | Evaluation |
| 048 | [A configurable default is an explicit adapter](048-a-configurable-default-is-an-explicit-adapter.txtar) | Evaluation |
| 049 | [An open interface completed by an implementation](049-an-open-interface-completed-by-an-implementation.txtar) | Evaluation |
| 050 | [Chained partial application](050-chained-partial-application.txtar) | Evaluation |
| 051 | [A residual contract on a partial closure](051-a-residual-contract-on-a-partial-closure.txtar) | Evaluation |
| 052 | [Self-unification after copying and embedding](052-self-unification-after-copying-and-embedding.txtar) | Evaluation |
| 053 | [Equal source, unequal captures](053-equal-source-unequal-captures.txtar) | Evaluation |
| 054 | [Combining result descriptions explicitly](054-combining-result-descriptions-explicitly.txtar) | Evaluation |
| 055 | [A predicate used as an ordinary constraint](055-a-predicate-used-as-an-ordinary-constraint.txtar) | Evaluation |
| 056 | [Portable foreign contract](056-portable-foreign-contract.txtar) | Evaluation |
| 057 | [Nested calls remain ordinary evaluation](057-nested-calls-remain-ordinary-evaluation.txtar) | Evaluation |
| 058 | [A structurally recursive fold](058-a-structurally-recursive-fold.txtar) | Evaluation |
| 059 | [A directly dependent result](059-a-directly-dependent-result.txtar) | D (out of scope) |
| 060 | [A dependent parameter constraint](060-a-dependent-parameter-constraint.txtar) | D (out of scope) |
| 061 | [Indexed finite sequences](061-indexed-finite-sequences.txtar) | D (out of scope) |
| 062 | [Length-preserving map](062-length-preserving-map.txtar) | D (out of scope) |
| 063 | [Safe indexing, including the zero-length case](063-safe-indexing-including-the-zero-length-case.txtar) | D (out of scope) |
| 064 | [Head with an explicit nonempty precondition](064-head-with-an-explicit-nonempty-precondition.txtar) | D (out of scope) |
| 065 | [Appending adds lengths](065-appending-adds-lengths.txtar) | D (out of scope) |
| 066 | [Zip rejects unequal lengths](066-zip-rejects-unequal-lengths.txtar) | D (out of scope) |
| 067 | [Split at a bounded position](067-split-at-a-bounded-position.txtar) | D (out of scope) |
| 068 | [Replication reifies its count](068-replication-reifies-its-count.txtar) | D (out of scope) |
| 069 | [A dependent result with a hidden length](069-a-dependent-result-with-a-hidden-length.txtar) | D (out of scope) |
| 070 | [Matrix dimensions, including empty matrices](070-matrix-dimensions-including-empty-matrices.txtar) | D (out of scope) |
| 071 | [Transpose with a precise shape](071-transpose-with-a-precise-shape.txtar) | D (out of scope) |
| 072 | [Dot product with matched dimensions](072-dot-product-with-matched-dimensions.txtar) | D (out of scope) |
| 073 | [Dimension-safe matrix multiplication](073-dimension-safe-matrix-multiplication.txtar) | D (out of scope) |
| 074 | [A request-indexed response](074-a-request-indexed-response.txtar) | D (out of scope) |
| 075 | [A relation between input and output records](075-a-relation-between-input-and-output-records.txtar) | D (out of scope) |
| 076 | [Clamping to an argument-dependent interval](076-clamping-to-an-argument-dependent-interval.txtar) | D (out of scope) |
| 077 | [A shared hidden representation](077-a-shared-hidden-representation.txtar) | Evaluation |
| 078 | [Sealing and opening](078-sealing-and-opening.txtar) | Syntax template (parsed) |
| 079 | [An integer representation](079-an-integer-representation.txtar) | Evaluation |
| 080 | [A record representation](080-a-record-representation.txtar) | Evaluation |
| 081 | [A client checked once](081-a-client-checked-once.txtar) | Evaluation |
| 082 | [A module law strengthened by universals](082-a-module-law-strengthened-by-universals.txtar) | D (out of scope) |
| 083 | [Heterogeneous values with their own operations](083-heterogeneous-values-with-their-own-operations.txtar) | Evaluation |
| 084 | [Eliminating a first-class package](084-eliminating-a-first-class-package.txtar) | Evaluation |
| 085 | [Sharing two values under one witness](085-sharing-two-values-under-one-witness.txtar) | Evaluation |
| 086 | [An abstract codec interface](086-an-abstract-codec-interface.txtar) | Evaluation |
| 087 | [Mixed prefixes and generic modules](087-mixed-prefixes-and-generic-modules.txtar) | Evaluation |
| 088 | [An empty arithmetic relation retained as hypotheses](088-an-empty-arithmetic-relation-retained-as-hypotheses.txtar) | Evaluation |
| 089 | [A local bound contradiction](089-a-local-bound-contradiction.txtar) | Evaluation |
| 090 | [A concrete result with a residual guard](090-a-concrete-result-with-a-residual-guard.txtar) | Evaluation |
| 091 | [A broad approximation is not a contradiction](091-a-broad-approximation-is-not-a-contradiction.txtar) | Evaluation |
| 092 | [Concrete counterexamples to implementation contracts](092-concrete-counterexamples-to-implementation-contracts.txtar) | Evaluation |
| 093 | [Uniform proofs and finite refutations](093-uniform-proofs-and-finite-refutations.txtar) | Evaluation |
| 094 | [A symbolic inconsistency exposed by one universal instance](094-a-symbolic-inconsistency-exposed-by-one-universal-instance.txtar) | Evaluation |
| 095 | [A flexible witness must remain refinable](095-a-flexible-witness-must-remain-refinable.txtar) | Evaluation |
| 096 | [A refinable argument restricts a call, not a universal domain](096-a-refinable-argument-restricts-a-call-not-a-universal-domain.txtar) | Evaluation |
| 097 | [Local execution and universal verification](097-local-execution-and-universal-verification.txtar) | Evaluation |
| 098 | [Incremental algorithm](098-incremental-algorithm.txtar) | Implementation pseudocode |
| 099 | [Supplying a label](099-supplying-a-label.txtar) | Evaluation |
| 100 | [Restricting public calls across files](100-restricting-public-calls-across-files.txtar) | Evaluation |
| 101 | [Keeping late contract failures explicit](101-keeping-late-contract-failures-explicit.txtar) | Evaluation |
