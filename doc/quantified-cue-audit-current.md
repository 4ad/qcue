**Quantified CUE audit of revision `530f554f0700` — 23 September 2026**

Specification: [quantified-cue.tex](quantified-cue.tex). Implementation scope: [quantified-cue-implementation.md](quantified-cue-implementation.md).

**Verdict at the audited revision: the extension is substantially implemented, but it does not faithfully enforce the specified semantics, including within the supported fragments.** I reproduced six current defects. Two permit unjustified successful validation, others reject valid inhabitants or calls, discard data during transport, or miss abstract-type escape. These are separate from the documented restrictions on dependent types and proof search.

The findings below describe revision `530f554f0700`, before repair. They do not repeat historical findings from the two previous audit reports as though their repairs had not happened. The audit itself changed no production code.

**Repair status — 24 September 2026:** all six reproduced defects have regression tests and implementation repairs, documented in the resolution table below. The elementary unary-proof gap is also repaired. The overloaded-adapter repair additionally checks transport agreement on overlapping domains; independent private proofs no longer suffice to certify incompatible public representations.

**Method and validation**

I read the proposal's semantic rules, surface language, examples, checking and residual-state requirements, and implementation milestones; traced the main parser, binding, elaboration, quantification, inference, capability, closure, universe, package, certification, validation, and export paths; and exercised additional small programs through the public Go API and CLI.

The focused quantified evaluator, API, parser, formatter, AST, and exporter tests passed. The complete `go test ./...` run also passed: 121 packages reported `ok`, with no failing packages. The first sandboxed full run could not create local test-server sockets; the successful run had those permissions. A writable temporary Go build cache avoided a separate sandbox cache restriction. A passing suite therefore does not cover the defects below.

Build the executable from the audited checkout:

```sh
GOCACHE=/tmp/cue-audit-go-cache go build -o /tmp/cue-audit ./cmd/cue
```

Save each example as a separate file under a temporary directory outside this repository's pinned CUE module. Each example below uses `@experiment(quantified)` explicitly. Use `cue vet -c case.cue` for root concrete validation and `cue export -e out case.cue` for a demanded result. A named function or package can be checked separately through:

```go
v := cuecontext.New().CompileString(source)
x := v.LookupPath(cue.ParsePath("out"))
err := x.Validate(cue.Concrete(true))
data, exportErr := x.MarshalJSON()
```

Ordinary `Validate()` is allowed to retain incomplete obligations. Failure to prove a proposition is not itself a correctness defect. P1 below denotes a semantic or checking defect to fix before relying on the affected guarantee; P2 denotes a significant scope, export, or transport defect.

**1. [P1] Partial-closure certification does not check saved arguments against their parameter contracts**

Primary location: [subsume/certify.go](../internal/core/subsume/certify.go), lines 231–246 and 288–299. Partial argument construction is in [adt/expr.go](../internal/core/adt/expr.go), around line 2384.

```cue
@experiment(quantified)
f: func(cb: func(int) -> 1, y: int) -> int: 0
cb: func(x: int) -> int: {v: x}.v
p: f(cb, ...)
g: func(y: int) -> int: p(y)
```

Actual: root `Validate(cue.Concrete(true))` and `cue vet -c` succeed. Both `p` and `g` are individually certified. Adding `out: g(2)` makes the demanded call incomplete with `function conformance remains unproved`.

The saved callback is identity, and input `0` witnesses that it does not inhabit `func(int) -> 1`. Certification should not establish either the partial closure's residual capability or the wrapper's successful arrow. Retaining an incomplete obligation would be sound.

The initial loop checks the saved callback's own implementation contract, `func(int) -> int`. It does not check the contract required by `f`'s parameter. The subsequent bound-argument membership check runs only when `partial != nil`. For the original obligation, `target.Fn == f.Fn`, so the branch at line 240 does not set `partial`; the body proof substitutes a hypothetical conforming callback instead of validating the actual saved one.

This violates the proposal's requirement that a caller preserve the proof obligation on an actual callback, and its successful-arrow soundness requirement (specification lines 2487–2491 and 2182–2189). It is not merely a failure to prove termination or a difficult higher-rank proposition.

Repair direction: keep checking the original implementation universally, but independently validate every saved slot against its required domain, including higher-order membership and nested records/lists. A saved argument's own valid annotation is insufficient. Retain any undecided membership obligation on the partial closure and its callers.

Test gap: `certification/partial_closures.txtar` already tests a saved callback whose *own implementation* is invalid. It needs a callback whose own contract is valid but whose contract required by the partial application is stronger.

**2. [P1] A failed inferred type assignment is treated as a refutation; guarded calls can lose applicable obligations**

Locations: [adt/quantified.go](../internal/core/adt/quantified.go), lines 673–727 and the callback inference case beginning around line 793; [adt/capability.go](../internal/core/adt/capability.go), lines 389–397.

First, a valid call is contradicted:

```cue
@experiment(quantified)
f(A: int): func(cb: func(A) -> int) -> int: 0
cb: func(x: number) -> int: 1
explicit: f[int](cb)
out: f(cb)
```

Actual: `explicit` exports `0`, but `out` is a hard conflict: `type argument number does not satisfy bound of A`. The instance `A=int` demonstrably admits the packet. Inference reads the callback's input `number` as the chosen type argument instead of using the contravariant requirement `A <= number` together with `A <= int`.

Dependent type bounds expose the same problem without callbacks:

```cue
@experiment(quantified)
f(A: int, B: A): func(x: B) -> B: x
explicit: f[int][int](1)
out: f(1)
```

Actual: the explicit call exports `1`; the implicit call conflicts because `1` does not satisfy the inferred bound of `B`. Inference guesses bottom for the unconstrained `A`, then treats failure of `B=1 <= A=bottom` as failure of the call. `A=int, B=1` is a valid assignment.

More seriously, when the same inference procedure is used for an attached universal capability, it can discard a result constraint:

```cue
@experiment(quantified)
cb: func(x: number) -> int: 1
out: ((func(cb: _) -> int: 0) &
      (forall (A: int) func(func(A) -> int) -> 1))(cb)
```

Actual: root concrete validation and `cue vet -c` succeed; `out` exports `0`. But `cb` belongs to `func(int) -> int`, so the retained universal's `A=int` clause requires the actual result to be `1`. Selecting `[int]` explicitly before this call exposes the conflict `0` versus `1`.

`inferInstance` returns the guessed assignment's non-incomplete bound error. `scheduleCapabilityResults` interprets that as exclusion of the guarded clause and continues without preserving another possible instance or an unresolved guard. A failed existential candidate is thereby promoted into evidence about all possible candidates.

This directly violates the specification's distinction between failed candidates and failed configurations, its rules for flexible witnesses, and its prohibition on discarding residual premises (lines 2538–2560, 2562–2590, and 2294–2379).

Repair direction: infer lower/upper constraints with the appropriate variance and telescope dependencies. When inference is incomplete, report an incomplete instance or retain the original applicability predicate. Only a proof that *no* admissible instance admits the packet justifies excluding an entire universal capability from that call.

Test gap: cover implicit/explicit agreement, variables appearing only in other binders' bounds, contravariant callback inputs, and attached clauses using those same inference paths. Merely checking a rejected explicit type argument does not test this distinction.

**3. [P1] Sealing an overloaded operation exposes only its first callable clause**

Primary location: [adt/package.go](../internal/core/adt/package.go), lines 687–688; adapter construction begins around line 577. Related proof path: [adt/opaque_proof.go](../internal/core/adt/opaque_proof.go), `ProofTypes`.

```cue
@experiment(quantified)
#M: exists A {
    f: (func(int) -> int) & (func(string) -> string)
}
p: seal #M with (A = int) {
    f: func(x: _) -> _: x
}
out: (open p as (A, P) {result: P.f("x")}).result
```

Actual: the package without `out` passes concrete validation. `P.f(2)` succeeds, but `P.f("x")` conflicts with `int`. Reversing the two interface clauses changes which domain is exposed.

The private implementation is identity and satisfies both promises. The public interface must admit both packet domains. This is a failure of the declared public capability after otherwise successful package certification.

The resolved interface is a `FuncValue` with a head function and additional `Types`. Transport passes only `f.Env` and `f.Fn` into the adapter constructor, dropping `f.Types`. The adapter consequently has one protocol and one result schema. The proof path also sees that single advertised signature, so validation does not discover the public adapter's lost capability.

This violates arrow-intersection semantics, retention of all conjunction obligations, and preservation of declared public operations through sealing (specification lines 627–657 and 1856–1894).

Repair direction: transport the complete capability value, preserving every clause and its environment, and check the public adapter's coverage of the complete interface. If transport cannot yet handle an intersection, it must return incompleteness rather than publishing a successfully certified single-clause substitute.

Test gap: place intersections and generic clauses *inside an exported operation's type*, rather than only conjoining whole package interfaces. Exercise all domains and both clause orders.

**4. [P1] Singleton and opaque-value equality still confuse contracts with runtime identity**

Locations: [adt/witness.go](../internal/core/adt/witness.go), line 72; [adt/equality.go](../internal/core/adt/equality.go), lines 172–174 and 241–255; [adt/funcsig.go](../internal/core/adt/funcsig.go), lines 1301–1309. Opaque `==` uses the same equality mode in [adt/binop.go](../internal/core/adt/binop.go), lines 77–79.

```cue
@experiment(quantified)
base: func(x: int) -> int: x
a: {f: base}
b: {f: base & (func(int) -> int)}
accept: func(x: a) -> int: 1
out: accept(b)
```

Actual: `b` passes concrete validation, but `out` conflicts with the singleton witness. The two records contain the same implementation. The additional contract is true and redundant; it does not change the closure's code, captures, or partial arguments.

There is an analogous failure inside one opaque carrier:

```cue
@experiment(quantified)
#M: exists A {
    left: A
    right: A
    same: func(A, A) -> bool
}
base: func(x: int) -> int: x
p: seal #M with (A = {f: func(int) -> int}) {
    left: {f: base}
    right: {f: base & (func(int) -> int)}
    same: func(x: {f: func(int) -> int},
               y: {f: func(int) -> int}) -> bool: true
}
out: (open p as (A, P) {
    result: P.same(P.left & P.right, P.left)
}).result
```

Actual: the meet of the abstract values conflicts. The representations differ only by a redundant contract on the same nested function.

The earlier repairs correctly use `runtimeIdentity` when comparing closure captures. These other observations still call `Equal` with flags `0`; nested functions then go through `equalFuncValues`, which also demands equal `Types`. Opaque equality propagates the same flags into its private representation.

The specification identifies closures by origin, captured runtime values, and normalized partial bindings, and says a successful conformance check preserves the closure (lines 659–664 and 1425–1465). The opaque equality model must use that same value identity (lines 1960–1974). Keeping contracts distinct for constraint-graph comparison is useful, but cannot define equality of already concrete operational inhabitants.

Repair direction: use operational identity for concrete singleton membership and abstract-value equality while retaining every contract as a separate validation obligation. Do not globally erase contracts from equality used to deduplicate constraint alternatives.

Test gap: extend the existing captured-contract identity regressions to singleton membership, lists/records containing callables, builtins, opaque equality, and opaque meet.

**5. [P2] Ordinary record transport drops hidden data fields**

Locations: [adt/package.go](../internal/core/adt/package.go), lines 568–571 and 738–749.

```cue
@experiment(quantified)
#M: exists A {id: func({}) -> {}}
p: seal #M with (A = int) {
    id: func(x: {}) -> {}: x
}
plain: (func(x: {}) -> {}: x)({_secret: 7})._secret
out: (open p as (A, P) {
    result: P.id({_secret: 7})._secret
}).result
```

Actual: `plain` is `7`; `out` fails with `undefined field: _secret`. Returning the transported record instead of selecting its field produces `{}`. The package and the record-returning version pass concrete validation.

Both transport paths preserve extra fields only if `Label.IsRegular()`. That condition removes hidden data even for ordinary open-record arguments/results. These are not private declarations on the module root; they are caller-supplied data flowing through an identity operation.

The specification preserves host record-field distinctions and ordinary data observations. The implementation guide also explicitly promises that ordinary open-record arguments/results retain their extra data, reserving interface projection for the module root. A field that is omitted from JSON can still be observed by CUE code.

Repair direction: separate module-root export projection from ordinary record transport, preserving hidden field identity and observable contents in the latter. Include nested records, lists, and callback round trips.

Test gap: the existing open-record transport tests cover regular extra fields; the hidden-capture tests concern source export, which is a different transport path.

**6. [P2] Abstract escape checking misses singleton predicates and attached function clauses; final export can emit a free type name**

Locations: [adt/package.go](../internal/core/adt/package.go), `abstractEscapes` at lines 974–1045; [compile/captures.go](../internal/core/compile/captures.go), lines 112–113; [export/closure.go](../internal/core/export/closure.go), `functionOriginValue` and `functionOrigin`.

A singleton predicate can hide the free abstract dependency:

```cue
@experiment(quantified)
#M: exists A {zero: A}
p: seal #M with (A = int) {zero: 0}
out: (open p as (A, P) {
    result: {w: {v: P.zero}, f: func(x: w) -> int: 0}.f
}).result
```

Actual: root and `out` concrete validation succeed. The escaped function's domain is the singleton of a record containing the local abstract value; no existential package binds that dependency. A directly written domain containing the abstract type is rejected.

`abstractEscapes` has no `WitnessType` case, so it never visits this singleton predicate's witness/upper graph. Its function case also examines only `Fn.Params` and `Fn.Ret`, ignoring attached `Types`:

```cue
@experiment(quantified)
#M: exists A {zero: A}
p: seal #M with (A = int) {zero: 0}
out: (open p as (A, P) {
    result: (func(x: _) -> _: x) & (func(A) -> A)
}).result
```

Actual: root and `out` concrete validation succeed. Formatting `out.Syntax(cue.Final())` returns this source without an incomplete-export diagnostic:

```cue
{
    CUECode & CUECode_1 & CUECode_1
    let CUECode = func(x: _) -> _: x
    let CUECode_1 = func(A) -> A
}
```

Recompilation in a fresh context fails with `reference "A" not found`. The lexical-dependency collector skips opened type names even while collecting non-runtime references, so the final exporter never captures or rejects this free dependency. Ordinary source export of this example does report an opaque-boundary error; final export does not.

This violates the existential elimination escape condition and self-contained source preservation. The first example has a nonredundant singleton dependency. The second attached clause is redundant for identity, but the implementation neither removes it by proof nor exports it faithfully. I did not demonstrate extraction of the private integer representation; the confirmed observations are an accepted out-of-scope predicate and invalid exported source.

Repair direction: inspect all retained signature clauses and singleton predicate dependencies during escape checking, preserving witness scopes. Track opened types as lexical dependencies for export even though they are erased from runtime captures. Unsupported escaping/export graphs must remain incomplete or be rejected explicitly.

Test gap: compare direct abstract occurrences with occurrences hidden in singleton witnesses and attached contracts. Recompile successful exports in a fresh context without any surrounding name `A`.

**Implemented features and remaining scope**

The implementation is much more than surface syntax. It has lexical binder identity, contextual aliases, explicit/implicit instances, retained universal clauses, intensional closures, partial calls, predicative checks, finite witness enumeration with a budget, opaque carriers/adapters, a structural certifier, and extensive regression fixtures. The existing fixes are substantial. Nevertheless, success in parsing, evaluating one call, and certifying a universal contract are three different levels of support.

The following restrictions are generally documented and should be distinguished from the six defects above:

| Area | Current boundary and practical consequence |
| --- | --- |
| Dependent profile D | General value ranges and parameter-dependent signatures are not implemented. Most vector, matrix, protocol-indexing, and arithmetic-law examples in the proposal are specifications of future support. Finite literal value binders are a restricted exception. |
| Existentials | Membership is mainly covariant data reasoning or reuse of an existing sealed witness. General witness synthesis and transparent existential elimination are absent. Opening supports one unbounded representation binder and record-shaped packages. This is a fragment of S_H + A. |
| Boolean quantified predicates | General Boolean placement remains residual. No general semantic-subtyping or quantified decision procedure is implemented. This is permitted provided the residual stays exact. |
| Certification | Optional-presence branches, general conditionals/arithmetic, recursive termination proofs, many builtins, effectful code, and foreign functions remain unsupported. Successful individual calls do not establish these contracts. |
| Even elementary proof coverage | `not: func(x: bool) -> bool: !x` remains unproved under concrete validation, although `not(true)` exports `false`. `certifier.expr` has no unary-expression rule. Users should not infer a comprehensive finite Boolean proof fragment from the language's syntax or its execution examples. |
| Universes | Explicit universe annotations support more checks; unresolved inferred relationships can remain incomplete. There is no general universe-constraint solver establishing every admissible higher-rank program. |
| Source export | Independent partial closures, opaque operations/packages, and some graphs combining a quantified composite with a separately exported method remain explicitly unsupported. Typed incompleteness is the correct result for these cases. |
| Effects/foreign interfaces | Syntax and capability information exist, but an `extern` declaration does not supply an implementation; universal effect/foreign proofs are not provided. The complete checked execution model in the proposal is not implemented. |

These limitations mean the checkout is not an implementation of every facility in the proposal. The implementation guide is largely candid about that. The central correctness issue is that incomplete support must not turn into a false proof, a false conflict, or silent loss of a clause; findings 1–4 show exactly those transitions.

**Recommended follow-up**

Fix the two proof/inference failures first, then the public overloaded-adapter and concrete-identity failures. Address record transport and scope/export preservation alongside them. Add regressions for compositions of features, particularly:

- saved arguments whose own contracts are valid but whose required contracts are not;
- implicit versus explicit instances, and failed guesses versus refuted whole guards;
- entire interface values versus individual head signatures during transport;
- the same concrete closure under additional proved contracts at every equality observation;
- ordinary, hidden, optional, definition, and pattern fields through each transport path;
- fresh-context export/reimport, followed by calls, type selections, equality, and later refinements.

The specification's finite reference-model checks are described but are not included in this checkout. Such checks could strengthen assurance about the retained-state laws, especially branch exclusion and approximation versus exact residuals. They would supplement the production tests; they would not prove the implementation correct by themselves.


**Resolution and verification — 24 September 2026**

The ten original executable CUE examples above were rerun through the repaired
`cue` CLI. The valid cases now export their expected values; the saved-callback
case remains unproved, the guarded result conflict is retained, and both abstract
escape cases are rejected. The eleventh CUE block is the old malformed export,
not an input example. API regressions now reject independent export of a closure
whose implementation retains an opened type name, in both source and final modes.

| Finding | Repair | Regression coverage |
| --- | --- | --- |
| 1. Saved partial arguments | Prove each saved value's membership in its required domain, independently of the original universal body proof. | [Saved-packet API matrix](../cue/certification_partial_test.go) |
| 2. Inference and guarded clauses | Track variance, lower/upper constraints, and telescope dependencies; verify the selected witness; refute all instances only from an independent necessary condition. | [Variance and bounds](../cue/testdata/quantified/inference/variance_bounds.txtar), [guarded instances](../cue/testdata/quantified/inference/guarded_instances.txtar) |
| 3. Overloaded adapters | Preserve complete interfaces, select admitted transport views, retain every result guard, and prove agreement on overlapping domains. | [Operations](../cue/testdata/quantified/opaque_composite_transport/overloaded_operations.txtar), [abstract carriers](../cue/testdata/quantified/opaque_composite_transport/overloaded_abstract.txtar), [callbacks](../cue/testdata/quantified/opaque_composite_transport/overloaded_callbacks.txtar), [generic clauses](../cue/testdata/quantified/opaque_composite_transport/overloaded_generic.txtar), [protocols](../cue/testdata/quantified/opaque_composite_transport/overloaded_protocols.txtar) |
| 4. Runtime identity | Separate three-valued inhabitant identity from constraint-graph equality, retain both opaque representation graphs, and validate their obligations. | [Singletons](../cue/testdata/quantified/closure_identity/singleton_contracts.txtar), [recursive comparisons](../cue/testdata/quantified/closure_identity/recursive_singletons.txtar), [opaque identity](../cue/testdata/quantified/opaque/runtime_identity.txtar) |
| 5. Hidden fields | Preserve ordinary hidden and definition fields, with their package-qualified labels, while maintaining root module projection. | [Hidden record transport](../cue/testdata/quantified/opaque_composite_transport/hidden_records.txtar) |
| 6. Escape and export | Traverse witness predicates, attached function/builtin clauses, and telescope bounds; retain erased opened names as lexical export dependencies. | [Retained predicates](../cue/testdata/quantified/opaque_closure_escape/retained_predicates.txtar), `TestQuantifiedOpaquePredicateExport` in [API tests](../cue/quantified_test.go) |
| Elementary unary proof gap | Prove Boolean negation and numeric signs, including exact results and numeric interval reversal. | [Unary certification](../cue/certification_unary_test.go) |

These repairs do not implement the entire proposal. The documented boundaries
on dependent profile D, general quantified proof search, universe solving,
effects, and independent opaque export remain. Some valid generic transports,
overlaps without a proved common transport, and partial overloaded operations
with different packet rows remain incomplete. The implementation guide describes
these limits; the regressions distinguish incompleteness from contradiction and
from successful certification.

Post-repair verification passed:

- `GOCACHE=/tmp/cue-audit-go-cache go test ./...`: 121 packages reported `ok`.
- `GOCACHE=/tmp/cue-audit-go-cache go test -race ./cue ./internal/core/adt ./internal/core/subsume`: all three packages passed, with no race reports.
- All ten executable baseline examples passed their repaired CLI expectations.
- Documentation links, whitespace checks, and the requested commit-message
  conventions were checked. Every repair commit has the required
  `Generated-by: gpt-6-astra` trailer, no sign-off, and lines no longer than
  75 columns.
