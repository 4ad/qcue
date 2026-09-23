**Quantified CUE: follow-up implementation audit — 23 September 2026**

Audited revision: `d2f481c6567c19431225f4ccf363196afc5c3cfb`.
Specification: [quantified-cue.tex](quantified-cue.tex).

**Repair status: all nine reproduced defects below have fixes and regression
tests.** The repairs address alias elaboration and substitution scopes, retained
callable obligations, bounded proof reuse, opaque identity, and source
preservation. Follow-up regressions also exposed and repaired shared alias-index
guards and free abstract dependencies hidden behind nested package interfaces.
The implementation still supports fragments of the proposal, rather than the
whole language. Unsupported transformations must report incompleteness.

The [previous audit](quantified-cue-audit.md) describes an older baseline and
records its repairs. The numbered findings here describe revision `d2f481c65`
**before these repairs**. Their actual outputs are historical reproductions,
not the behavior expected after the fixes.

**Repairs and regression coverage**

| Finding | Repair | Regression |
| --- | --- | --- |
| 1. Alias erasure | Follow free dependencies and actual substitutions; keep runtime-index guards on individual uses rather than shared templates. | [erasure fixtures](../cue/testdata/quantified/api/erasure.txtar), [alias API checks](../cue/alias_witness_test.go) |
| 2. Alias witness decoding | Separate predicate and runtime compilation while sharing lexical binders and code origins. | [alias API checks](../cue/alias_witness_test.go) |
| 3. Callback obligations | Discharge actual packet membership before returning or memoizing a result; share the independent proof context. | [callback packets and package proofs](../cue/certification_protocol_test.go) |
| 4. Nested seals | Preserve independent packages by identity; retain public schemas and reject unresolved dependent transports. | [nested package fixture](../cue/testdata/quantified/opaque_composite_transport/nested_packages.txtar), [opacity API checks](../cue/opaque_transport_test.go) |
| 5. Predicate export | Serialize residual dependencies as predicates, retaining closedness, presence, patterns, defaults, and lexical scopes. | [predicate export/recompile](../cue/lexical_export_test.go) |
| 6. Hidden captures | Export observable hidden fields; reject foreign hidden labels that cannot be rebound faithfully. | [capture export/recompile](../cue/lexical_export_test.go), [foreign hidden capture](../internal/core/export/testdata/quantified/foreign_hidden_capture.txtar) |
| 7. Callback identity | Recognize inverse boundary transports when comparing identities, retaining execution adapters and their obligations. | [callback round trips](../cue/testdata/quantified/opaque_callbacks/round_trip_identity.txtar), [containers and partial callbacks](../cue/testdata/quantified/opaque_callbacks/round_trip_containers.txtar) |
| 8. Composite export | Retain lexical introductions and selections, emit shared composite code once, and reject unsupported mixed code projections. | [composite export/recompile](../cue/composite_export_test.go), [lexical selection](../cue/testdata/quantified/composite_instances/lexical_selection.txtar), [mixed export diagnostic](../internal/core/export/testdata/quantified/mixed_composite.txtar) |
| 9. Proof expansion | Memoize successful proofs with their required hypotheses and bound total proof work; exhaustion remains incomplete. | [deterministic work-budget checks](../internal/core/subsume/certify_work_test.go) |

The architectural changes keep three distinctions explicit: shared syntax versus
per-use elaboration, executable identity versus attached validation obligations,
and predicates versus runtime data. Transport and export now preserve those
distinctions instead of reconstructing a weaker value from visible fields.
Successful proof reuse records its assumptions; failed or active checks never
become cached evidence.

The export repair supports records, lists, telescopes, refinements, selected and
excluded views, shared copies, and functions returning generic composites.
Exporting a composite together with a separately exported method of the same
introduction remains unsupported and returns a typed incomplete-export error.
Dependent nested-package transport likewise remains incomplete; it no longer
exposes a private instantiation through a public definition or pattern.

**Validation after repair**

The focused compiler, evaluator, public API, exporter, and subsumption suites
pass. The complete `go test ./...` suite also passes across **121 packages with
tests**, including the CLI tests that require local listening sockets. New
resource regressions use deterministic work counts, not timing thresholds.
Every reproduced defect has a regression in the table above; additional tests
exercise declaration permutations, nested containers, refinement, source
export/recompile, and safe incomplete results at unsupported boundaries.

**Baseline scope and validation**

I compared the proposal's binding, quantification, call-packet, identity, opacity, universe, residual-validation, and preservation requirements against the compiler, evaluator, conformance checker, subsumption, exporter, and regression corpus. I exercised additional programs through the CLI and public Go API, including declaration permutations and export/recompile checks.

The focused tests passed:

```sh
go test ./internal/core/adt ./cue ./cue/parser ./cue/format \
  ./cue/ast/astutil ./internal/core/export ./internal/core/subsume \
  ./internal/core/compile
```

The complete `go test ./...` suite also passed: **121 packages with tests**. The first full-suite attempt was blocked by the sandbox's prohibition on listening sockets. Rerunning with local networking available passed. These are therefore gaps in the passing regression suite, not failures already caught by it.

I built the CLI with `go build -o /tmp/qcue-audit-current ./cmd/qcue`. CLI examples below should be run from a temporary directory outside this repository's older-version module, or with the quantified experiment explicitly enabled. These reproductions were collected before changing production code. Performance trials had a five-second timeout; I did not run an exhaustion test against the baseline.

For API examples, `ctx` denotes `cuecontext.New()`. Concrete validation means `v.Validate(cue.Concrete(true))`; ordinary `v.Validate()` is allowed to retain unresolved obligations. A failure to prove an unsupported proposition is not classified as a semantic defect here.

**1. [P1] Parametric aliases can bypass erasure, making unification observably noncommutative**

Location: [compile/erasure.go](../internal/core/compile/erasure.go), with the identity consequences in [adt/closure.go](../internal/core/adt/closure.go).

```cue
f(A: int): func() -> int: {
    Hidden(B) = A
    out: Hidden(int)
}.out

left:  f[1] & f[2]
right: f[2] & f[1]
out: [left(), right()]
```

Actual: `qcue export -e out` succeeds and prints `[1, 2]`.

The alias has a fixed argument, `int`, but its body captures the enclosing erased parameter `A`. The erasure walker examines an alias template's body only when an **argument** contains an erased parameter. It never checks this free dependency. In contrast, directly returning `A` is correctly rejected.

Consequently, two selections of the same supposedly erased zero-argument implementation have different runtime behavior. Closure identity considers them the same origin with the same runtime captures, and the meet retains whichever executable view is on the left. Reversing two conjuncts changes a concrete result.

This violates both the erased, one-subject interpretation and the intersection semantics of unification. It is stronger evidence than merely accepting an unusual surface form. Root concrete validation of the generic function remains incomplete, but formation and ordinary demanded execution already admit the invalid behavior.

The erasure check must follow free dependencies of alias templates as well as substitutions, while preserving the distinction between type-only and runtime occurrences. Regression tests should compare direct code, ordinary aliases, parametric aliases with fixed arguments, and both orders of selected-view unification.

**2. [P1] A parametric alias loses singleton-witness dependencies depending on declaration order**

Locations: [compile/quantified.go](../internal/core/compile/quantified.go) and [compile/compile.go](../internal/core/compile/compile.go).

```cue
x: int
Alias(A) = x & A
f: func(y: Alias(int)) -> string: "ok"
out: f(2)
```

Actual:

- `out.MarshalJSON()` succeeds with `"ok"`.
- Concrete validation of `f` succeeds despite the unresolved ordinary witness `x`.
- Exporting `f` independently captures `int`, permanently replacing the witness by its current approximation.

Move only the alias declaration:

```cue
x: int
f: func(y: Alias(int)) -> string: "ok"
Alias(A) = x & A
out: f(2)
```

Now exporting `out` correctly reports `singleton witness in function signature remains unresolved`, and concrete validation of `f` remains incomplete. Inlining `x & int` also correctly preserves the obligation. Refining `x` to `1` makes the first program's call conflict.

`aliasTemplate` caches one compiled template per AST declaration, without distinguishing predicate and runtime compilation. Visiting the alias declaration eagerly compiles it in an ordinary expression context. A preceding signature use instead compiles it in a type context. The ordinary `let` repair has separate predicate compilation, but parametric aliases do not.

This breaks capture-avoiding abbreviation, ordinary-witness singleton decoding, and declaration-order independence. The fix needs contextual alias expansion/caching with stable binder and function-origin identities, not merely a special case for `x & A`.

**3. [P1] Complete calls can discard the conformance obligation on an actual callback**

Locations: [adt/expr.go](../internal/core/adt/expr.go), [adt/expr.go](../internal/core/adt/expr.go), and [adt/validate.go](../internal/core/adt/validate.go).

```cue
out: (func(cb: func(int) -> 1) -> int: 5)(
    func(n: int) -> int: {value: n}.value,
)
```

Actual: root `Validate(cue.Concrete(true))` and `qcue vet -c` both succeed. JSON export produces `5`.

The supplied callback is identity, so it does not inhabit `func(int) -> 1`: input `0` is a counterexample. Checking that callback with the contract independently leaves conformance unproved. Calling `cb(0)` instead of returning `5` exposes the conflict. Even calling `cb(1)` allows the enclosing program to pass concrete validation, despite the invalid universal callback promise.

There is also a revealing syntactic comparison: replacing `{value: n}.value` with `n` makes speculative counterexample checking catch the invalid callback at packet `(0)`. Wrapping the same body in a record causes the obligation to disappear from complete validation rather than remain incomplete.

The final parameter check sets `Final` and `ReportIncomplete`, but neither enables concrete callable checking nor supplies an independent function checker. The call then returns, and can memoize, a scalar result without carrying this unresolved argument-membership obligation. Later root validation has no visible callback to revisit.

This finding concerns the **actual packet's declared callback-membership obligation**. It does not require proving every property of every callee merely to observe one execution. The proposal expressly distinguishes local execution from universal certification, but also requires actual callback obligations to survive the caller's conditional proof. The implementation must retain this dependency and either discharge it when completeness is demanded or report incompleteness.

**4. [P1] Transporting a nested existential package erases its seal**

Location: [adt/package.go](../internal/core/adt/package.go), especially construction of the replacement record at line 659.

```cue
#Inner: exists B {tag: 1}
#Outer: exists A {inner: #Inner}

p:     seal #Inner with (B = int) {tag: 1}
other: seal #Inner with (B = int) {tag: 1}
q:     seal #Outer with (A = int) {inner: p}

out: (open q as (A, Q) {
    result: Q.inner & other
}).result
```

Actual: root concrete validation succeeds. `out` can even be exported as ordinary JSON, `{"tag":1}`.

The corresponding direct `p & other` must conflict because the seals are distinct. Another observation of the same defect is that `open Q.inner as (B, I) {...}` fails with `package witness is not available for opening`. Simply returning `Q.inner` exports its data, whereas exporting the original sealed package is correctly blocked.

The recursive transport path treats the nested package as an ordinary record and constructs a new vertex from its fields. It neither retains the nested `sealed` identity nor treats the closed existential as an independent abstraction boundary. Thus it loses package operations and equality distinctions in addition to the export boundary.

Transport should preserve an already closed package when no transformation of its bound witness is needed, or use an explicit package-aware transport rule. If unsupported, it must remain incomplete. Rebuilding the visible record is not an equivalent fallback. Regressions need nested packages in records, lists, arguments, and results, including packages with no directly opaque public fields.

**5. [P1] Residual quantifier export weakens captured predicates**

Location: [export/value.go](../internal/core/export/value.go), specifically `e.value(value)` at line 514.

```cue
#T: {a?: int}
I: exists A {value: #T, zero: A}
```

Export just `I` through the public API and recompile the result independently:

| Export request | Emitted expression |
| --- | --- |
| `I.Syntax()` | `exists (A) {value: ({a?: int}), zero: A}` |
| `I.Syntax(cue.Final())` | `exists (A) {value: ({}), zero: A}` |

Ordinary source export loses the definition's closedness. Final export additionally loses the optional integer constraint.

These are observable changes to accepted implementations. After importing the exported expression as `v`, use:

```cue
p: seal v with (A = int) {
    value: {b: 1}
    zero: 0
}
out: (open p as (A, P) {result: P.value.b}).result
```

Both exports accept this implementation; concrete validation of `p` succeeds and `out` is `1`. The original `#T` forbids field `b`. With Final export, `{a: "bad"}` is also accepted, and the public operation-free observation returns `"bad"`; the original rejects the integer/string conflict.

The recent schema-preserving export helper is used for function-origin predicate captures, but residual `Universal`/`Existential` templates use a separate substitution path that still serializes dependencies as data. Predicate dependencies in every export path need schema preservation, including closedness, optional/required fields, patterns, defaults, and scoped obligations.

**6. [P2] Closure export removes hidden fields needed by runtime code**

Location: [export/closure.go](../internal/core/export/closure.go), through [adt/composite.go](../internal/core/adt/composite.go).

```cue
r: {_secret: 7}
f: func() -> int: r._secret
out: f()
```

The original passes concrete validation and returns `7`. Both ordinary `f.Syntax()` and `f.Syntax(cue.Final())` successfully emit the equivalent of:

```cue
{
    CUECode({})
    let CUECode = func(CUECapture: _) -> _:
        func() -> int: CUECapture._secret
}
```

After recompilation, calling this closure fails with `undefined field: _secret`.

`functionOriginValue` projects a runtime capture using `ToDataAll`, which deliberately removes nonregular fields. The captured value here is the whole record `r`, and its hidden field is read by the executable body. A JSON-style projection is not an adequate serialization of that runtime environment.

The exporter must carry the data the code can observe, with correct hidden-label identity, or capture the selected free value directly. If it cannot preserve the capture, it should report incomplete export rather than emit broken code. This also needs coverage for hidden fields inside composite captures and for package-qualified hidden labels.

**7. [P2] Passing a callback through a sealed identity operation changes its identity**

Locations: [adt/package.go](../internal/core/adt/package.go) and [adt/closure.go](../internal/core/adt/closure.go).

```cue
#M: exists A {
    id: func(func(int) -> int) -> (func(int) -> int)
}
p: seal #M with (A = int) {
    id: func(f: func(int) -> int) -> (func(int) -> int): f
}
f: func(x: int) -> int: x

out: (open p as (A, P) {
    same: P.id(f) & f
}).same
```

Actual: `conflicting function identities`.

The private operation returns its supplied callback unchanged. Boundary transport wraps the callback going inward and wraps it again going outward. The resulting double adapter is compared against the original function as a different code origin. The adapter cache does not normalize opposite-direction transport back to the original public value.

This occurs even though the callback's signature contains no abstract type. It also occurs when passing a module's own abstract operation through an identity callback operation, so avoiding adapters for entirely ordinary signatures would fix only part of the issue.

The proposal makes equality/unification observable and requires equality-respecting transport and preservation of public aliasing. Transport should preserve that round trip, while still distinguishing genuinely independent exported operation handles. Regressions should compare returned callbacks with their originals and repeated/copy-derived handles, through records and lists as well as direct arrows.

**8. [P2] Evaluated export drops composite quantifier introductions and breaks type selection**

Locations: [export/value.go](../internal/core/export/value.go), and the `subjectScheme` metadata maintained by [adt/subject.go](../internal/core/adt/subject.go).

```cue
r(A): {f: func(x: A) -> A: x}
out: r[int].f(1)
```

The original passes concrete validation. `qcue eval` emits:

```cue
r: f: CUECode
out: 1
let CUECode = forall (A) func(x: A) -> A: x
```

After recompilation, adding `again: r[int].f(2)` fails with `invalid non-ground value int (must be concrete int)`. The original allows this selection and returns `2`.

`Value.Syntax(cue.Final())` exhibits the same problem. Ordinary source-oriented `Value.Syntax()` preserves the original quantifier and passes the round-trip check, so this is specifically an evaluated/Final export gap.

The record's fields are exported, including the generic function, but the record's selectable quantifier introduction is not. Preserving each field's arrow obligations is insufficient to preserve the composite subject's type-application interface. A similar example, `r(A): [...A]`, is printed as `r: []`, losing its selection interface.

The exporter needs a representation for retained composite introductions and selected views, or should explicitly report unsupported export. Tests should exercise *new* selections after recompilation, not only compare already computed output fields.

**9. [P2] Certification expands a small shared proof graph exponentially**

Locations: [subsume/certify.go](../internal/core/subsume/certify.go), [subsume/certify.go](../internal/core/subsume/certify.go), and [subsume/certify.go](../internal/core/subsume/certify.go).

Generate a straight-line family:

```cue
f0: func(x: int) -> int: x
f1: func(x: int) -> int: f0(f0(x))
f2: func(x: int) -> int: f1(f1(x))
// Continue the same pattern through fN.
```

There are no ground calls in this input; `qcue vet -c` is checking the declared functions. Observed elapsed times on this checkout were:

| Highest function | Source bytes | Result |
| --- | ---: | --- |
| `f5` | 202 | Pass, approximately 0.011 s |
| `f7` | 272 | Pass, approximately 0.045 s |
| `f9` | 342 | Pass, approximately 0.588 s |
| `f10` | 378 | Pass, approximately 2.287 s |
| `f11` | 416 | Still running at the five-second timeout; terminated |

These are local observations, not portable thresholds. The code explains the growth: resolving a captured function proves its implementation; checking the call proves that implementation again. Each function contains two uses of its predecessor. There is an active-proof stack and a depth limit, but no completed-proof memoization or work budget to bound this repeated expansion.

This is independent of the repaired finite-quantifier enumeration budget. A depth limit does not bound the width of a proof tree. The residual chapter requires speculative checking to be resource-bounded and to retain the exact obligation on exhaustion. Reusing proofs keyed by implementation, target, scope, and assumptions would handle this shared acyclic case; a separate proof-work budget is still needed for unsupported or expensive cases.

**Coverage gaps relative to the full proposal**

These are principally acknowledged limitations, not evidence that a conservative incomplete answer is wrong. They nevertheless mean that the implementation should not be described as a complete implementation of S_H, A, or the document as a whole.

| Required area | Current implementation and consequence |
| --- | --- |
| General value dependence, profile D | [The compiler](../internal/core/compile/quantified.go) accepts literal finite ranges but rejects general value binders. Parameter/result annotations resolve outside the runtime parameter scope. The dependent length, matrix-dimension, safe-index, and request-indexed contracts are not generally implemented. |
| Arbitrary quantified Boolean predicates | [Universal.validate](../internal/core/adt/abstract.go) always returns incomplete. Supported arrows, fixed-record distribution, covariant data extrema, and finite enumeration do not constitute general quantified Boolean checking. |
| Transparent existential introduction/elimination | [Existential.validate](../internal/core/adt/package.go) covers a covariant extremal rule and reuse of a sealed witness. There is no general scoped witness solver or transparent existential opening. Noncovariant and mixed-prefix obligations often remain residual. |
| First-class package elimination | [PackageOpen](../internal/core/adt/package.go) requires a sealed record and one carrier. Scalar and multiple-carrier packages are not generally openable. Finding 4 concerned an already supported, one-carrier package losing its identity when nested; that path is repaired. |
| Generic client proofs using `open` | The repairs add a proof rule using a fresh abstract carrier and the admitted interface's operation hypotheses. It covers supported single-carrier clients; arbitrary existential elimination remains outside the proof fragment. Running one concrete client call still does not establish its reusable contract. |
| Effects and foreign execution | [certifier.function](../internal/core/subsume/certify.go) leaves effect-annotated and `extern` implementations unproved. Tags participate in some capability comparisons; the full checked-outcome/bridge semantics and proofs are absent. |
| Structural recursion | [The evaluator](../internal/core/adt/recursion.go) admits some concrete finite-list descent. The certifier's active-call rejection is not a general termination certificate, and recursive implementations remain outside its general proof fragment. |
| Structural proof completeness | Optional presence branches, several conditionals and expression forms, arbitrary Boolean inclusion, arithmetic implications, and many vacuous cases remain incomplete. These limits are permissible only while obligations remain attached. |
| Opaque transport coverage | [totalTransport](../internal/core/adt/opaque_proof.go) supports a restricted structural fragment. Generic/recursive and overlapping transports can remain incomplete. Direct builtin implementations are also not handled by the function transport branch, which requires a `FuncValue`; a sealed `strings.ToUpper` operation does not currently work directly. |
| Independent source export | Partial closures, opaque operations/packages, some captured graphs, and other unsupported values deliberately report incomplete export. That is a reasonable boundary; findings 5, 6, and 8 concerned paths that previously returned apparently successful, changed source. Mixed composite/method graphs now explicitly report incompleteness. |
| Certificates and incremental resource policy | The code uses retained ADT constraints, assumption-aware proof reuse, probes, and bounded proof work. It does not implement the independently checked certificate graph and task-local scheduling/resource interface described in the residual chapter. The repeated proof expansion in finding 9 is now covered by deterministic budget regressions. |
| Representation-independent replacement theorem | There is no general checker for equality-respecting module simulations or interface laws. The theorem is conditional in the proposal; passing seal construction or individual counter examples cannot establish it. The repairs for findings 4 and 7 address concrete violations of its prerequisites. |

The current appendix accurately says the finite reference-model scripts and result files are not included. The earlier audit's historical complaint about stale assertion-count claims has been corrected; it should not be reported as a current defect. There is still no executable finite reference-model evidence in this checkout to supplement the implementation tests.

**What the implementation does establish**

There is real support across the language stack: lexical quantifier syntax, declaration sugar, alpha-renamable binders, aliases, explicit and inferred instances, retained guarded arrow clauses, rigid-variable proofs for selected higher-rank programs, intensional closure identity, partial application, concrete finite-list recursion, finite literal quantification, and substantial seal/open transport. The regression corpus also exercises many important negative cases and all the repairs from the prior audit.

The distinction between contradiction and incompleteness is present in the architecture. Arbitrary universal residuals are not automatically certified, and the recent finite-expansion implementation retains the full predicate on budget exhaustion. Those were sound design choices already present at the baseline. The repairs address the separate paths above that erased obligations or identities.

**Testing strategy**

The repairs add invariance tests across equivalent forms and tests composing independently supported features:

1. Alias declaration permutations, alias inlining, and predicate/runtime uses of the same parametric alias. Check output completeness, closure capture completeness, later refinement, and export.
2. Erasure through alias free variables, nested aliases, fixed arguments, and the commutativity/idempotence of selected-view unification.
3. Callback arguments whose bodies are equivalent but fall inside/outside the speculative probe vocabulary. Validate the entire resulting program, not just a separately retained named callback.
4. Nested seals with equal visible data and distinct identities; preservation through containers, calls, equality, source export, and reopening.
5. Schema export through residual quantifiers as well as through closures, in both ordinary and Final modes. After recompilation, try forbidden additional fields and invalid values of optional fields.
6. Runtime captures containing hidden fields. Reinvoke the exported closure, rather than merely compare its old materialized outputs.
7. Boundary transport round trips for callbacks and public handles, including identity/unification observations.
8. New composite type selections after evaluated export/recompile.
9. Shared acyclic proof graphs with a bounded-work assertion. Tests should check residual behavior on budget exhaustion rather than depend on machine-specific timings.

These repair priorities are now represented by the fixes and regressions listed
above. They do not establish the proposal's general preservation theorems or
complete its intentionally unsupported fragments. The
[implementation guide](quantified-cue-implementation.md) records the current
supported behavior and remaining boundaries.
