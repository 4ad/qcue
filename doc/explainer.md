# Universal types and functions in CUE

A universal type constrains one value at every type instance. For a function,
this lets an interface relate its inputs and results while preserving CUE's
ordinary refinements. The implementation can be passed to another function,
stored in a record, and constrained by declarations in several files.

This document explains the language in [version 9 of the paper](paper.pdf)
for experienced CUE users. The examples run in this fork, which enables the
feature by default. The [command guide](cmd/cue.md) explains how to run it.

## Reading a universal function

`func(x: A) -> B: e` declares a function with parameter `x`, input description
`A`, result description `B`, and body `e`. Its result description is a contract
that the body must establish. Omitting `: e` gives a function interface.
An interface describes the callable value to be supplied.

In `id(A): ...`, `A` is a type parameter: the **same** `id` must satisfy the
declaration for every choice of `A`. Explicit type arguments use square
brackets; ordinary calls use parentheses.

<!-- paper: 5 -->
```cue
id(A): func(x: A) -> A: x
integer: id(7)                 // 7
text:    id("seven")           // "seven"
record:  id({port: 443})        // {port: 443}
chosen:  id[int & >=0](7)       // 7
```

The body works because its argument already satisfies `A`. Selecting
`id[int & >=0]` records a particular interface of that same function. Calls
can also infer an instance from their arguments and required results.

Throughout this document, **certified** means that the checker has established
a contract. **Rejected** means that it has evidence against a stated
requirement. **Pending** means that a proof obligation still needs evidence;
**incomplete** means that demanded data is still missing. A call can also
fail with ordinary CUE bottom, `_|_`.

For example, a constant body cannot satisfy identity's universal contract:

```cue
wrong(A): func(x: A) -> A: 0    // rejected
```

Choose the singleton type `A = 1`. The call accepts `1` and successfully
returns `0`, which violates its result contract. Testing a universal claim
at a singleton is often the quickest way to understand it.

Examples below have their own scopes. Negative lines are individual checks;
run them separately from the successful cases. A later refinement is an
additional declaration conjoined with the example's original source.

## Descriptions and their completions

An ordinary CUE description specifies possible completed values. Write
`A ≤ B` when every value allowed by `A` is allowed by `B`: `A` refines `B`,
or, equivalently, `B` subsumes `A`. Thus `3 ≤ int ≤ number ≤ _`.

At the level of allowed values, `&` is intersection, `|` is union, `_|_` is
the empty set, and `_` admits every value. A literal is a singleton set.
Defaults additionally carry CUE's usual preference information. Record
descriptions additionally say which fields are allowed and which must be
present; their open or closed character remains significant.

Completing an ordinary field chooses a value satisfying its constraints.
Copies of a schema can choose different values:

<!-- paper: 4 -->
```cue
#Cell: {value: int}
left:  #Cell & {value: 1}
right: #Cell & {value: 2}
```

Declarations about the *same* field instead constrain a common completion.
References preserve the sharing defined by CUE's scope rules. Copying a
record rebinds its internal references to the copy; references to an outer
scope continue to name that outer scope. Consequently, the semantic object
for an open configuration is its set of **joint completions**: assignments
that satisfy all its connected fields together.

One can hide a local field after checking these joint constraints. In logical
terms, that asks whether *there exists* a completion of the hidden field.
Sharing must be established before hiding: choosing one `x` satisfying both
`P(x)` and `Q(x)` is a stronger requirement than choosing independent values
for the two predicates.

### One subject at every instance

A **semantic type** is a predicate on complete values, or equivalently the
set of values satisfying it. The extension adds function values to the
ordinary data values. Types can describe either. A universal type is the
intersection of its instance predicates, all applied to one subject.

`forall (A: U) T` means: for every semantic type `A` refining `U`, the subject
satisfies `T`. An omitted bound means `_`. If `T[A := B]` denotes replacement
of the bound type variable by `B`, the rule is:

```text
v satisfies forall (A: U) T
    exactly when v satisfies T[A := B] for every B ≤ U.
```

The subject `v` stays fixed as `B` varies. Every bound includes the empty
type among its subtypes. These facts explain all three declarations below:

```cue
empty(A): [...A]          // [] is the only possible value
impossible(A): A          // _|_
box(A): {value: A}        // _|_
```

At `A = _|_`, the list can have no elements; `[]` also satisfies every other
instance. `impossible` would have to be a value of the empty type. `box`
would need a present `value` field of that empty type.

A **parametric alias**, written with `=`, constructs a description by
substitution. Each use supplies its type arguments:

```cue
Box(A) = {value: A}
one: Box(int) & {value: 1}
```

Here `Box(int)` expands to `{value: int}`. The quantified declaration
`box(A): ...` above constrains one field at all instances; the alias defines
an expression abbreviation. This distinction determines whether a generic
schema creates useful instances or an impossible universal subject.

### Binder spellings and scope

The following declarations express the same universal interface. Their
callback parameter has type `func(A) -> B`; their list parameter and result
have element types `A` and `B` respectively.

<!-- paper: 2 -->
```cue
map(A, B): func(func(A) -> B, [...A]) -> [...B]
map: forall (A, B) func(func(A) -> B, [...A]) -> [...B]
map: func<A, B>(func(A) -> B, [...A]) -> [...B]
```

A single unbounded variable can be written `forall A`. A bound is written
`A: U`, and may use earlier type parameters. For example,
`forall (A: number, B: A) ...` gives `B ≤ A ≤ number` in its body.
Quantifiers extend as far right as their enclosing expression permits;
parentheses delimit a quantified argument or result.

A lexical `forall` prefix inside a record quantifies the record. These two
declarations have the same meaning:

<!-- paper: 3 -->
```cue
r: {
    forall A
    id: func(A) -> A
}
r: forall A {id: func(A) -> A}
```

Binders have lexical identity. Renaming a bound variable preserves meaning;
using the same spelling in a different scope creates a different binder.
Another file can constrain `r` while the binder's private scope is preserved.

Type parameters provide proof information and are erased from execution.
Function parameters bind the body; parameter and result descriptions are
formed in the enclosing scope of data references and type binders. For
example, a captured outer `Limit` can occur in a result description, while
a term parameter's name is available in the body.

### Predicting what a type parameter preserves

A repeated type parameter denotes a common predicate. Its arguments can be
different inhabitants of that predicate:

<!-- paper: 6 -->
```cue
pair(A: number): func(x: A, y: A) -> [A, A]: [x, y]
p: pair[int & >=1](2, 5)       // [2, 5]
q: pair(1, 2.5)                // [1, 2.5]
bad: pair[int](1, 2.5)         // rejected: 2.5 conflicts with int
swap(A, B): func(p: [A, B]) -> [B, A]: [p[1], p[0]]
s: swap([7, "seven"])          // ["seven", 7]
```

For `q`, the predicate `1 | 2.5` is one admissible choice of `A`. The bound
`number` allows both inhabitants. The independent variables in `swap`
preserve the two component descriptions separately.

A bound gives the operations available on arbitrary inhabitants. A numeric
bound permits comparison, so selecting one of the inputs preserves `A`:

<!-- paper: 7 -->
```cue
min(A: number): func(x: A, y: A) -> A: {
    if x <= y {out: x}
    if x > y  {out: y}
}.out
m: min[int & >=10](12, 20)      // 12
same(A: number): func(x: A) -> A: x + 1 // rejected
```

Every branch of `min` returns an existing `A`. In `same`, addition establishes
a numeric result, but the required result is the particular subtype `A`.
The instance `A = 0` exposes the mismatch: the result is `1`. Likewise, an
arbitrary numeric subtype need not be closed under negation or multiplication.

Universal ranges are part of the contract:

<!-- paper: 8 -->
```cue
choose(A: int):    func(A, A) -> A
choose(A: string): func(A, A) -> A
stronger(A: int | string): func(A, A) -> A

r(A): {id: func(A) -> A}
r: {label: "utilities"}
// The same accumulated constraint:
r(A): {id: func(A) -> A, label: "utilities"}
```

The two `choose` clauses cover integer subtypes and string subtypes.
`stronger` also covers mixed subtypes such as `1 | "one"`, hence calls with
one inhabitant of each kind. Shrinking a bound checks fewer instances and
weakens that universal obligation. Adding another declaration with `&`
adds an obligation to the same subject.

Universals distribute over conjunction when their ranges agree. A constraint
independent of a binder, such as `label`, can move inside it. Independent
universal binders can exchange order. Disjunction needs more care: allowing
either of two predicates at *each* instance can admit a subject that satisfies
neither predicate at *every* instance. Thus distributing `forall` over `|`
would change its meaning.

## Function contracts and open refinement

A function type is a predicate on an implementation. For the mathematical
notation `A → B`, `A` describes the accepted arguments and `B` describes
successful results. With several parameters, `A` describes an **argument
packet**: the supplied positions, labels, and omissions together.

The arrow means: for every complete packet satisfying `A`, every successful
return of this implementation satisfies `B`. This is a **partial-correctness
contract**. Execution can encounter a constraint failure or remain unfinished.
Successful return is what establishes the result's membership in `B`.

### A result contract is a proof obligation

A result annotation asks the checker to prove something about the body.
An explicit body constraint such as `x & >0` is an executable **assertion**:
it forwards `x` when the constraint succeeds and fails otherwise.

<!-- paper: 34 -->
```cue
asText: func(x: int) -> string: x           // rejected
asTwo:  func(x: int) -> 2: 1                // rejected
positive: func(x: int) -> (int & >0): x     // rejected
positiveChecked: func(x: int) -> (int & >0): x & >0
ok: positiveChecked(3)                     // 3
bad: positiveChecked(0)                    // _|_
pick: func(x: int | string) -> int: x       // rejected
pickChecked: func(x: int | string) -> int: x & int
stop(A): func(x: A) -> A: _|_               // certified partial function
```

The first three bodies can successfully return values outside their promised
results: an integer, `1`, or `0` respectively supplies a counterexample.
`pick` can return a string. Their asserted counterparts establish the desired
constraint on every successful execution. `stop` has no successful return,
so it satisfies its partial contract. Its closure is a value; calling it
produces bottom.

### Contravariance checks a capability

Suppose a client requires `A → B`, and the implementation accepts packets
described by `C`. Attaching that interface creates two obligations:

1. **Coverage:** prove `A ≤ C`, so every promised packet is accepted.
2. **Result:** under an arbitrary input in `A`, prove that the body returns
   a value in `B` whenever it succeeds.

If the implementation already has result type `D`, `D ≤ B` proves the second
obligation. Thus, when `A ≤ C` and `D ≤ B`, we have `(C → D) ≤ (A → B)`.
Inputs are **contravariant** and results **covariant**: accepting more inputs
and promising a more precise result gives a stronger capability.

<!-- paper: 29 -->
```cue
wide: func(number) -> string
wide: func(x: int) -> string: "ok"       // rejected: number ≤ int fails
narrow: func(int) -> string
narrow: func(x: number) -> string: "ok"  // accepted: int ≤ number
apply: func(g: func(number) -> string) -> string: g(1.5)
ints: func(x: int) -> string: "ok"
bad: apply(ints)                         // rejected at callback linking
```

The fractional packet `1.5` witnesses the failed coverage checks. The constant
body's string result satisfies its result obligation, independently of that
input failure. Supplying a callback links an actual implementation to the
capability assumed by its client.

### A live description remains connected to its calls

A field used as a type is a **live description**. Its constraints can be
refined by later declarations. An alias of a fixed type names that fixed
predicate:

<!-- paper: 16 -->
```cue
let Fixed = int
fixed: func(x: Fixed) -> int: 2 * x

Live: int
live: func(x: Live) -> int: 2 * x
a: fixed(2)                 // 4
b: live(2)                  // 4

// A later file may add Live: >=0.
```

`Live: int` establishes the upper bound `Live ≤ int`. It supports the
multiplication proof before `Live` is concrete. The function still names
`Live`, and the call allocates a fresh packet constrained by `Live & 2`.
The packet takes its own completion; `Live` remains available for other
calls and later refinements. A `let` alias containing a live reference
preserves that reference's dependency.

For this call, the retained relationship can be written with a fresh argument
cell `p` and result cell `b`:

```text
p satisfies Live
p = 2
b = 2 * p
b satisfies int
```

Computing `b = 4` preserves all four requirements. Adding `Live: >=0` is
compatible. Adding `Live: >2` refutes this invocation because its packet
still has to satisfy `Live`.

The result can be constrained independently in another file:

<!-- paper: 17 -->
```cue
_input: int
_output: int
_f: func(x: _input) -> int: 2 * x
a: _f(2)
a: _output

// Each is a separate possible refinement:
// _input: >=0               // a remains 4
// _input: >2                // invocation fails
// _output: >=4              // a remains 4
// _output: <4               // result fails
```

Each restriction describes that call's packet or result. The reusable
function's result contract remains `int`.

### Why this refinement preserves contravariance

An open configuration describes joint choices of data, live descriptions,
and implementations. Call one such joint choice a **world**. The constraints
already present select a set of worlds, the **store**. Adding a declaration
restricts this store to worlds satisfying both declarations.

At each world, `Live → int` has the ordinary contravariant interpretation.
Across refinements, the source expression still refers to the same `Live`
coordinate. The store narrows; the source graph and its references keep
their identity. This gives two distinct comparisons:

| Comparison | What follows |
| --- | --- |
| Fixed types `Small ≤ int` | `(int → B) ≤ (Small → B)` by contravariance. |
| An open graph followed by `Live: >=0` | Joint completions must satisfy the old graph **and** the new constraint. |

For a precise statement, let `P` be the predicate of joint completions of
the original graph and `R` the new declaration. The refined graph denotes
`P ∩ R`, hence a subset of `P`. Hiding local cells still gives a subset of
the original observations. Replacing the closed type `int → B` by
`(int & >=0) → B`, in contrast, would compare two different predicates on
functions; the second requires behavior on fewer inputs.

**Refine the shared description by conjunction, and keep its dependencies.**
This preserves CUE's information order while each function capability is
checked contravariantly. An established result can stay the same or become
inconsistent. A displayed default retains the ordinary CUE preference rules
and the alternatives from which it was selected.

### The two directions of evidence

An upper bound tells the checker what operations an input supports. A result
or instance membership may require the reverse inclusion. An open goal
stays pending when its supporting facts establish only one direction.

<!-- paper: 18 -->
```cue
A: number
f: func(x: A) -> int: x      // initially pending

// Added by another file:
A: int                      // proves A ≤ int
answer: f(2)                // 2
```

Initially, `A ≤ number` leaves open whether `A ≤ int`. A fraction in
`number` is a *possible* inhabitant of `A`; that alone supplies no persistent
counterexample. Later `A: int` proves the result goal. A fixed `number`
parameter would admit `1.5` outright and refute the identity body's integer
result promise.

The same distinction appears on the result side:

<!-- paper: 19 -->
```cue
R: int
same: func(x: R) -> R: x                  // certified
constant: func(_: int) -> R: 2            // pending
checked: func(x: int) -> R: x & R         // certified partial body

// R: 2   supplies the constant's result proof.
// R: >2  refutes that constant result.
```

`same` forwards the evidence its input already carries. `constant` needs
`2 ≤ R`, whereas the declaration supplies `R ≤ int`. `checked` establishes
membership by executing the assertion. Even equality of currently visible
record fields can leave a live result goal pending: later constraints may
add fields or validation requirements to that description.

Coverage also needs the direction appropriate to the client's capability:

<!-- paper: 24 -->
```cue
Small: int
local: func(x: Small) -> int: x + 1
consumer: func(g: func(int) -> int) -> int: g(2)
a: consumer(local)                      // coverage pending
```

The client promises itself all integer inputs, so linking requires
`int ≤ Small`. The field proves only `Small ≤ int`. The particular packet
`Small & 2` can be compatible while this universal coverage goal remains
unproved. A fixed `let Small = int` supplies both inclusions.

Bounds on type variables can themselves be live:

<!-- paper: 25 -->
```cue
Upper: number
choose(A: Upper): func(x: A, y: A) -> A: {
    if x <= y {out: x}
    if x > y  {out: y}
}.out
a: choose[Upper](2, 5)               // 2, retaining packet constraints
needs: choose[int & >=0](2, 5)       // instance-bound proof pending
// A later file may add Upper: int; a still has result 2.
```

The body uses `A ≤ Upper ≤ number`. Selecting `Upper` proves its own bound
by reflexivity. Selecting `int & >=0` requires `(int & >=0) ≤ Upper`, which
the numeric upper bound does not supply. Adding `Upper: int` strengthens
that upper bound while the reverse inclusion remains a separate obligation.

For a live universal bound, the same store interpretation applies: at each
world, check every `A` included in that world's `Upper`. Refinement restricts
the worlds in which this fixed formula is considered. Previously established
proofs retain their references and assumptions; new declarations can supply
missing premises or impose additional goals.

<!--
 Copyright 2026 The CUE Authors

 Licensed under the Apache License, Version 2.0 (the "License");
 you may not use this file except in compliance with the License.
 You may obtain a copy of the License at

     https://www.apache.org/licenses/LICENSE-2.0

 Unless required by applicable law or agreed to in writing, software
 distributed under the License is distributed on an "AS IS" BASIS,
 WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 See the License for the specific language governing permissions and
 limitations under the License.
-->
