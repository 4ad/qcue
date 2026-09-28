# Paper examples

The [explainer](../../../../doc/explainer.md) develops these examples for
experienced CUE users; each section link connects a listing to its explanation.

Every listing from version 9 is preserved verbatim. Executable sections carry
assertions, with prior definitions and linked implementations where needed.
Later refinement and source round trips are also tested by
[`propagation_test.go`](../../../propagation_test.go).

The companion [type reference](../../../../doc/types.md) lists quantified
builtin contracts, operator signatures, and standard-library function types.
The written contracts are checked by
[reference tests](../../../../internal/cmd/gentypes/main_test.go), separately
from the verbatim paper listings below.

| Listing | Example | Explainer section |
| --- | --- | --- |
| 1 | [States and transitions](001-states-and-transitions.txtar) | [Explanation](../../../../doc/explainer.md#direct-checks-and-suspended-work) |
| 2 | [Universal declarations and instances](002-universal-declarations-and-instances.txtar) | [Explanation](../../../../doc/explainer.md#binder-spellings-and-scope) |
| 3 | [Universal declarations and instances](003-universal-declarations-and-instances.txtar) | [Explanation](../../../../doc/explainer.md#binder-spellings-and-scope) |
| 4 | [Completion witnesses and universal subjects](004-completion-witnesses-and-universal-subjects.txtar) | [Explanation](../../../../doc/explainer.md#descriptions-and-their-completions) |
| 5 | [One identity at many instances](005-one-identity-at-many-instances.txtar) | [Explanation](../../../../doc/explainer.md#reading-a-universal-function) |
| 6 | [Common predicates and preserved refinements](006-common-predicates-and-preserved-refinements.txtar) | [Explanation](../../../../doc/explainer.md#predicting-what-a-type-parameter-preserves) |
| 7 | [Bounded selection](007-bounded-selection.txtar) | [Explanation](../../../../doc/explainer.md#predicting-what-a-type-parameter-preserves) |
| 8 | [Distinct universal ranges](008-distinct-universal-ranges.txtar) | [Explanation](../../../../doc/explainer.md#predicting-what-a-type-parameter-preserves) |
| 9 | [A polymorphic map](009-a-polymorphic-map.txtar) | [Explanation](../../../../doc/explainer.md#passing-and-returning-functions) |
| 10 | [Filtering and nested traversal](010-filtering-and-nested-traversal.txtar) | [Explanation](../../../../doc/explainer.md#passing-and-returning-functions) |
| 11 | [Mapping a record](011-mapping-a-record.txtar) | [Explanation](../../../../doc/explainer.md#passing-and-returning-functions) |
| 12 | [Composition and captured data](012-composition-and-captured-data.txtar) | [Explanation](../../../../doc/explainer.md#composition-and-captures) |
| 13 | [A universally quantified parameter](013-a-universally-quantified-parameter.txtar) | [Explanation](../../../../doc/explainer.md#a-polymorphic-callback) |
| 14 | [Quantified results and nested rank](014-quantified-results-and-nested-rank.txtar) | [Explanation](../../../../doc/explainer.md#a-polymorphic-callback) |
| 15 | [An impredicative instance](015-an-impredicative-instance.txtar) | [Explanation](../../../../doc/explainer.md#universal-types-as-type-arguments) |
| 16 | [A live input and a fixed alias](016-a-live-input-and-a-fixed-alias.txtar) | [Explanation](../../../../doc/explainer.md#a-live-description-remains-connected-to-its-calls) |
| 17 | [Input and result constraints on one call](017-input-and-result-constraints-on-one-call.txtar) | [Explanation](../../../../doc/explainer.md#a-live-description-remains-connected-to-its-calls) |
| 18 | [A stronger bound completes a proof](018-a-stronger-bound-completes-a-proof.txtar) | [Explanation](../../../../doc/explainer.md#the-two-directions-of-evidence) |
| 19 | [Proving a live result](019-proving-a-live-result.txtar) | [Explanation](../../../../doc/explainer.md#the-two-directions-of-evidence) |
| 20 | [Evaluation produces certification premises](020-evaluation-produces-certification-premises.txtar) | [Explanation](../../../../doc/explainer.md#checking-and-evaluation-make-progress-together) |
| 21 | [A partially known capture](021-a-partially-known-capture.txtar) | [Explanation](../../../../doc/explainer.md#records-captured-data-and-function-identity) |
| 22 | [An unfounded proof dependency](022-an-unfounded-proof-dependency.txtar) | [Explanation](../../../../doc/explainer.md#proof-dependencies-must-be-grounded) |
| 23 | [A live observation guard](023-a-live-observation-guard.txtar) | [Explanation](../../../../doc/explainer.md#checking-explicitly-impossible-interfaces) |
| 24 | [Coverage requires the opposite inclusion](024-coverage-requires-the-opposite-inclusion.txtar) | [Explanation](../../../../doc/explainer.md#the-two-directions-of-evidence) |
| 25 | [A live universal bound](025-a-live-universal-bound.txtar) | [Explanation](../../../../doc/explainer.md#the-two-directions-of-evidence) |
| 26 | [Residual validation and strict arguments](026-residual-validation-and-strict-arguments.txtar) | [Explanation](../../../../doc/explainer.md#computed-results-retain-their-constraints) |
| 27 | [Scoped instances and linking hypotheses](027-scoped-instances-and-linking-hypotheses.txtar) | [Explanation](../../../../doc/explainer.md#universal-types-as-type-arguments) |
| 28 | [A shared quantified record](028-a-shared-quantified-record.txtar) | [Explanation](../../../../doc/explainer.md#shared-records-and-declarations-across-files) |
| 29 | [Contravariant attachment](029-contravariant-attachment.txtar) | [Explanation](../../../../doc/explainer.md#contravariance-checks-a-capability) |
| 30 | [Regions of a conjunctive interface](030-regions-of-a-conjunctive-interface.txtar) | [Explanation](../../../../doc/explainer.md#combining-interfaces-on-one-implementation) |
| 31 | [Explicitly contradictory observations](031-explicitly-contradictory-observations.txtar) | [Explanation](../../../../doc/explainer.md#checking-explicitly-impossible-interfaces) |
| 32 | [An independent anchor for a universal](032-an-independent-anchor-for-a-universal.txtar) | [Explanation](../../../../doc/explainer.md#universal-observations-and-concrete-instances) |
| 33 | [Generic meet and explicit specialization](033-generic-meet-and-explicit-specialization.txtar) | [Explanation](../../../../doc/explainer.md#universal-observations-and-concrete-instances) |
| 34 | [Annotations and executable assertions](034-annotations-and-executable-assertions.txtar) | [Explanation](../../../../doc/explainer.md#a-result-contract-is-a-proof-obligation) |
| 35 | [Equivalent arrangements of constraints](035-equivalent-arrangements-of-constraints.txtar) | [Explanation](../../../../doc/explainer.md#shared-records-and-declarations-across-files) |
| 36 | [Field evidence and presence](036-field-evidence-and-presence.txtar) | [Explanation](../../../../doc/explainer.md#records-captured-data-and-function-identity) |
| 37 | [Failure inherited by composition](037-failure-inherited-by-composition.txtar) | [Explanation](../../../../doc/explainer.md#composition-and-captures) |
| 38 | [Slot binding and omission](038-slot-binding-and-omission.txtar) | [Explanation](../../../../doc/explainer.md#calling-conventions-are-part-of-the-type) |
| 39 | [Package-qualified labels](039-package-qualified-labels.txtar) | [Explanation](../../../../doc/explainer.md#calling-conventions-are-part-of-the-type) |
| 40 | [Defaults belong to the protocol](040-defaults-belong-to-the-protocol.txtar) | [Explanation](../../../../doc/explainer.md#omission-and-defaults) |
| 41 | [Adapters are implementations](041-adapters-are-implementations.txtar) | [Explanation](../../../../doc/explainer.md#omission-and-defaults) |
| 42 | [Open rows and normalized partial bindings](042-open-rows-and-normalized-partial-bindings.txtar) | [Explanation](../../../../doc/explainer.md#open-parameter-rows-and-partial-calls) |
| 43 | [Closure identity through copying](043-closure-identity-through-copying.txtar) | [Explanation](../../../../doc/explainer.md#what-identifies-a-function-value) |
| 44 | [Shared results and implementation alternatives](044-shared-results-and-implementation-alternatives.txtar) | [Explanation](../../../../doc/explainer.md#what-identifies-a-function-value) |
| 45 | [Validation across field and call boundaries](045-validation-across-field-and-call-boundaries.txtar) | [Explanation](../../../../doc/explainer.md#validation-follows-the-selected-value) |
| 46 | [Conditional certificates and native checks](046-conditional-certificates-and-native-checks.txtar) | [Explanation](../../../../doc/explainer.md#linking-native-implementations) |
| 47 | [Finite iteration and repeated invocation](047-finite-iteration-and-repeated-invocation.txtar) | [Explanation](../../../../doc/explainer.md#a-finite-fold) |
| 48 | [Direct contradictions and residual equations](048-direct-contradictions-and-residual-equations.txtar) | [Explanation](../../../../doc/explainer.md#direct-checks-and-suspended-work) |
| 49 | [Native folds and operand evidence](049-native-folds-and-operand-evidence.txtar) | [Explanation](../../../../doc/explainer.md#structural-operations-preserve-evidence) |
