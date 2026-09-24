**Quantified CUE audit — revision `64815d1e625d`, 24 September 2026**

This report records the original revision. The subsequent repairs, broader
boundary audit, and verification are documented in
[Quantified CUE boundary architecture](quantified-cue-boundaries.md).

The implementation does **not** faithfully implement the whole proposal. It
implements a substantial, explicitly restricted fragment, but there are also
correctness defects within that fragment. In particular, it can refute a
satisfiable call, discard constraints during transport, expose erased
annotations through equality, and certify operations whose admitted calls do
not succeed.

This is a new audit of the current checkout against
[quantified-cue.tex](quantified-cue.tex). Earlier audit documents describe other
revisions; their repaired findings are not assumed to remain defects here.
The findings below were reproduced against a binary and Go API helper built
from this revision. No implementation code was changed.

Priority **P1** means a semantic correctness or certification defect requiring
attention before relying on the affected guarantee. **P2** means a supported
composition is unnecessarily left unproved. A failure to prove a genuinely
unsupported predicate is not, by itself, a soundness defect.

| # | Priority | Finding | Observed consequence |
| --- | --- | --- | --- |
| 1 | P1 | Abstract-call admission uses an enriched argument | A satisfiable program becomes bottom; supplying an implementation removes that conflict. |
| 2 | P1 | Structural equality compares retained function contracts | Erased type selections and redundant annotations change concrete Boolean results. |
| 3 | P1 | Ordinary record transport drops latent constraints | A certified identity operation loses optional constraints and closedness. |
| 4 | P1 | Package-opening proofs assume an executable witness | A certified client cannot execute on a record accepted by its existential parameter. |
| 5 | P1 | Sealing ignores value-binder sorts | An out-of-range witness is accepted and the resulting package is certified. |
| 6 | P1 | List transport evaluates nested predicates in the wrong scope | A certified operation rejects its valid result instead of wrapping it. |
| 7 | P1 | Inward record transport leaves definitions in the public scope | A certified operation rejects an admitted argument. |
| 8 | P2 | Call certification only uses an ordinary callback's head clause | Reordering an intersection changes whether the caller is certified. |

All snippets below use `@experiment(quantified)`. Add that attribute when
running them independently. Results distinguish `Validate()` from
`Validate(cue.Concrete(true))`; exporting `out` alone does not establish that
every other declaration is certified.

**1. Abstract calls turn compatible arguments into proved domain membership**

Primary locations:
[abstract.go:65](../internal/core/adt/abstract.go#L65),
[expr.go:2053](../internal/core/adt/expr.go#L2053), and
[expr.go:2610](../internal/core/adt/expr.go#L2610).

```cue
f: func({a: 1}) -> string
out: f({}) & 0
```

Actual: compilation and ordinary validation report conflicting values `0` and
`string`.

Expected: an unresolved call, not a contradiction. This contract constrains
calls whose record argument contains `a: 1`. It does not establish that every
call on `{}` returns a string. There are implementations of the contract that
accept a wider domain and return `0` on the empty record. For example, append:

```cue
f: func(x: {}) -> (int | string): {
    if len(x) == 0 {r: 0}
    if len(x) > 0  {r: "ok"}
}.r
```

The combined program now has no ordinary validation error, and `out` exports
`0`. This implementation satisfies the original contract: every record with
`a: 1` is nonempty and therefore returns `"ok"`. Full concrete validation leaves
the function's conditional-body proof incomplete, which is a separate,
documented checker limitation. The important observation is that the first
program was already satisfiable and should not have been refuted.

`abstractCall` tests a clause by constructing a temporary executable function
and calling it. The ordinary call path conjoins the parameter predicate into
the activation. Thus `{}` becomes compatible with `{a: 1}` by acquiring the
field. Success of that calculation is treated as evidence that the clause
applies to the original argument, and its result predicate becomes an
unconditional constraint on the abstract result.

The same enrichment is directly visible in
`(func(x: {a: 1}) -> int: len(x))({})`, which exports `1`. Propagating constraints
into existential call cells can be legitimate under the proposal. It does not
justify treating that particular refinement as the only possible behavior of
an unspecified implementation. Abstract application must retain the other
possibilities.

This violates the successful-call interpretation and the distinction between
membership, compatibility, and guarded consequences in the specification's
chapters on functions, checking, and refinement-stable normalization. It also
violates the requirement that adding a valid implementation cannot repair an
established contradiction.

Repair direction: use the original packet and the three-valued membership
judgment to determine applicability. The probe must not use its enriched
activation as membership evidence. Unknown admission must leave a guard or
residual call. Add tests comparing bodyless contracts with subsequently
supplied implementations, including missing fields, optional fields, and list
elements.

**2. Equality still makes erased annotations observable inside containers**

Primary locations:
[binop.go:112](../internal/core/adt/binop.go#L112),
[equality.go:239](../internal/core/adt/equality.go#L239), and
[equality.go:244](../internal/core/adt/equality.go#L244).

```cue
base: func(x: int) -> int: x
out: [base] == [base & (func(int) -> int)]
```

Actual: full concrete validation succeeds and `out` exports `false`.
Expected: `true`. Both operands contain the same implementation; the additional
contract is valid and redundant.

The same problem makes erased type selection observable:

```cue
id(A): func(x: A) -> A: x
out: [id[int]] == [id[string]] // actual: false
```

It also affects builtins: comparing `[strings.ToUpper]` with
`[strings.ToUpper & (func(string) -> string)]` produces `false` after successful
concrete validation.

There is a particularly clear inconsistency for opaque values:

```cue
#M: exists A {left: A, right: A}
base: func(x: int) -> int: x
p: seal #M with (A = {f: func(int) -> int}) {
    left:  {f: base}
    right: {f: base & (func(int) -> int)}
}
out: (open p as (A, P) {
    result: {
        direct: P.left == P.right
        nested: [P.left] == [P.right]
    }
}).result
```

Actual: `{"direct": true, "nested": false}`; full concrete validation succeeds.
Equality is not preserved by the list constructor.

Direct opaque equality uses `runtimeValueIdentity`. List equality and enabled
struct equality still call `Equal`, whose function and builtin cases compare
retained contract graphs. Its opaque case also recursively uses `Equal` on
the representation. The previous separation of operational identity from
constraint-graph equality has therefore not reached every observation.

This violates the specification's closure-identity and erasure rules
(including its sections on implementation preservation and captured values),
and the componentwise equality preservation required for abstract modules.

Repair direction: use operational identity for callable and opaque leaves of
runtime structural equality, retaining the host language's intended field
visibility and numeric-comparison policies. Keep graph equality separate for
deduplication. Test `==`, `!=`, nested lists/records, builtin values, selected
views, and redundant contracts. Do not merely change the direct opaque case.

**3. Opaque identity operations discard optional constraints and closedness**

Primary locations:
[package.go:551](../internal/core/adt/package.go#L551),
[package.go:563](../internal/core/adt/package.go#L563), and
[package.go:694](../internal/core/adt/package.go#L694).

```cue
#M: exists A {id: func({}) -> {}}
p: seal #M with (A = int) {
    id: func(x: {}) -> {}: x
}
out: (open p as (A, P) {
    r: P.id({x?: int}) & {x: "bad"}
}).r
```

Actual: full concrete validation succeeds and `out` exports `{"x":"bad"}`.
The equivalent direct call

```cue
out: (func(x: {}) -> {}: x)({x?: int}) & {x: "bad"}
```

correctly conflicts. Replacing the argument by `close({})` yields the same
discrepancy: the sealed identity permits the added field, whereas the direct
identity reports `field not allowed`.

There is no abstract occurrence in this operation's input or output. Transport
should preserve the caller's record and its retained constraints. Instead,
both record paths rebuild a `StructLit` from materialized members. They skip
absent optional fields and do not retain the original record's closedness or
whole constraint graph. The repaired preservation of hidden member fields
does not address these latent predicates.

This is not the permitted projection of private fields at a module's root.
The affected values are ordinary arguments and results of a public operation.
It violates exact residual preservation, the host field-presence/closedness
rules, and the implementation guide's distinction between root projection
and ordinary record transport.

Repair direction: transport the retained record predicate as well as its
present fields, translating abstract occurrences in their original scopes.
An ordinary record with no abstract occurrences can use identity transport.
Add round-trip refinement tests for optional fields, patterns, required fields,
definitions, closedness, and nested containers. Check later refinements, not
only the immediate JSON result.

**4. The checker certifies opening existential values that runtime cannot open**

Primary locations:
[opaque_proof.go:21](../internal/core/adt/opaque_proof.go#L21),
[certify.go:464](../internal/core/subsume/certify.go#L464), and
[package.go:924](../internal/core/adt/package.go#L924).

```cue
#M: exists A {x: A}
use: func(p: #M) -> int:
    (open p as (A, P) {result: 0}).result
```

Actual: the whole declaration passes concrete validation. But adding

```cue
out: use({x: 1})
```

makes the call incomplete with `package witness is not available for opening`.
This argument belongs to the parameter description: `#M & {x: 1}` itself passes
concrete validation, and a client with the same parameter that simply returns
`0` accepts it. Supplying a sealed instance instead also makes `use` succeed.

The proof rule extracts an existential interface from the parameter predicate
and creates a fresh abstract view. Runtime `PackageOpen.evaluate`, however,
requires a vertex with a `sealedPackage`. Covariant transparent existential
membership accepts ordinary records without constructing that executable
witness. The proof establishes an elimination operation that the evaluator
does not provide for all admitted inputs.

The restriction to opening sealed packages is documented. The defect is
certifying an arrow over a larger domain despite that restriction. Under the
successful-arrow definition, a certified function must succeed on every
admitted packet; an unresolved opening does not satisfy that promise.

Repair direction: make existential evidence available to runtime elimination,
or distinguish the executable sealed-package domain from a transparent
existential predicate and require the appropriate evidence in the proof rule.
Until then, leave this client unproved. Test a certified consumer with every
representation that the parameter's membership checker accepts.

**5. Sealing accepts value witnesses outside their declared range**

Primary location:
[package.go:350](../internal/core/adt/package.go#L350); related representation:
[quantified.go:300](../internal/core/adt/quantified.go#L300).

```cue
#M: exists (n in 1 | 2, A) {value: n}
p: seal #M with (n = 7, A = string) {value: 7}
```

Actual: compilation, ordinary validation, and full concrete validation all
succeed. Using `n = int` instead of `n = 7` also succeeds with `value: 7`.

Expected: reject the unsupported sealing form or retain an unresolved
obligation; it cannot certify this witness. `n` is value-sorted and must range
over `1 | 2`. It is not an unbounded representation type.

Mixed value/type prefixes are retained as an `Existential`, which lets sealing
reach the template. The seal loop rejects nonnil subtype bounds and checks
type universes, but never examines `TypeParameter.ValueRange`. It then creates
an opaque type carrier for every parameter and marks the existential as
discharged using those assignments. The value sort and its range have been
lost. The unused type parameter `A` is what keeps this particular existential
on the residual path; the value-only spelling takes a different normalization
path and is rejected by sealing.

This violates both sort-correct formation and existential introduction's
requirement that the supplied witness belong to its declared sort. The current
opaque profile explicitly permits unbounded representation types, so rejecting
value binders here would be consistent with its stated scope.

Repair direction: check binder sort before constructing carriers. Either
support value witnesses with proper range/evidence handling or reject them
explicitly. Cover mixed prefixes, out-of-range literals, predicate-valued
witnesses, and alternate binder orders. This is an invalid acceptance, not a
request for general dependent-profile support.

**6. Certified list transport loses the lexical scope of compound predicates**

Primary location:
[package.go:573](../internal/core/adt/package.go#L573), especially line 591;
also the unchecked evaluation at
[package.go:598](../internal/core/adt/package.go#L598).

```cue
#M: exists A {
    f:    func() -> [...(A | null)]
    read: func(A) -> int
}
p: seal #M with (A = int) {
    f:    func() -> [...int]: [7]
    read: func(x: int) -> int: x
}
```

Actual: the package passes full concrete validation. Adding

```cue
out: (open p as (A, P) {r: P.read(P.f()[0])}).r
```

fails with conflicts between `7` and `null`, and between `7` and the opaque
type. Expected: `7` after wrapping the integer as an abstract element and
unwrapping it through `read`.

Controls: the scalar result `A | null` works; a direct list element `A` works.
The failure also occurs for a fixed list `[(A | null)]` and for the union
inside a record element. These source branches have disjoint private kinds,
so this is not the documented limitation on ambiguous overlapping transports.

The AST list path descends into its elements using the enclosing environment,
without installing the list scope assumed by compiled reference offsets.
Direct `TypeReference` transport happens to bypass this problem by looking up
the parameter identity. A compound predicate is evaluated instead, and gets
the wrong environment. The fallback also ignores the evaluation-completeness
flag and can return the untransported private value. The public result check
then sees an integer where an opaque value is required.

The proof path evaluates the complete schema correctly and certifies the
adapter, so the mismatch also invalidates the advertised transport-totality
guarantee. With overlapping `A | int`, a related list call can export the plain
integer even though whole-package certification stays incomplete; unsupported
transport should not silently fall through to identity.

Repair direction: use the list's lexical environment, or consistently perform
transport through the resolved schema. Preserve evaluation failures and
incompleteness rather than ignoring them. Exercise compound predicates under
every constructor in both transport directions, with a positive assertion that
each certified operation executes successfully on admitted concrete inputs.

**7. Definitions in transported arguments retain the public carrier**

Primary locations:
[package.go:540](../internal/core/adt/package.go#L540) and
[package.go:688](../internal/core/adt/package.go#L688).

```cue
#M: exists A {
    zero: A
    f: func({#T: A, value: A}) -> int
}
p: seal #M with (A = int) {
    zero: 7
    f: func(x: {#T: int, value: int}) -> int: x.value
}
```

Actual: the package passes concrete validation. But this admitted call fails:

```cue
out: (open p as (A, P) {
    r: P.f({#T: A, value: P.zero})
}).r
```

The error is `x.#T: conflicting values int and opaque type`. Expected: `7`.

Both definition-field branches copy the schema's public definition unchanged.
They do not account for transport direction. The ordinary `value` field is
unwrapped to its private integer, but `#T` still denotes the public abstract
carrier when the private implementation checks its argument.

Definitions are predicates rather than runtime data, which explains why they
should not be made concrete. Their free abstract occurrences still need the
correct public/private interpretation. The totality proof currently accepts
this case despite the evaluator's incompatible transformation.

Repair direction: transport definition predicates in the appropriate direction
while retaining their scope, closedness and other constraints. Test declared
definitions containing the carrier, not only extra definitions with ordinary
data predicates. Include definitions inside nested records and lists.

**8. Ordinary overloaded callback proofs depend on intersection order**

Primary location:
[certify.go:733](../internal/core/subsume/certify.go#L733), through line 771.

```cue
f: func(g: (func(int) -> int) & (func(string) -> string)) -> string:
    g("x")
id: func(x: _) -> _: x
out: f(id)
```

Actual: `out` exports `"x"`, but full concrete validation reports
`f: function conformance remains unproved`. Swapping the two arrow clauses in
`g` makes the entire program pass concrete validation.

The call-proof code initializes its candidate sources from
`f.ResidualSignature()` only. It enumerates additional alternatives specially
for `OpaqueCall`, but omits `FuncValue.Types` for an ordinary callback. The
string capability is consequently unavailable to the body proof when it is
the second conjunct.

Both orders denote the same intersection. This is a completeness defect in an
elementary structural case, rather than a false certificate: the conservative
outcome is safe, but the claimed compositional support for arrow intersections
is materially weaker than expected. A callback that uses both domains will
need both clauses regardless of their order.

Repair direction: retain all applicable ordinary callback clauses during
proof-level application, including their environments, remaining selections,
and partial-application interpretation. Test both conjunction orders and bodies
that exercise every clause.

**Coverage of the proposal and remaining implementation limits**

The code has real implementations of lexical quantified syntax, aliases,
explicit and inferred type application, retained universal obligations,
intensional closures, packet protocols, partial application, finite value
quantification, predicative checks, sealed carriers, and structural
certification. The new defects do not make that work merely syntactic.
However, the following boundaries prevent describing this checkout as a full
implementation of the document:

| Area | Current support and consequence |
| --- | --- |
| General dependent profile D | The compiler rejects general value ranges and parameter-dependent signatures. The finite literal exception does not implement dependent vectors, matrices, arithmetic module laws, or request-indexed APIs. |
| Existential elimination | Runtime opening requires sealed, record-shaped packages with one carrier. Transparent existential elimination, multi-carrier opening, and scalar package opening are absent. Finding 4 shows that certification does not consistently respect this boundary. |
| Existential solving | Covariant data membership and reuse of supplied sealed witnesses are supported; general witness synthesis and arbitrary mixed-prefix reasoning remain unresolved. Finding 5 is an incorrect escape from that residual boundary. |
| Quantified Boolean predicates | Universals outside the distributive/structural fragment are retained as residual validators. There is no general semantic-subtyping procedure for arbitrary quantified Boolean placement. |
| Function proofs | Optional-presence cases, general conditionals, many arithmetic implications, most builtins, and general recursive termination remain unproved. Ground execution succeeding is not a universal proof. Finding 8 additionally exposes a small, order-sensitive gap in the structural fragment. |
| Universes | Explicit levels, level computation and occurs checks exist. Unknown inferred relationships can remain incomplete; there is no general universe-constraint solver. |
| Effects and foreign execution | Effects and `extern` syntax are retained, but implementation certification for these functions is unsupported. An `extern` declaration does not provide a foreign implementation or the complete checked bridge described in the proposal. |
| Opaque transport | Generic, recursive and overlapping transports have documented residual cases. Findings 3, 6 and 7 are stronger failures: information is lost or certification succeeds while ordinary execution fails. |
| Source export | Supported closures, captures, aliases and selected subjects have substantial preservation machinery. Independent partial closures, opaque values/operations/packages, and some shared composite/method graphs deliberately report incomplete export. The proposal's general round-trip obligation is therefore only partially implemented. |
| Representation-independent evolution | The code provides mechanisms intended to support the theorem's hypotheses. It does not construct a general equality-respecting simulation between library versions. Findings 2 and 3 already violate observations that such a simulation must preserve. |

Most of these limits are candidly documented in
[the implementation guide](quantified-cue-implementation.md). The proposal
allows staged profiles and incomplete proof search, so they should be reported
as scope limitations rather than indiscriminately counted as correctness bugs.
Unsupported cases must nevertheless retain their exact meaning and must not
acquire unjustified certificates or contradictions.

There is also a documentation discrepancy: the proposal's finite-regression
appendix still says this checkout contains no reference-model scripts or
results. The checkout now contains independent model tests and
[an oracle guide](quantified-cue-oracles.md). Those tests cover useful finite
fragments; they are not a complete execution of every reference-model family
listed in the appendix.

**Verification and limits of this audit**

The starting tree was clean at revision `64815d1e625d`. The audit examined the
proposal and implementation guide, parser/compiler binding paths, quantifier
normalization, universe checks, inference, call/membership handling, identity,
the conformance checker, package transport, export paths, and their regression
coverage. Findings were checked with focused standalone inputs and controls.

Verification used Go `go1.27.1 darwin/arm64`:

- `go test ./cue ./cue/parser ./cue/format ./cue/ast/... ./internal/core/...`
  passed.
- `go test ./...` passed with **121 packages reporting `ok`**. The initial
  sandboxed run could not start some localhost test servers; the permitted
  rerun with local networking succeeded.
- The audit helper separately checked compilation errors, ordinary validation,
  concrete validation, and JSON export of `out`. CLI checks confirmed the list
  transport diagnostics. Tests were run against the unchanged implementation.
- The extended oracle/fuzz mode was not run. The normal suite includes its
  default finite models. Passing that suite does not cover the new combinations
  demonstrated above and is not a proof of the unrestricted calculus.

To reproduce API observations, compile a snippet with
`cuecontext.New().CompileString("@experiment(quantified)\n" + source)`, call
`Validate()` and `Validate(cue.Concrete(true))`, and inspect
`LookupPath(cue.ParsePath("out")).MarshalJSON()`. For CLI reproduction, build
`./cmd/qcue` and run the files outside this repository's older pinned CUE
module, or in a separate module using language version `v0.18.0`.

The main testing gap is composition across semantic boundaries. Existing
record-membership checks do not establish correct abstract-call admission;
direct opaque equality does not establish equality under constructors;
transporting present fields does not establish residual preservation; and a
proof over a resolved interface does not establish that its execution adapter
uses the same scopes and predicates. Regression additions should assert these
relationships directly, with invalid controls and later refinements.

The first repair priorities are false refutation, erased identity, lost
constraints, and certification/runtime agreement. Extending the dependent
profile or adding more proof search should follow those corrections.
