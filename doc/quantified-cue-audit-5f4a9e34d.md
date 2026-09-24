**Quantified CUE implementation audit — 24 September 2026**

Audited revision: `5f4a9e34d03d7e5af65a2246a2fc4db55c2a3528`.
Specification: [quantified-cue.tex](quantified-cue.tex).
Declared implementation scope: [quantified-cue-implementation.md](quantified-cue-implementation.md).

**Verdict: substantial implementation, but not faithful enforcement of the proposal, even within the advertised fragments.** The implementation is much more than a parser or prototype: it implements lexical quantification, guarded capabilities, several higher-rank checks, closure identity, finite witnesses, and opaque packages. Nevertheless, current code can certify an impossible public operation, accept an invalid callback packet, and validate an existential subject after discarding conflicting constraints. Other reproduced problems expose private implementation identity, refute valid calls, or mishandle type selection.

This report describes seven current findings. It does not repeat the repaired examples in the earlier audit reports as if they were still broken. Findings 1–4 affect semantic guarantees; findings 5–7 concern incorrect rejection, lost selection metadata, or unnecessary incompleteness. P1 means a correctness defect to address before relying on the affected guarantee; P2 means a significant behavioral or coverage defect. No production code was changed by this audit.

**Method and test results**

I read the proposal's semantics, profiles, surface forms, examples, checking rules, residual-state obligations, and implementation contracts; compared these with the implementation guide; and traced parsing/binding, compilation, quantifier reduction, inference, universe checks, function contracts, certification, closure identity, sealing/opening, validation, and source export. Additional small programs were exercised through both `cue` and the public Go API. Controls included regular versus hidden fields, direct versus separately supplied implementations, reordered clauses, and inlined versus shared type descriptions.

The focused evaluator, API, parser, formatter, AST, exporter, and certification tests passed. The full `go test ./...` also passed: **121 packages reported `ok`, with zero failing packages**. The first sandboxed full run failed because a CLI test could not bind a loopback test-server port; the successful run allowed those sockets. A writable temporary Go build cache was used. Passing existing tests does not cover the counterexamples below.

Reproduction setup:

```sh
GOCACHE=/tmp/cue-audit-go-cache go build -o /tmp/cue-audit ./cmd/cue
```

Save each example in its own temporary directory outside the repository's pinned CUE module, with `@experiment(quantified)` at the beginning of the file. Then run:

```sh
/tmp/cue-audit vet -c case.cue
/tmp/cue-audit export -e out case.cue
```

Where the distinction between root and selected-value validation matters, the observations below use:

```go
v := cuecontext.New().CompileString(source)
rootErr := v.Validate(cue.Concrete(true))
out := v.LookupPath(cue.ParsePath("out"))
outErr := out.Validate(cue.Concrete(true))
data, exportErr := out.MarshalJSON()
```

Ordinary `Validate()` may accept residual obligations. That is intentional and is not treated as evidence of successful certification. An incomplete proof is also not a contradiction.

**1. [P1] Hidden callback fields bypass the actual packet's conformance checks**

Primary locations: [adt/validate.go](../internal/core/adt/validate.go), lines 228–239 and 256–261; call-site validation in [adt/expr.go](../internal/core/adt/expr.go), lines 2588–2605.

```cue
cb: func(x: int) -> int: {v: x}.v
f: func(arg: {_cb: func(int) -> 1}) -> int: 0
out: f({_cb: cb})
```

Actual: root concrete validation succeeds, `out` passes concrete validation, and `out` exports as `0`. Replacing `_cb` with an ordinary field name makes the call incomplete, as it should.

The callback's own `int -> int` contract is valid, but it does not satisfy the argument's required `int -> 1` contract: input `0` is a counterexample. Changing `f`'s body to `arg._cb(1)` still permits root concrete validation and exports `1`; one successful callback invocation does not establish the universal promise. The same bypass works with a builtin:

```cue
import "strings"
f: func(arg: {_cb: func(string) -> "yes"}) -> int: 0
out: f({_cb: strings.ToUpper})
```

This also passes concrete validation and exports `0`.

The call path detects the nested callable and requests concrete validation with `CheckFunction`/`CheckBuiltin`. However, the validator increments `inDefinition` for every nonregular field, including a supplied hidden runtime field. `checkConcrete()` then becomes false and both conformance hooks are skipped. The result is returned without the actual callback's unresolved membership obligation.

The proposal explicitly requires the caller to preserve validation of the actual callback and forbids using an annotation or a single successful invocation as its proof (specification lines 2487–2491). The implementation guide makes the same promise for completed calls and callbacks nested in composites.

Repair direction: give validation of supplied runtime packets a traversal policy that checks hidden runtime data and its callable obligations. Preserve the ordinary schema treatment of undemanded definitions and absent optional fields. Test hidden callbacks at multiple nesting depths, both ignored and invoked, and include builtins.

**2. [P1] Existential membership discards conflicting hidden fields and definitions**

Primary location: [adt/package.go](../internal/core/adt/package.go), lines 191–220. The projection being used is [adt/composite.go](../internal/core/adt/composite.go), lines 816–873.

```cue
v: (exists A {_x: 1}) & {_x: 2}
out: v._x
```

Actual: ordinary and concrete validation both succeed, including at the root. `out` exports as `2`. The same failure occurs with definitions:

```cue
v: (exists A {#X: 1}) & {#X: 2}
out: v.#X
```

This also validates and exports `2`. Nesting the conflicting hidden field inside an ordinary record reproduces the failure. Replacing `_x` with regular `x`, or replacing `exists` with `forall`, exposes the expected conflict.

There is no difficult witness search here. `A` is unused, and its type universe is nonempty, so the existential is equivalent to the constant record predicate. No witness can make one field equal both `1` and `2`.

`validateWitness` calls `vertex.ToDataAll(c)` to avoid recursively checking the same existential validator. That operation removes hidden fields and definitions. Membership is then checked against a different subject: the instantiated existential body supplies `_x: 1`, while the candidate's contradictory `_x: 2` is absent. `sameCapabilityShape` also ignores nonregular labels, so it does not detect the altered subject.

This violates existential membership, preservation of host field distinctions, and the concrete validation guarantee. It is independent of the documented limits on general existential synthesis or elimination.

Repair direction: remove the validator's self-reference without applying a JSON-style projection to its candidate. Retain all observable fields, their package-qualified labels, presence, and constraints. Extend the existing existential membership tests to hidden fields, definitions, nested occurrences, and later `Unify`/`FillPath` refinement.

**3. [P1] Opaque overload certification mistakes a positional/name-only overlap for disjoint domains**

Primary location: [adt/opaque_function.go](../internal/core/adt/opaque_function.go), lines 243–291, especially 257–263. The proof consumes this answer in [adt/opaque_proof.go](../internal/core/adt/opaque_proof.go), `ProofTypes`.

```cue
#M: exists A {
    f: (func(x!: int) -> int) & (func(x: int) -> A)
}
p: seal #M with (A = int) {
    f: func(x: int) -> int: x
}
```

Actual: root `Validate(cue.Concrete(true))`, validation of `p`, and `cue vet -c` all succeed.

The public interface is impossible. Both clauses admit the packet `(x: 1)`. Its one result would have to be both an ordinary integer and a member of the fresh opaque carrier `A`. Those public kinds are disjoint, even though both private representations are integers. Both arrows are pure, so a shared admitted effect cannot resolve the conflict.

Adding this demand does not produce the promised successful result:

```cue
out: (open p as (A, P) {result: P.f(x: 1)}).result
```

It reports `overlapping interface transports remain unresolved`. Reversing the two interface clauses prevents concrete certification of `p` in the first place. Thus certification is also sensitive to conjunction order.

`coherentPair` uses directional parameter alignment as a disjointness test. When a positional-and-labeled parameter is compared with a name-only parameter, positional matching fails. Lines 260–263 treat that as proof that the packet domains are disjoint. It is not: the matching labeled packet belongs to both. This incorrectly bypasses the required agreement of result transports.

Repair direction: reason about intersections of complete packet domains, including both positional and labeled alternatives. A failed parameter alignment cannot establish disjointness. Keep unknown overlap incomplete. Cover both clause orders, matching labels across parameter modes, defaults, partial applications, and genuinely disjoint label sets.

**4. [P1] Public operation identity exposes private sharing and changes when a type alias is inlined**

Locations: [adt/opaque_function.go](../internal/core/adt/opaque_function.go), lines 24–38; [adt/closure.go](../internal/core/adt/closure.go), lines 47–56.

```cue
#F: func(int) -> int
#M: exists A {f: #F, g: #F}
h: func(x: int) -> int: x
p: seal #M with (A = int) {f: h, g: h}
out: (open p as (A, P) {result: (P.f & P.g)(1)}).result
```

Actual: the program passes concrete validation and exports `1`. Now change only the private implementation to:

```cue
p: seal #M with (A = int) {
    f: func(x: int) -> int: x
    g: func(x: int) -> int: x
}
```

Both operations still implement the same ordinary behavior and independently pass their required contracts. But `out` now conflicts with `conflicting function identities`. The public client has detected whether the implementation shared one private closure or used two private code origins.

There is a second control: retain the shared private `h`, but inline the two occurrences of `#F`:

```cue
#M: exists A {f: func(int) -> int, g: func(int) -> int}
```

This also changes the successful observation into a conflict. Sharing a type predicate is being confused with sharing a public operation. The interface never declared `g: f`.

Adapter caching uses the signature node/environment and private closure identity; adapter equality can recurse into the private closure. Consequently, public equality depends on private sharing and on whether bodyless type syntax shares an AST origin.

The sealing discipline explicitly associates public handles with the interface's declared export/alias graph and makes private code origins and captures unobservable (specification lines 1866–1869). A common contract is not an operation-alias equation. This failure concerns that boundary requirement; it is not a claim that arbitrary behavior-changing module replacements satisfy the representation-simulation theorem.

Repair direction: derive public handle identities from stable interface export identities and explicit public aliases. Keep type-description sharing independent of operation sharing. Preserve copied handles and callback round-trip identity without consulting hidden implementation identity for equality of independent exports. Add tests that inline aliases and vary private sharing while keeping the declared public graph fixed.

**5. [P2] Capability guards can manufacture a missing hidden field and refute a valid call**

Primary location: [adt/capability.go](../internal/core/adt/capability.go), lines 172–190, especially the exclusion of nonregular labels at line 181. This is related to finding 2, but uses a different membership path and requires an additional fix.

```cue
f: (func(x: {}) -> int: {
    if x._required == _|_ {v: 0}
    if x._required != _|_ {v: 1}
}.v) & (func({_required: 1}) -> 1)

out: f({})
```

Expected: `0`. The implementation returns `1` when the hidden field exists and `0` when it is absent. It satisfies the attached guarantee on every packet in that guarantee's domain. The empty record is outside that domain.

Actual: `out` is a hard `conflicting values 0 and 1` error. Removing the attached contract gives `0`; supplying `_required: 1` gives `1`.

Guard membership first unifies the supplied argument with the guard. This can create the missing `_required` field. The shape check is intended to distinguish that enrichment from membership of the original packet, but it skips hidden fields and definitions. Validation therefore reports the guard established and incorrectly applies its result `1` to the empty packet.

The current certifier leaves the presence-branch implementation itself unproved; that is an allowed checking limitation. Turning its valid concrete call into a contradiction is a separate semantic error. The required correction is to preserve the supplied packet's complete observable shape when deciding applicability, as already done for regular fields. Include hidden fields and definitions in the regression matrix.

**6. [P2] Scalar universal reduction loses type-selection and universe metadata**

Primary location: [adt/quantified.go](../internal/core/adt/quantified.go), lines 155–170. Related formation logic: [adt/universe.go](../internal/core/adt/universe.go), lines 61–105.

```cue
c(A): 1
out: c[int]
```

Expected: `1`, selecting an admissible instance of the same universally constrained subject. Actual: a hard error, `invalid operand c (found int, want list or struct)`.

Using explicit RHS syntax, `c: forall (A in Type(0)) 1`, gives the same result. Wrapping the constant in a record makes selection work:

```cue
c(A): {n: 1}
out: c[int].n // 1
```

The reduction evaluates the universal body and records the introduction only when the result is a record or list vertex. A scalar loses its binder metadata, so later indexing is interpreted as ordinary data indexing.

This also makes enforcement of the documented universe-level rule depend on result representation:

```cue
poly: forall (A in Type(0)) 1
f(T in Type(0)): func(x: T) -> T: x
out: f[poly](1) // accepted and exports 1
```

The corresponding quantified record argument is rejected with `universe level 1, exceeding Type(0)`. The implementation guide says a quantifier over `Type(n)` lives above level `n`, and the existing universe regressions retain even unused binders on composite and callable subjects. Scalar reduction bypasses that retained-telescope rule.

Repair direction: retain universal introduction/selection metadata independently of the subject's runtime kind. Apply the same formation policy before or after normalization. Test constants, scalar unions, unused binders, explicit universe annotations, and export/reimport as well as records and functions.

**7. [P2] Selecting a quantified record can restart an already consumed binder on its method**

Locations: [adt/subject.go](../internal/core/adt/subject.go), lines 94–109; [adt/quantified.go](../internal/core/adt/quantified.go), lines 563–567 and 586–618.

```cue
r(A): {f(B): func(A) -> A}
r: {f: func(x: _) -> _: x}
out: r[int].f[string](1)
```

The identity implementation satisfies the complete generic contract, and concrete validation of `r` succeeds. `r[int]` selects the outer `A`; `.f[string]` should then select the inner `B`, which is unused here. The result should be `1`.

Actual: the demanded result remains incomplete with `type argument inference remains unresolved: ... inferred instance does not prove argument membership`. Both declaration orders reproduce it. A directly implemented spelling succeeds:

```cue
r(A): {f(B): func(x: A) -> A: x}
out: r[int].f[string](1) // 1
```

Composite selection copies the specialized function environments and retains the original universal clauses, but does not establish a corresponding `functionSelection` describing which binders remain available for elimination. Subsequent method selection enumerates both the selected and original telescopes. It consumes `B` in the specialized clause and consumes the already-selected `A` again in the original one. The resulting extra specialized clause triggers unnecessary unresolved inference during the call.

Keeping the original universal as an obligation is correct; making its consumed prefix available for a new selection is not. This finding is a completeness/compositionality defect, not an unjustified successful proof. It affects the supported combination of explicit instances and separately supplied implementations.

Repair direction: carry the remaining selection telescope into projected function views while preserving original clauses solely as obligations. Test nested binders after record/list selection, bodies supplied in separate declarations, and source export/reimport of these selected views.

**Scope and implementation shortcomings beyond the reproduced defects**

The complete proposal is not implemented. The guide is mostly explicit about this, so these are scope limits rather than claims of undiscovered unsoundness:

| Proposal facility | Current boundary and consequence |
| --- | --- |
| Dependent profile D | General value binders are rejected in `compile/quantified.go`; only finite literal ranges are supported. Function signature constraints use the declaration environment rather than the proposal's left-to-right dependent argument telescope. Successor, length, matrix, and request-indexed examples in D are not generally executable/checkable as written. |
| General universal Boolean predicates | `adt.Universal.validate` unconditionally returns incomplete. Retention is sound, but even some simple uniformly satisfiable combinations of quantified unions and arrows cannot be certified. Higher-rank syntax support is not a general decision procedure. |
| General existential reasoning | Transparent existential membership mainly handles covariant data extrema and reuse of sealed witnesses. General witness synthesis, transparent elimination, and mixed-prefix reasoning remain residual. These restrictions prevent full S_H conformance. |
| Package elimination | `PackageOpen.evaluate` needs a sealed record with exactly one carrier. Scalar packages and multi-carrier openings are unsupported; transparent existential descriptions do not provide an openable witness. |
| Effects and foreign implementations | The certifier rejects implementation proofs for effect-marked or `extern` functions. An extern declaration supplies neither a foreign implementation nor its execution. The checked bridge and codec examples are interface descriptions, not an implemented end-to-end effect system. |
| Totality and recursion | Ground finite-list descent can execute; general recursive termination proofs, many branch-sensitive bodies, and arbitrary arithmetic implications remain incomplete. A successful call is not a reusable function proof. |
| Opaque transport | Recursive schemas, many generic transports, overlapping unions without a transport-agreement proof, dependent nested packages, and partial overloads with different rows retain limitations. The newly reproduced overlap error must be fixed before extending this proof fragment. |
| Source export | Supported closures, captures, and composites are handled, but independent partial closures, opaque packages/operations, some foreign hidden captures, and mixed shared code/composite origins intentionally report incomplete export. These are interoperability limits. |
| Formal assurance | The proposal's theorems assume sound elaboration, local proofs, primitive behavior, and residual retention. They are not proofs of this Go implementation. The specification explicitly says the finite reference-model scripts/results are absent. No independent exhaustive semantic model was available to cross-check the kernel. |

The public three-way distinction between refuted, established, and unresolved is the right implementation boundary. Unsupported proof search should continue to yield incompleteness. Findings 1–5 cannot be explained by that allowance: they either publish an invalid complete observation/certificate, expose forbidden identity information, or manufacture a contradiction.

**Test gaps and repair order**

The existing corpus has meaningful negative tests and extensive regression coverage. Its remaining weaknesses are combinations and observation boundaries:

- Apply the callback and existential membership matrices to regular fields, hidden fields, definitions, and nested composites. A field's omission from JSON must not erase constraints used by CUE code.
- For every certified opaque overload, check the same contract under all clause orders and all admitted packet modes. Exercise positional/name-only overlap, not just different scalar domains.
- Compare aliased and inlined bodyless signatures, shared and distinct private closures, and explicitly aliased public exports. Public handle equality must follow the declared interface.
- Compare direct generic implementations with equivalent contract-plus-implementation declarations. Compose record selection with method selection, then repeat after export/reimport.
- Check universe formation and type selection across every subject kind, including constants and otherwise unused binders.
- Keep separate assertions for ordinary validation, concrete certification, selected-value JSON export, and later refinement. One does not substitute for another.

Prioritize the successful-but-invalid results in findings 1–3, then the abstraction boundary in finding 4 and the false guard refutation in finding 5. Restore consistent selection metadata before broadening inference or proof coverage. General D support and stronger proof search are separate development projects.
