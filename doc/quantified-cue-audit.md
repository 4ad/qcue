**Quantified CUE implementation audit — 23 September 2026**

Audited revision: `d7385e8cbc9f`. Specification: [quantified-cue.tex](../doc/quantified-cue.tex). The findings below describe that baseline. All eleven reproduced defects have since been repaired; the resolution and regression index at the end records the work.

**Verdict: substantial parts are implemented, but the implementation does not faithfully enforce the specification, even within its advertised fragment.** The problems are not limited to unsupported dependent types or incomplete proof search. Reproductions below show invalid implementations being certified, valid calls being contradicted, required constraints disappearing on export, and inconsistent treatment of seal identity, universes, declaration order, and witness dependencies.

The implementation guide accurately acknowledges many limitations. Those limitations are separated below from defects that violate the supported semantics. In particular, an unresolved result is not itself a bug: the specification expressly permits sound, incomplete checking.

**Validation and reproduction setup**

I read the proposal's semantics, surface forms, profiles, examples, and checking obligations; traced the compiler, quantified evaluator, capability machinery, closure identity, universe checks, package boundaries, certifier, subsumption, and source exporter; and exercised additional small programs through the CLI and public Go API.

Both the focused core-package tests and `go test ./...` passed. The initial sandboxed full-suite run failed where tests needed local listening sockets; rerunning with those permissions completed successfully. The defects below therefore escape the existing test suite. The performance reproduction was deliberately terminated after four seconds; I did not attempt an out-of-memory test.

Build the CLI with `go build -o /tmp/qcue-audit ./cmd/qcue`. Run reproductions from a temporary directory outside this checkout's older-version CUE module. The following snippets assume quantified mode; adding `@experiment(quantified)` makes that explicit. An older module's language-version gate still applies even with the attribute.

For implementation certification, run `/tmp/qcue-audit vet -c case.cue`. For an individual result, use `/tmp/qcue-audit eval -e out case.cue` or the public API:

```go
v := cuecontext.New().CompileString(source)
x := v.LookupPath(cue.ParsePath("out"))
err := x.Validate(cue.Concrete(true))
data, exportErr := x.MarshalJSON()
```

P1 below means a correctness defect that should be fixed before relying on the affected semantic guarantee. P2 means a significant export or resource-management defect.

**1. [P1] Higher-order capability guards assume the function membership they need to prove**

Location: [capability.go:109](../internal/core/adt/capability.go), especially the validation call at line 135.

```cue
g: func(x: int) -> int: {out: x}.out
f: func(cb: func(int) -> _) -> _: cb(0)
f: func(func(int) -> string) -> string
out: f(g)
```

Expected: `out` is `0`, or the checker conservatively retains an unresolved applicability obligation. The additional clause applies only to callbacks that return strings for every integer. `g` is not such a callback. Both contracts on `f` are valid: applying a callback to zero preserves its result guarantee.

Actual: evaluation reports `conflicting values string and 0`. Removing `out` makes the entire program pass `vet -c`, including certification of both functions. Changing `g`'s body from `{out: x}.out` to the equivalent `x` also makes the call work. Thus whether an inapplicable guard produces a contradiction depends on the counterexample search's syntactic vocabulary.

`capabilityMember` conjoins the candidate with the proposed domain constraint, then runs `Validate` with `Concrete` and `Final`, but without `CheckFunction` or `CheckBuiltin`. A complete closure descriptor with a newly attached, unproved contract is accepted as a member. This mistakes compatibility with a retained obligation for proof of that obligation.

The same issue reproduces with callbacks nested in records and with `strings.ToUpper`: an attached `func(func(string) -> int) -> int` clause incorrectly narrows the actual result `"X"` to `int`.

This violates the guarded-clause rules and the prohibition on using a target obligation as its own evidence. Membership needs independent callable conformance checking, recursively through composites; failure to prove it must remain unknown rather than applying the clause unconditionally.

**2. [P1] Calls of partial closures can be certified against the wrong protocol**

Location: [certify.go:489](../internal/core/subsume/certify.go). The remaining protocol already exists as [ResidualSignature](../internal/core/adt/capability.go).

```cue
f: func(x: int, y: int) -> int: y
p: f(1, ...)
g: func(x: int, y: int) -> int: p(x, y)
```

Actual: `vet -c` succeeds. Adding `out: g(2, 3)` fails with `too many positional arguments in function call`.

`p` has one remaining argument, but `certifier.call` constructs its source type using `f.Fn`, the original two-argument signature. It does not use the saved partial bindings to obtain the callable protocol. It consequently certifies a function that rejects every packet in its declared nonempty domain. Conversely, a wrapper making the valid call `p(x)` remains unproved.

The same problem occurs with labeled calls that try to supply a previously bound slot. The body checker must use the residual protocol and preserve the original implementation obligations separately. This is distinct from the existing checks for attaching a residual contract to a partial closure.

**3. [P1] The special proof rule for `len` ignores packet labels**

Location: [certify.go:462](../internal/core/subsume/certify.go).

```cue
f: func(x: string) -> int: len(wrong: x)
```

Actual: `vet -c` succeeds. Adding `out: f("abc")` fails with `labeled arguments are not supported for builtin len: it declares no parameter names`.

The proof rule checks the builtin name, argument count, and argument kind, then returns `int`. It never validates `ArgLabels` against the builtin's actual protocol. The result-kind rule is sound only after complete packet coverage is established. This directly violates successful-arrow soundness, independently of partial application.

**4. [P1] Exporting a closure weakens captured schema predicates**

Location: [closure.go:56](../internal/core/export/closure.go), particularly the `e.value(v)` call at line 76.

```cue
#T: {a: int}
f: func(x: #T) -> int: x.a
out: f({a: 1})
```

The original program passes concrete validation. Original `f({a: 1, b: 2})` correctly fails with `field not allowed`. However, `qcue eval` exports the closure as:

```cue
f: CUECode({a: int})
CUECode(CUEType) = func(x: CUEType) -> int: x.a
```

After recompilation, `f({a: 1, b: 2})` returns `1`. The definition's closedness has disappeared. Public `f.Syntax(cue.Final())` exhibits the same weakening.

Optional constraints are also lost. With `#T: {a?: int}` and a constant-returning implementation, evaluated export passes `{}` to the factory. The original rejects `f({a: "bad"})`; the rebuilt function returns successfully.

The exporter serializes an erased predicate dependency through its ordinary value-output settings. Those settings may omit optional fields and definition closedness, which is appropriate for some data observations but not for a predicate controlling future calls. This breaks equivalence under later refinement and the proposal's preservation of record constraints.

Predicate dependencies need a schema-preserving representation independent of data-output options, including closedness, optional/required fields, patterns, and relevant residual predicates. If that representation cannot be produced, export must remain incomplete.

**5. [P1] Structural equality forgets seal identity**

Locations: [equality.go:69](../internal/core/adt/equality.go), [witness.go:72](../internal/core/adt/witness.go), and [closure.go:71](../internal/core/adt/closure.go).

```cue
#M: exists A {tag: 1}
a: seal #M with (A = int) {tag: 1}
b: seal #M with (A = int) {tag: 1}
f: func(x: a) -> int: 1
out: f(b)
```

Actual: concrete validation succeeds and `out` exports as `1`. In contrast, `a & b` correctly reports `conflicting opaque package identities`.

The signature's ordinary reference `a` must denote that particular package's singleton. `b` has a different seal. `WitnessType.validate` uses `Equal`, whose record comparison checks fields and base values but does not compare the vertices' `sealed` identities. Visible equality therefore substitutes for package identity.

This also affects closures capturing packages:

```cue
make: func(p: _) -> (func() -> _): func() -> _: p
fa: make(a)
fb: make(b)
same: fa & fb
```

`same` passes concrete validation although its closures capture different generative packages. Both failures disappear only accidentally when visible fields contain other identity-distinguishing values. Seal identity must participate consistently in concrete equality, singleton membership, and runtime capture comparison, including packages with only ordinary public data.

**6. [P1] Attached universal clauses bypass universe levels and the occurs check**

Locations: [universe.go:59](../internal/core/adt/universe.go) and [universe.go:214](../internal/core/adt/universe.go).

```cue
f: func(x: _) -> _: x
f: forall (A in Type(0)) func(A) -> A
out: f[f](f)
```

Actual: `vet -c` succeeds. The equivalent direct generic implementation, `f: forall (A in Type(0)) func(x: A) -> A: x`, correctly rejects its own use as a type argument. The attached-clause form bypasses the predicative restriction.

A separate level reproduction is:

```cue
f: func(x: _) -> _: x
f: forall (A in Type(1)) func(A) -> A
Box(A in Type(1)) = {value: A}
out: Box(f)
```

This succeeds, whereas the directly quantified version is rejected as universe level 2 exceeding `Type(1)`.

`universeOf` raises the level for binders in `v.Env`, but for `v.Types` examines only each function's parameter/result expressions. It omits the strict increase for the attached clause's telescope. `universeOccurs` likewise traverses the main environment and selected arguments, but not the attached clauses' environments.

Every retained clause and its binder dependencies must participate in both checks. A scheme's formation discipline cannot depend on whether it came from the implementation declaration or another conjunct.

**7. [P1] Type selection on a composite subject depends on declaration order**

Location: [subject.go:24](../internal/core/adt/subject.go).

```cue
a: forall (A: int) {f: func(x: A) -> A: x}
a: forall (B: string) {g: func(x: B) -> B: x}
out: a[string].g("s")
```

Actual: the program fails because `string` does not satisfy the bound of `A`. Swapping the first two declarations makes it pass concrete validation and produce `"s"`.

`instantiateSubject` visits `subject.schemes` and immediately returns either the first failure or the first selected view. It never considers another retained universal clause. This makes conjunction observably noncommutative. The function-selection implementation already considers alternative retained clauses; composite selection does not.

The selector must resolve its applicable clauses without source-order dependence, retain every original obligation, and distinguish an excluded instance from an unresolved admissibility check. This needs permutation tests for record/list subjects, including separately contributed clauses with different bounds.

**8. [P1] An ordinary witness loses its singleton dependency through an expression alias**

Locations: [compile.go:261](../internal/core/compile/compile.go) and [quantified.go:154](../internal/core/compile/quantified.go).

```cue
x: int
let Y = x & int
f: func(y: Y) -> string: "ok"
out: f(2)
```

Actual: both `f.Validate(cue.Concrete(true))` and `out.Validate(cue.Concrete(true))` succeed; `out.MarshalJSON()` returns `"ok"`. The root itself remains incomplete because `x` is unresolved.

Inlining the alias as `func(y: x & int)` retains the singleton witness and blocks the result while `x` is unknown. Refining `x` to `1` makes the alias-based call conflict, but its prior complete observation and closure certification retained no demanded witness check. Exporting `f` at the unresolved stage even emits a factory applied to `int`, turning the current approximation into a permanent input predicate.

Aliases are compiled with `typePosition = false`. At a type-position use, `ordinaryWitnessReference` follows a `LetReference` only if the alias body is itself a recognized reference. It returns false for `x & int`, leaving the inner `x` as a raw field reference. This violates both capture-avoiding abbreviation semantics and the explicit requirement that ordinary witnesses denote eventual singletons rather than their current upper approximations.

Witness classification must survive arbitrary supported alias expressions, without making legitimate predicate aliases such as `let Nat = int & >=0` into runtime witnesses. Alias inlining should preserve validation, capture completeness, and export behavior.

**9. [P2] Exported implemented functions omit parametric aliases they still reference**

Locations: [closure.go:174](../internal/core/export/closure.go), [captures.go:49](../internal/core/compile/captures.go).

```cue
Box(A) = {value: A}
f(A): func(x: A) -> Box(A): {value: x}
out: f(1)
```

The program passes `vet -c`. Evaluated export contains:

```cue
f: CUECode
out: value: 1
let CUECode = forall (A) func(x: A) -> Box(A): {value: x}
```

`Box` is absent. Recompiling the emitted source fails with `reference "Box" not found`. The same issue occurs for aliases in a binder bound or implementation body.

The exporter copies the function's source, including alias applications, but does not emit or expand the alias declaration. Free-reference analysis follows an alias template's dependencies and arguments, without preserving the alias name required by the copied source. The nested-origin traversal explicitly stops at `AliasApplication`.

Export must either carry the lexical alias definition and its dependencies, or expand it capture-avoidantly while preserving implementation origins. Reporting success with unresolved source references is not a supported residual representation.

**10. [P2] Bodyless function contracts are exported without their lexical environment**

Locations: [compile.go:1369](../internal/core/compile/compile.go) and [value.go:538](../internal/core/export/value.go).

```cue
x: 1
f: func(x) -> int
```

Exporting only `f` with `f.Syntax(cue.Final())` produces `func(x) -> int`, without binding or substituting `x`. Rebuilding it independently fails. More seriously, if the destination already has an `x`, the contract can silently capture that different witness.

For example, a contract originally obtained from `r: {x: 1, f: func(x) -> int}` and attached to a top-level implementation is printed at top level as `func(x) -> int`. A separate top-level `x: 2` then changes its meaning on reimport.

The compiler populates `Captures` and `References` only when `fn.Body != nil`. The bodyless export path clones the original source and substitutes `Fn.Captures`, which is empty, plus selected type arguments. It does not reconstruct the lexical dependencies of the contract itself.

This is separate from missing parametric-alias declarations: it affects plain captured values and predicates, including attached residual contracts. Descriptions need lexical dependency preservation even when they have no executable body.

**11. [P2] Finite quantifier expansion has no resource bound and eagerly builds the Cartesian product**

Location: [quantified.go:219](../internal/core/adt/quantified.go), especially the recursive expansion at line 257.

```cue
out: exists (n0 in 0 | 1, n1 in 0 | 1, n2 in 0 | 1) 1
```

Extending this same expression to `n19` creates a 309-byte file including the experiment attribute, but evaluates the constant body up to `2^20` times. On this machine, 8 binders took approximately 0.011 seconds, 12 took 0.032 seconds, and 16 took 0.468 seconds. The 20-binder run exceeded four seconds and was terminated. These timings are observations, not portable performance thresholds.

The code recursively enumerates every combination and accumulates conjunction/disjunction values. There is no expansion budget, residual fallback, or elimination of unused binders. This contradicts the residual chapter's explicit policy of avoiding Cartesian expansion and bounding speculative instantiation separately from demanded ground evaluation.

This is an availability problem for small inputs, not an argument that all quantified solving should be fast. Lazy scoped residuals, safe vacuity elimination, and task-local expansion limits would avoid exhausting resources while preserving the predicate.

**Declared limitations and additional shortcomings**

The following are coverage gaps relative to the proposal, not examples of false certification. Most are acknowledged in [the implementation guide](../doc/quantified-cue-implementation.md).

| Proposal area | Implementation status and consequence |
| --- | --- |
| General dependent-value profile D | Unsupported. Nonliteral value ranges are rejected by [quantifiedTemplate](../internal/core/compile/quantified.go). Function parameter/result constraints are compiled outside the runtime parameter scope, so the proposed dependent signatures are not implemented. Finite literal enumeration is not general dependent quantification. |
| Arbitrary quantified Boolean descriptions | Retained, but [Universal.validate](../internal/core/adt/abstract.go) always returns incomplete. Even `forall A ((func(A) -> A) | (func() -> int))` with a constant integer-returning implementation cannot be discharged. This is sound incompleteness, but substantially less than general higher-rank checking. |
| Transparent existential witnesses | Membership covers covariant extrema and some reuse of sealed witnesses. There is no general witness synthesis or transparent witness-opening rule. Mixed quantified prefixes and noncovariant cases frequently remain residual. |
| Opaque package elimination | [PackageOpen](../internal/core/adt/package.go) opens record-shaped packages with one representation carrier. Scalar and multi-carrier openings remain incomplete. |
| Certification of module clients | Concrete execution of `open` inside a function works in supported cases, but `certifier.expr` has no `PackageOpen` rule. Executable examples such as a client taking `#Showable` do not thereby obtain the proposal's reusable proof under an arbitrary opened witness. |
| Effects and foreign interfaces | Effect tags participate in some comparisons, but [certifier.function](../internal/core/subsume/certify.go) rejects proof of effect-annotated or `extern` implementations. `extern` alone supplies no executable foreign implementation. The proposal's complete checked-outcome/bridge story is not realized. |
| Structural recursion | Finite-list descent permits certain ground executions. Universal termination/conformance proofs remain unsupported, so successful folds do not imply certification of their declared contracts. |
| Ordinary structural proof completeness | Optional parameter branches, most conditionals, several expression forms, and arithmetic refinements remain incomplete. Even obvious vacuity such as `forall (A: _|_) func(x: A) -> string: 1` is not discharged. Some of these are outside the stated checker fragment; they should not be presented as semantic contradictions. |
| Source export boundaries | Partial closures, opaque operations/packages, and unsupported captures intentionally report incomplete export. Those are legitimate limits; findings 4, 9, and 10 concern export paths that instead emit altered or unbound source. |
| Proof and resource architecture | The implementation uses local Boolean/three-way checks, retained ADT constraints, and bounded probes. It does not expose the independently checkable certificate graph and task-local resource interface described in the residual chapter. That is not by itself a proof of unsoundness, but the chapter's guarantees cannot simply be inherited from its reference algorithm. |

The proposal's appendix also says `checks/check_model.py`, `checks/check_residual.py`, and their JSON result files accompany the document and gives exact assertion counts. **Those files are absent from this revision.** Consequently those claims are not reproducible from the checkout. Even if restored, finite algebraic reference checks would supplement, rather than establish, correctness of the Go elaborator and evaluator.

**What is working, and what the tests establish**

This is a substantial implementation rather than only surface syntax. Existing regressions exercise lexical quantifier forms, ordinary generic calls, rigid-variable proofs for identity/map-like bodies, finite witnesses, retained callable clauses, several higher-rank examples, copied closure identity, partial application execution, ordinary seal/open transport, and many previously found refinement/export defects. The guide is appropriately explicit that it supports fragments of S_H and A rather than every proposal feature.

However, tests of successful calls and tests of successful certification answer different questions. The new failures occur primarily at intersections between individually covered features: callable guards and proof obligations; partial application and body checking; aliases and singleton decoding; composite subjects and multiple quantified clauses; predicate capture and data export; seals and generic equality. Existing tests passing therefore does not establish the four conformance contracts at the end of the proposal.

The most valuable next regressions would preserve behavior under alias inlining, record-body wrapping, declaration permutation, direct versus separately attached quantifiers, and source export/reimport. Certification tests should pair every newly accepted call rule with malformed packets and actual failing executions. Equality tests should compare direct unification with singleton admission and closure-capture identity for the same concrete values. Each negative test should distinguish contradiction from an allowed incomplete obligation.

The repair priority is to restore sound certification, guarded applicability, witness/seal identity, predicative formation, and faithful predicate export before expanding the supported proof fragment. The following repair work addresses these findings without treating unsupported proof search as a successful proof.

**Repair and regression index**

All eleven reproduced defects above have been fixed. The baseline reproductions
remain in this report to explain the failed guarantees; they do not describe the
current behavior.

| Finding | Repair and regression |
| --- | --- |
| 1 | Independent callable inclusion checks establish guards; unproved applicability stays pending. [Direct, nested, and builtin guards](../cue/testdata/quantified/capabilities/higher_order_guard_proof.txtar). |
| 2–3 | Body certification checks residual closure protocols and builtin packets through the shared capability rules. [Valid and invalid packet tests](../cue/certification_protocol_test.go). |
| 4 | Captured predicates export in schema mode, preserving closedness, optional/required fields, patterns, references, and defaults under ordinary and Final output. [Round-trip tests](../cue/lexical_export_test.go). |
| 5 | Value equality compares seal identity before public fields. [Singletons, closure captures, empty packages, and copied seals](../cue/testdata/quantified/opaque/seal_identity.txtar). |
| 6 | Formation and occurs checks traverse every retained clause environment, composite introduction, selected argument, and singleton wrapper. [Attached and nested universe tests](../cue/testdata/quantified/universes/attached_clauses.txtar). |
| 7 | Composite selection considers all admissible introductions and records which telescopes remain selectable. [Order, multiple matches, and chained selection](../cue/testdata/quantified/instantiation/composite_clause_order.txtar). |
| 8 | Predicate aliases have contextual decoding and separate evaluation caches; code origins and lexical binders remain canonical. [Refinement, compile-order, callable-alias, and iteration tests](../cue/alias_witness_test.go). |
| 9–10 | Implementations and bodyless contracts share lexical closure conversion. Alias declarations, bounds, and captured witnesses travel with exported source. [Alias chains, binder bounds, destination scope, and unresolved witnesses](../cue/lexical_export_test.go). |
| 11 | Finite expansion shares a bounded work budget with nested/deferred work, retains the full residual on exhaustion, and avoids products for independent literal bodies. [Products, nested expansion, constant bodies, residual export, and refinement](../cue/finite_expansion_test.go). |

The repairs also address related defects exposed by these regressions: unused
record arguments now validate nested singleton and required-field obligations;
finite existential residuals cannot use the extremal rule for type binders;
recompiling an alias cannot create a second code origin or conflate captures
from different comprehension iterations. The proposal's stale accompanying-file
and assertion-count claims have been corrected without introducing replacement
reference-model scripts.

The focused core/API tests and the complete `go test ./...` suite pass, covering
121 packages with tests. Compiler golden changes are limited to stable internal
let-label numbering. The documented unsupported proof fragments remain
incomplete; this work does not claim full implementation of profiles D or A,
general quantified decision procedures, or a certificate-graph checker.
