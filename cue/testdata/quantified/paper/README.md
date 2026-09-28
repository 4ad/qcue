# Paper examples

Every listing from version 9 is preserved verbatim. Executable sections carry
assertions, with prior definitions and linked implementations where needed.
Later refinement and source round trips are also tested by
[`propagation_test.go`](../../../propagation_test.go).

The companion [type reference](../../../../doc/types.md) lists quantified
builtin contracts, operator signatures, and standard-library function types.
The written contracts are checked by
[reference tests](../../../../internal/cmd/gentypes/main_test.go), separately
from the verbatim paper listings below.

| Listing | Example |
| --- | --- |
| 1 | [States and transitions](001-states-and-transitions.txtar) |
| 2 | [Universal declarations and instances](002-universal-declarations-and-instances.txtar) |
| 3 | [Universal declarations and instances](003-universal-declarations-and-instances.txtar) |
| 4 | [Completion witnesses and universal subjects](004-completion-witnesses-and-universal-subjects.txtar) |
| 5 | [One identity at many instances](005-one-identity-at-many-instances.txtar) |
| 6 | [Common predicates and preserved refinements](006-common-predicates-and-preserved-refinements.txtar) |
| 7 | [Bounded selection](007-bounded-selection.txtar) |
| 8 | [Distinct universal ranges](008-distinct-universal-ranges.txtar) |
| 9 | [A polymorphic map](009-a-polymorphic-map.txtar) |
| 10 | [Filtering and nested traversal](010-filtering-and-nested-traversal.txtar) |
| 11 | [Mapping a record](011-mapping-a-record.txtar) |
| 12 | [Composition and captured data](012-composition-and-captured-data.txtar) |
| 13 | [A universally quantified parameter](013-a-universally-quantified-parameter.txtar) |
| 14 | [Quantified results and nested rank](014-quantified-results-and-nested-rank.txtar) |
| 15 | [An impredicative instance](015-an-impredicative-instance.txtar) |
| 16 | [A live input and a fixed alias](016-a-live-input-and-a-fixed-alias.txtar) |
| 17 | [Input and result constraints on one call](017-input-and-result-constraints-on-one-call.txtar) |
| 18 | [A stronger bound completes a proof](018-a-stronger-bound-completes-a-proof.txtar) |
| 19 | [Proving a live result](019-proving-a-live-result.txtar) |
| 20 | [Evaluation produces certification premises](020-evaluation-produces-certification-premises.txtar) |
| 21 | [A partially known capture](021-a-partially-known-capture.txtar) |
| 22 | [An unfounded proof dependency](022-an-unfounded-proof-dependency.txtar) |
| 23 | [A live observation guard](023-a-live-observation-guard.txtar) |
| 24 | [Coverage requires the opposite inclusion](024-coverage-requires-the-opposite-inclusion.txtar) |
| 25 | [A live universal bound](025-a-live-universal-bound.txtar) |
| 26 | [Residual validation and strict arguments](026-residual-validation-and-strict-arguments.txtar) |
| 27 | [Scoped instances and linking hypotheses](027-scoped-instances-and-linking-hypotheses.txtar) |
| 28 | [A shared quantified record](028-a-shared-quantified-record.txtar) |
| 29 | [Contravariant attachment](029-contravariant-attachment.txtar) |
| 30 | [Regions of a conjunctive interface](030-regions-of-a-conjunctive-interface.txtar) |
| 31 | [Explicitly contradictory observations](031-explicitly-contradictory-observations.txtar) |
| 32 | [An independent anchor for a universal](032-an-independent-anchor-for-a-universal.txtar) |
| 33 | [Generic meet and explicit specialization](033-generic-meet-and-explicit-specialization.txtar) |
| 34 | [Annotations and executable assertions](034-annotations-and-executable-assertions.txtar) |
| 35 | [Equivalent arrangements of constraints](035-equivalent-arrangements-of-constraints.txtar) |
| 36 | [Field evidence and presence](036-field-evidence-and-presence.txtar) |
| 37 | [Failure inherited by composition](037-failure-inherited-by-composition.txtar) |
| 38 | [Slot binding and omission](038-slot-binding-and-omission.txtar) |
| 39 | [Package-qualified labels](039-package-qualified-labels.txtar) |
| 40 | [Defaults belong to the protocol](040-defaults-belong-to-the-protocol.txtar) |
| 41 | [Adapters are implementations](041-adapters-are-implementations.txtar) |
| 42 | [Open rows and normalized partial bindings](042-open-rows-and-normalized-partial-bindings.txtar) |
| 43 | [Closure identity through copying](043-closure-identity-through-copying.txtar) |
| 44 | [Shared results and implementation alternatives](044-shared-results-and-implementation-alternatives.txtar) |
| 45 | [Validation across field and call boundaries](045-validation-across-field-and-call-boundaries.txtar) |
| 46 | [Conditional certificates and native checks](046-conditional-certificates-and-native-checks.txtar) |
| 47 | [Finite iteration and repeated invocation](047-finite-iteration-and-repeated-invocation.txtar) |
| 48 | [Direct contradictions and residual equations](048-direct-contradictions-and-residual-equations.txtar) |
| 49 | [Native folds and operand evidence](049-native-folds-and-operand-evidence.txtar) |
