# Builtin, operator, and standard-library types

This is the type reference for **Quantified CUE**, the fork in this repository.
Functions and universal types are enabled by default. See the
[implementation guide](implementation.md) for configuration and proof limits,
and the [paper](paper.pdf) ([source](paper.tex)) for the semantics.

- [Reading the types](#reading-the-types)
- [Predeclared types and builtins](#predeclared-types-and-builtins)
- [Operators and expressions](#operators-and-expressions)
- [Calling standard-library functions](#calling-standard-library-functions)
- [Standard-library catalogue](#standard-library-catalogue)
- [Maintaining this reference](#maintaining-this-reference)

## Reading the types

CUE types are constraints on values. A literal such as `3` is also a type,
containing just that value. `_` admits any value; `_|_` admits none. `A & B`
requires both constraints, and `A | B` admits either. In the tables below,
`A`, `B`, and `R` stand for arbitrary descriptions.

| Notation | Meaning |
| --- | --- |
| `[...A]` | A list of zero or more elements satisfying `A`. |
| `[A, ...A]` | At least one element, with every element satisfying `A`. |
| `[A, B]` | Exactly two elements, in the indicated order. |
| `{...}` or `{}` | An open record; additional fields are permitted. |
| `{x: A, y?: B}` | An `x` field constrained by `A`; `y`, if present, satisfies `B`. |
| `#Name` | A definition, commonly used for a named schema. |
| `func(x: A) -> R` | A function interface with one argument and successful results in `R`. |
| `func(x: A = v) -> R` | The argument defaults to `v` when omitted. |
| `forall (A) func(x: A) -> A` | One implementation supports every type instance `A`. |
| `forall (A: number) ...` | `A` may be any subtype of `number`. |
| `validator(A)` | The domain of a native validator; its predicate is retained by the validator value. |

A declaration containing only `func(...) -> ...` describes an interface; it
has no executable body. A function implementation adds `: expression`.
Parameter names in the package catalogue are callable labels as well as
positional slots. For example, `strings.Repeat(count: 2, s: "ab")` uses the same
slots as `strings.Repeat("ab", 2)`.

A result type describes **successful returns**. It does not promise that every
input produces a value. Parsing malformed text, dividing by zero, overflowing a
native conversion, or indexing outside a list can fail despite correct operand
types. Non-concrete data can leave evaluation incomplete. Type checking does
not fill those inputs with arbitrary values.

An intersection of arrows describes one implementation satisfying each
interface. For example, `list.Sum` accepts numbers and additionally guarantees
an integer result for integer inputs. A union containing a `validator(...)`
form records a native's validation and ordinary call forms; see
[validators](#validators-and-schema-arguments).

## Predeclared types and builtins

These names need no import. Their definitions are in
[predeclared.go](../internal/core/compile/predeclared.go),
[builtin.go](../internal/core/compile/builtin.go), and
[validator.go](../internal/core/compile/validator.go).

### Primitive types and numeric ranges

| Name | Description |
| --- | --- |
| `bool` | `true` or `false`. |
| `string` | Text strings. |
| `bytes` | Byte strings, written with single quotes. |
| `int` | Integers, without a fixed machine-word bound. |
| `float` | Decimal floating-point values; a distinct numeric kind even for integral literals such as `1.0`. |
| `number` | `int` or `float`. |
| `uint` | `int & >=0`. |
| `rune` | `int & >=0 & <=0x10ffff`; this bound does not exclude Unicode surrogates. |
| `int8`, `int16`, `int32`, `int64`, `int128` | For the indicated width *n*: integers from −2<sup>n−1</sup> through 2<sup>n−1</sup>−1. |
| `uint8`, `uint16`, `uint32`, `uint64`, `uint128` | For width *n*: integers from 0 through 2<sup>n</sup>−1. |
| `float32` | Numbers between `-3.40282346638528859811704183484516925440e+38` and the positive bound. |
| `float64` | Numbers between `-1.797693134862315708145274237317043567981e+308` and the positive bound. |

The `float32` and `float64` names impose inclusive magnitude bounds. They do
not round values, restrict precision to IEEE representations, or exclude
integers. There is no predeclared `byte` alias; use `uint8` for one byte's
numeric value and `bytes` for a byte string. `null` is its own singleton value
and type. Lists and records use structural syntax, rather than a predeclared
`list` or `struct` type.

### Functions

The arrows in this table describe positional call domains. The more precise
rules below explain information retained from individual operands.

| Builtin | General call type | Meaning |
| --- | --- | --- |
| `len` | `func(string \| bytes \| [...] \| {...}) -> int` | Byte length of strings/bytes, element count of lists, or count of present regular record fields. |
| `close` | `func({...}) -> {...}` | Close a record against additional fields; retain its existing constraints. |
| `and` | `func([...]) -> _` | Unify the list's elements. The empty conjunction is `_`. |
| `or` | `func([...]) -> _` | Form a disjunction of the list's elements. The empty disjunction has no successful value. |
| `div` | `func(int, int) -> int` | Euclidean quotient. |
| `mod` | `func(int, int) -> int` | Euclidean remainder, nonnegative and smaller than the divisor's absolute value. |
| `quo` | `func(int, int) -> int` | Quotient truncated toward zero. |
| `rem` | `func(int, int) -> int` | Remainder for `quo`, with the dividend's sign when nonzero. |
| `error` | `func(string) -> _\|_` | Fail with a custom diagnostic. |

`len` does not count definitions, hidden fields, or absent optional fields.
Its result is nonnegative; checking can retain exact lengths or length bounds
when the operand supplies them. Counting an open list or record observes its
currently present elements or fields. `close` closes the outer record, not all
nested records recursively, and preserves the operand's field types.

All four integer division functions reject a zero divisor. They satisfy
`x == div(x, y)*y + mod(x, y)` and `x == quo(x, y)*y + rem(x, y)` on successful
calls.

For a nonempty list of `A` values, `and` has a successful result in `A`.
`or` has a successful result in `A` even when its input type permits an empty
list. Thus the following wrappers check:

```cue
// Nonempty conjunction and arbitrary disjunction.
_all(A): func(xs: [A, ...A]) -> A: and(xs)
_any(A): func(xs: [...A]) -> A: or(xs)
merged: _all([{a: 1}, {b: 1}])
chosen: _any([1, 2]) & 2
// Result: merged = {"a":1,"b":1}
// Result: chosen = 2
```

The original empty-permitting conjunction interface is invalid:

```cue invalid
// An empty conjunction cannot promise an arbitrary A.
_f(A): func(xs: [...A]) -> A: and(xs)
```

Its body must also handle `xs: []`, for which `and(xs)` is `_`, not an arbitrary
`A`. More generally, only mandatory elements establish a conjunction's result
constraints; an optional tail cannot establish a mandatory result field.
`or([])` currently reports an incomplete empty-disjunction error.

### Validator constructors and contextual names

| Form | Type or role |
| --- | --- |
| `matchN(n, checks)` | A validator of `_`; `n` is an integer constraint and `checks` is a list of schemas. |
| `matchIf(condition, then, else)` | A validator of `_`; all three arguments are schemas. |
| `validator(A)` | A type former recording the domain `A` of a native validator. |
| `self` | With `aliasv2`, a contextual reference to the containing subject; its type comes from that subject. |

`matchN` counts how many schemas in `checks` match the subject being validated
and constrains that count by `n`. `matchIf` validates the subject against `then`
when it matches `condition`, otherwise against `else`. They inspect the
subject's completed validation state and are used as constraints, not as
ordinary functions returning Boolean values. This differs from `list.MatchN`,
which counts matching **elements of a list**.

```cue
// Apply a builtin validator to a value.
positive: 4 & matchN(1, [int & >0, string])
conditional: 4 & matchIf(int, >0, string)
// Result: positive = 4
// Result: conditional = 4
```

Public predeclared identifiers also have `__` spellings, such as `__int`,
`__len`, and `__validator`, for referring to them when ordinary names are
shadowed. `validator` is available with the functions extension, which is on
by default in this fork.

The compiler also recognizes internal helpers. `__closeAll` recursively closes
a literal record or list; `__reclose` reapplies definition closedness to a
literal or a `close(...)` call. Both require `explicitopen` and have
syntax-sensitive operands, rather than unrestricted first-class function
interfaces. `__test_experiment` returns its argument only with the testing
experiment enabled. `__no_sharing` is an evaluator sentinel, not a callable
function. These are implementation facilities, not standard-library APIs.

## Operators and expressions

Operator checking covers every possible operand alternative. Two independently
chosen `int | string` operands cannot be added: the integer/string and
string/integer combinations are invalid. A default preference does not remove
an alternative from this check.

### Binary operators

| Operator | Operand domains | Successful result |
| --- | --- | --- |
| `&` | Any `A`, `B` | `A & B`; a conflict has no successful value. |
| `\|` | Any `A`, `B` | `A \| B`. |
| `+`, `-`, `*` | Two numbers | `int` for two integers, `float` for two floats, otherwise `number`. |
| `/` | Two numbers | `number`, including for two integer operands. |
| `+` | Two strings, or two byte strings | The same text kind, by concatenation. |
| `*` | A string or byte string and an `int`, in either order | The text operand's kind, by repetition. |
| `==`, `!=` | Data values; see the comparison rules below | `bool`. |
| `<`, `<=`, `>`, `>=` | Two numbers, two strings, or two byte strings | `bool`. |
| `=~`, `!~` | A `string` subject and a `string \| bytes` regex pattern | `bool`. |
| `&&`, `\|\|` | Two `bool` operands | `bool`. |

With `structcmp` enabled, equality accepts combinations of `null`, Boolean,
numeric, string, bytes, list, and record kinds, including different data kinds.
Record and list equality uses data comparison, not equivalence of schemas.
Without `structcmp`, record equality and cross-kind comparisons are restricted;
same-kind non-record data, numeric pairs, and comparisons with `null` remain
supported. The language version and file experiments select this behavior.
Functions do not have general `==` equality, though comparison with `null` is
allowed. The list predicates' closure-identity handling is a separate rule.

List concatenation and repetition use `list.Concat` and `list.Repeat`, not
`+` and `*`. Integer quotient and remainder use the predeclared functions above;
there is no `%` operator. Invalid regex syntax, zero division, and other value
errors remain possible after operand checking.

Logical operators retain their Boolean operand domains when the `shortcircuit`
experiment is enabled; that experiment changes whether execution needs to
evaluate the right operand. Exponentiation uses `math.Pow`, not `^`.

### Unary operators and bounds

| Operator | Operand | Successful result or constraint |
| --- | --- | --- |
| `+x` | A number | Preserves `x`'s description. |
| `-x` | A number | Numeric negation, preserving numeric kind and supported interval refinements. |
| `!x` | `bool` | `bool`. |
| `<x`, `<=x`, `>x`, `>=x` | A number, string, or bytes | A bound on `number`, `string`, or `bytes`, respectively. |
| `!=x` | A data value | Excludes that value within its kind; numeric bounds admit `number`, and `!=null` admits non-null values. |
| `=~x`, `!~x` | A string or bytes pattern | A regex constraint of the pattern's kind. |
| `*x` in a disjunction | Any description | Marks the preferred alternative; does not change its type. |

Unlike unary `+`, unary `-` does not preserve an arbitrary subtype `A`:
negating an inhabitant of `int & >0` does not return that subtype. A numeric
bound such as `>3` admits numbers, not just integers; write `int & >3` when
integer values are required.

### Selection, slicing, interpolation, and calls

| Expression | Required information | Successful result |
| --- | --- | --- |
| `x.field` | A record with evidence for that field | The field's description. |
| `x[label]` | A record and a known label, or finite alternatives of known labels | The union of the selected field descriptions. |
| `xs[i]` | A list and an integer index | The possible selected element descriptions; `[...A]` yields `A`. |
| `xs[lo:hi]` | A list and integer bounds | A list preserving possible element descriptions. |
| `b[lo:hi]` | Bytes and integer bounds | `bytes`. |
| `"\(x)"`, `'\(x)'` | Interpolated values in `number \| string \| bytes \| bool` | The enclosing literal's `string` or `bytes` kind. |
| `f(args)` | An executable function and arguments admitted by its call protocol | The checked result description. |
| `f[A]` | A universal function and an admissible type argument | Its selected instance, retaining the implementation's universal obligations. |
| `x...` | A record or list, with `explicitopen` enabled | Spreads its contents while recursively disregarding closedness inherited from `x`. |
| `x?`, `x.field?`, `x[i]?` | A reference inside a `try` clause, with `try` enabled | The reference's value when available; absence can select the fallback instead. |

An optional field alone does not establish availability for selection. Every
possible dynamic record label needs field evidence. List indexing can fail
out of bounds; a tuple `[A, B]` indexed by an arbitrary integer has successful
results in `A | B`. Slice bounds may be omitted and use the usual beginning/end
defaults. Direct string indexing and slicing, and direct byte indexing, are
not supported; use the string/byte conversion helpers in the catalogue.

Postfix spread keeps existing field and element constraints; closedness can be
imposed again by the enclosing definition. On other value kinds, spread is
currently a no-op. It is distinct from the `...` that creates a partial call
or declares an open list tail. Optional-reference `?` does not itself produce
a Boolean. Field markers `?` and `!`, alias bindings `=` and `~`, and the arrow
`->` are declaration syntax rather than ordinary value operators. See the
[base-language reference](ref/spec.md) for their syntax and experiment rules.

```cue
// An index preserves the list's element type.
_at(A): func(xs: [...A], i: int) -> A: xs[i]
selected: _at([{name: "Ada"}, {name: "Grace"}], 1)
// Result: selected = {"name":"Grace"}
```

Checking also retains literal results and supported arithmetic refinements.
It is bounded proof search, not a complete decision procedure for every CUE
constraint. A mathematically valid interface can require evidence beyond the
current checker; consult the [implementation guide](implementation.md).

## Calling standard-library functions

### Generic results, labels, defaults, and partial calls

Native generic signatures retain relationships between arguments and results.
For example, list rearrangement functions retain their element type, `list.Min`
and `list.Max` retain the input number subtype, and `list.SortStrings` retains
the input string subtype. Structural rules can additionally preserve tuple
positions and lengths for known list transformations. Implementation-declared
pure functions can provide exact results for finite concrete alternatives,
within the checker's proof budget. These facts come from the implementation;
adding a client annotation cannot make an unsupported contract true.

```cue
// Select a generic native and save a labeled argument.
import (
    "list"
    "strings"
)
_reverse: list.Reverse[{name: string}]
_twice: strings.Repeat(count: 2, ...)
names: _reverse([{name: "Ada"}, {name: "Grace"}])
text: _twice("ab")
// Result: names = [{"name":"Grace"},{"name":"Ada"}]
// Result: text = "abab"
```

A trailing `...` explicitly creates a partial function. It keeps saved operands
live; later refinement and source export preserve their dependencies. Defaults
belong to individual declarations: most `path` operations default to `"unix"`,
`path.VolumeName` defaults to `"windows"`, and `path.ToSlash`, `path.FromSlash`,
and `path.SplitList` require an explicit operating system.

### Validators and schema arguments

A native predicate can also constrain its first argument. A bare one-argument
validator, such as `list.UniqueItems`, is used directly. A validator with saved
arguments, such as `strings.MinRunes(3)`, is constructed by omitting the validated
first slot. Supplying every argument performs the ordinary Boolean call.
Explicit `...` instead requests a partial function, including for predicates.

```cue
// A validation constraint, a Boolean call, and a partial function.
import "strings"
name: "Ada" & strings.MinRunes(3)
valid: strings.MinRunes("Ada", 3)
_check: strings.MinRunes(min: 3, ...)
alsoValid: _check("Grace")
// Result: name = "Ada"
// Result: valid = true
// Result: alsoValid = true
```

The catalogue explicitly lists validator forms declared by the package API.
The evaluator also permits the implicit first-slot validator form for other
eligible Boolean natives, such as `list.IsSorted(cmp)`. A native with parameter
defaults, such as `path.IsAbs`, does not acquire this form. `uuid.Valid` has an
ordinary result of `true` on success; invalid input produces an error.

An implementation-owned `@schema()` parameter deliberately accepts a
non-concrete description. Examples include JSON/YAML validation, OpenAPI's
schema input, and the count and match descriptions in `list.MatchN`.
A saved schema keeps its references through partial application. An ordinary
data slot does not become a schema slot because client code adds `@schema()`.

```cue
// Save a schema as an argument to a JSON validator.
import "encoding/json"
_accept: json.Validate(v: {name: string}, ...)
valid: _accept('{"name":"Ada"}')
// Result: valid = true
```

### Sorting templates and unresolved data

`list.Sort` and `list.SortStable` take a comparator record with fields `x`, `y`,
and `less`. It is a reusable template: checking supplies the actual element
description to both slots and verifies that `less` is Boolean. The shape
`{x: _, y: _, less: bool}` alone does not prove that a particular comparison
body supports arbitrary elements.

```cue
// Sort records using evidence for the fields read by the comparator.
import "list"
_sort: func(xs: [...{rank: int}]) -> [...{rank: int}]:
    list.Sort(xs, {x: _, y: _, less: x.rank < y.rank})
ordered: _sort([{rank: 2}, {rank: 1}])
// Result: ordered = [{"rank":1},{"rank":2}]
```

Using `x < y` for completely unconstrained `A` is invalid: records, for example,
do not support ordered comparison. `list.Ascending` and `list.Descending`
support numbers or strings whose possible pairs admit comparison; a mixed
number/string list is not ordered by those templates. `Sort` is stable;
`SortStable` is its deprecated spelling. `IsSorted` uses the same comparator
mechanism and returns `bool` when its comparisons can be completed.

Unresolved comparator captures remain incomplete until refined. Likewise,
`list.Contains` and `list.UniqueItems` retain unresolved comparisons unless a
known match, duplicate, or disjoint description already determines the result.
OpenAPI configuration flags and metadata must also be sufficiently concrete
when serialization demands them.

These rules include correctness fixes to earlier behavior. The addition of
contracts and the accompanying evaluator fixes are **not a guarantee of 100%
backwards compatibility**: invalid function bodies can be rejected earlier,
and some operations that previously returned premature results now remain
incomplete. See the regression coverage linked below.

### Conversion boundaries

Package declarations describe the domains accepted by their native adapters.
All four `encoding/base64` functions require `null` as their encoding selector.
Serialization and template inputs admit data kinds, not arbitrary function
values. CSV cells, byte conversions, native integer ranges, and encoded-text
formats have further value requirements that can still fail at execution.
A broad record/list domain does not guarantee that every nested value can be
serialized. Optional result fields are marked `?`; fixed tuples, numeric
bounds, and literal results in the catalogue are part of the contract.

The `tool/*` packages expose task schemas for `cue cmd`, not ordinary pure
functions. Their incomplete fields include inputs and outputs supplied during
task execution. The `tool` entry describes the command schema injected into
`_tool.cue` and `_tool_test.cue` files; `tool` itself cannot be imported as a
regular package.

## Standard-library catalogue

Each block is the package's complete API declaration, including types,
constants, defaults, imports, attributes, and task schemas. Function bodies
are supplied by the linked native implementation; these blocks are reference
interfaces, not replacement package implementations. Follow a package's source
link for descriptions of individual operations. Package `@pure()` attributes
record implementation evidence used for checking, not a capability clients
can grant to arbitrary code.

<!-- BEGIN GENERATED PACKAGE TYPES -->

This catalogue covers all 33 package interfaces.

- [`crypto/ed25519`](#package-crypto-ed25519)
- [`crypto/hmac`](#package-crypto-hmac)
- [`crypto/md5`](#package-crypto-md5)
- [`crypto/sha1`](#package-crypto-sha1)
- [`crypto/sha256`](#package-crypto-sha256)
- [`crypto/sha512`](#package-crypto-sha512)
- [`encoding/base64`](#package-encoding-base64)
- [`encoding/csv`](#package-encoding-csv)
- [`encoding/hex`](#package-encoding-hex)
- [`encoding/json`](#package-encoding-json)
- [`encoding/openapi`](#package-encoding-openapi)
- [`encoding/toml`](#package-encoding-toml)
- [`encoding/yaml`](#package-encoding-yaml)
- [`html`](#package-html)
- [`list`](#package-list)
- [`math`](#package-math)
- [`math/bits`](#package-math-bits)
- [`net`](#package-net)
- [`path`](#package-path)
- [`regexp`](#package-regexp)
- [`strconv`](#package-strconv)
- [`strings`](#package-strings)
- [`struct`](#package-struct)
- [`text/tabwriter`](#package-text-tabwriter)
- [`text/template`](#package-text-template)
- [`time`](#package-time)
- [`tool`](#package-tool)
- [`tool/cli`](#package-tool-cli)
- [`tool/exec`](#package-tool-exec)
- [`tool/file`](#package-tool-file)
- [`tool/http`](#package-tool-http)
- [`tool/os`](#package-tool-os)
- [`uuid`](#package-uuid)

<a id="package-crypto-ed25519"></a>

### `crypto/ed25519`

[API declarations and comments](../pkg/crypto/ed25519/pkg.cue)

```cue
@experiment(functions)
@pure()

package ed25519

PublicKeySize: 32

Valid: (func(message: bytes | string, signature: bytes | string) -> validator(bytes | string)) |
	(func(publicKey: bytes | string, message: bytes | string, signature: bytes | string) -> bool)
```

<a id="package-crypto-hmac"></a>

### `crypto/hmac`

[API declarations and comments](../pkg/crypto/hmac/pkg.cue)

```cue
@experiment(functions)
@pure()

package hmac

MD5: "MD5"

SHA1: "SHA1"

SHA224: "SHA224"

SHA256: "SHA256"

SHA384: "SHA384"

SHA512: "SHA512"

SHA512_224: "SHA512_224"

SHA512_256: "SHA512_256"

Sign: func(hashName: string, key: bytes | string, data: bytes | string) -> bytes
```

<a id="package-crypto-md5"></a>

### `crypto/md5`

[API declarations and comments](../pkg/crypto/md5/pkg.cue)

```cue
@experiment(functions)
@pure()

package md5

Size: 16

BlockSize: 64

Sum: func(data: bytes | string) -> bytes
```

<a id="package-crypto-sha1"></a>

### `crypto/sha1`

[API declarations and comments](../pkg/crypto/sha1/pkg.cue)

```cue
@experiment(functions)
@pure()

package sha1

Size: 20

BlockSize: 64

Sum: func(data: bytes | string) -> bytes
```

<a id="package-crypto-sha256"></a>

### `crypto/sha256`

[API declarations and comments](../pkg/crypto/sha256/pkg.cue)

```cue
@experiment(functions)
@pure()

package sha256

Size: 32

Size224: 28

BlockSize: 64

Sum256: func(data: bytes | string) -> bytes

Sum224: func(data: bytes | string) -> bytes
```

<a id="package-crypto-sha512"></a>

### `crypto/sha512`

[API declarations and comments](../pkg/crypto/sha512/pkg.cue)

```cue
@experiment(functions)
@pure()

package sha512

Size: 64

Size224: 28

Size256: 32

Size384: 48

BlockSize: 128

Sum512: func(data: bytes | string) -> bytes

Sum384: func(data: bytes | string) -> bytes

Sum512_224: func(data: bytes | string) -> bytes

Sum512_256: func(data: bytes | string) -> bytes
```

<a id="package-encoding-base64"></a>

### `encoding/base64`

[API declarations and comments](../pkg/encoding/base64/pkg.cue)

```cue
@experiment(functions)
@pure()

package base64

EncodedLen: func(encoding: null, n: int) -> int & >=-9223372036854775808 & <=9223372036854775807

DecodedLen: func(encoding: null, x: int) -> int & >=-9223372036854775808 & <=9223372036854775807

Encode: func(encoding: null, src: bytes | string) -> string

Decode: func(encoding: null, s: string) -> bytes
```

<a id="package-encoding-csv"></a>

### `encoding/csv`

[API declarations and comments](../pkg/encoding/csv/pkg.cue)

```cue
@experiment(functions)
@pure()

package csv

Encode: func(x: [...[...(null | bool | number | string | bytes | [...] | {...})]]) -> string

Decode: func(r: bytes | string) -> [...[...string]]
```

<a id="package-encoding-hex"></a>

### `encoding/hex`

[API declarations and comments](../pkg/encoding/hex/pkg.cue)

```cue
@experiment(functions)
@pure()

package hex

EncodedLen: func(n: int) -> int & >=-9223372036854775808 & <=9223372036854775807

DecodedLen: func(x: int) -> int & >=-9223372036854775808 & <=9223372036854775807

Decode: func(s: string) -> bytes

Dump: func(data: bytes | string) -> string

Encode: func(src: bytes | string) -> string
```

<a id="package-encoding-json"></a>

### `encoding/json`

[API declarations and comments](../pkg/encoding/json/pkg.cue)

```cue
@experiment(functions)
@pure()

package json

Valid: validator(bytes | string) | (func(data: bytes | string) -> bool)

Compact: func(src: bytes | string) -> string

Indent: func(src: bytes | string, prefix: string, indent: string) -> string

HTMLEscape: func(src: bytes | string) -> string

Marshal: func(v: null | bool | number | string | bytes | [...] | {...}) -> string

MarshalStream: func(v: [...]) -> string

UnmarshalStream: func(data: bytes | string) -> [...]

Unmarshal: func(b: bytes | string) -> null | bool | number | string | [...] | {...}

Validate: (func(v: _ @schema()) -> validator(bytes | string)) |
	(func(b: bytes | string, v: _ @schema()) -> bool)
```

<a id="package-encoding-openapi"></a>

### `encoding/openapi`

[API declarations and comments](../pkg/encoding/openapi/pkg.cue)

```cue
@experiment(functions)
@pure()

package openapi

MarshalSchema: func(config: {version: string, selfContained?: bool, expandReferences?: bool, info?: {...}}, schema: _ @schema()) -> string

#Config: {

	version!: "3.0.0"

	info?: #Info

	selfContained: bool | *false

	expandReferences: bool | *false
}

#Info: {
	title!:          string
	version!:        string
	summary?:        string
	description?:    string
	termsOfService?: string
	contact?:        #Contact
	license?:        #License
}

#Contact: {
	name?:  string
	url?:   string
	email?: string
}

#License: {
	name!: string
	url?:  string
}
```

<a id="package-encoding-toml"></a>

### `encoding/toml`

[API declarations and comments](../pkg/encoding/toml/pkg.cue)

```cue
@experiment(functions)
@pure()

package toml

Marshal: func(v: {...}) -> string

Unmarshal: func(data: bytes | string) -> {...}
```

<a id="package-encoding-yaml"></a>

### `encoding/yaml`

[API declarations and comments](../pkg/encoding/yaml/pkg.cue)

```cue
@experiment(functions)
@pure()

package yaml

Marshal: func(v: null | bool | number | string | bytes | [...] | {...}) -> string

MarshalStream: func(v: [...]) -> string

Unmarshal: func(data: bytes | string) -> null | bool | number | string | bytes | [...] | {...}

UnmarshalStream: func(data: bytes | string) -> [...]

Validate: (func(v: _ @schema()) -> validator(bytes | string)) |
	(func(b: bytes | string, v: _ @schema()) -> bool)

ValidatePartial: (func(v: _ @schema()) -> validator(bytes | string)) |
	(func(b: bytes | string, v: _ @schema()) -> bool)
```

<a id="package-html"></a>

### `html`

[API declarations and comments](../pkg/html/pkg.cue)

```cue
@experiment(functions)
@pure()

package html

Escape: func(s: string) -> string

Unescape: func(s: string) -> string
```

<a id="package-list"></a>

### `list`

[API declarations and comments](../pkg/list/pkg.cue)

```cue
@experiment(functions)
@pure()

package list

Drop: forall (A) func(x: [...A], n: int) -> [...A]

FlattenN: func(xs: [...], depth: int) -> [...]

Repeat: forall (A) func(x: [...A], count: int) -> [...A]

Concat: forall (A) func(a: [...[...A]]) -> [...A]

Take: forall (A) func(x: [...A], n: int) -> [...A]

Slice: forall (A) func(x: [...A], i: int, j: int) -> [...A]

Reverse: forall (A) func(x: [...A]) -> [...A]

MinItems: (func(n: int) -> validator([...])) | (func(list: [...], n: int) -> bool)

MaxItems: (func(n: int) -> validator([...])) | (func(list: [...], n: int) -> bool)

UniqueItems: validator([...]) | (func(a: [...]) -> bool)

Contains: func(a: [...], v: _) -> bool

MatchN: (func(n: _ @schema(), matchValue: _ @schema()) -> validator([...])) |
	(func(list: [...], n: _ @schema(), matchValue: _ @schema()) -> bool)

Avg: func(xs: [...number]) -> number

Max: forall (A: number) func(xs: [...A]) -> A

Min: forall (A: number) func(xs: [...A]) -> A

Product: (func(xs: [...number]) -> number) & (func(xs: [...int]) -> int)

Range: (func(start: number, limit: number, step: number) -> [...number]) &
	(func(start: int, limit: int, step: int) -> [...int])

Sum: (func(xs: [...number]) -> number) & (func(xs: [...int]) -> int)

Sort: forall (A) func(list: [...A], cmp: {x: _, y: _, less: bool}) -> [...A]

SortStable: forall (A) func(list: [...A], cmp: {x: _, y: _, less: bool}) -> [...A]

SortStrings: forall (A: string) func(a: [...A]) -> [...A]

IsSorted: func(list: [...], cmp: {x: _, y: _, less: bool}) -> bool

IsSortedStrings: validator([...string]) | (func(a: [...string]) -> bool)

Comparer: {
	T:    _
	x:    T
	y:    T
	less: bool
}

Ascending: {
	Comparer
	T:    number | string
	x:    T
	y:    T
	less: x < y
}

Descending: {
	Comparer
	T:    number | string
	x:    T
	y:    T
	less: x > y
}
```

<a id="package-math"></a>

### `math`

[API declarations and comments](../pkg/math/pkg.cue)

```cue
@experiment(functions)
@pure()

package math

MaxExp: 2147483647

MinExp: -2147483648

MaxPrec: 4294967295

ToNearestEven: 0

ToNearestAway: 1

ToZero: 2

AwayFromZero: 3

ToNegativeInf: 4

ToPositiveInf: 5

Below: -1

Exact: 0

Above: 1

Jacobi: func(x: int, y: int) -> -1 | 0 | 1

MaxBase: 62

Floor: func(x: number) -> int

Ceil: func(x: number) -> int

Trunc: func(x: number) -> int

Round: func(x: number) -> int

RoundToEven: func(x: number) -> int

MultipleOf: (func(y: number) -> validator(number)) | (func(x: number, y: number) -> bool)

Abs: (func(x: number) -> (number & >=0)) & (func(x: int) -> (int & >=0))

Acosh: func(x: number) -> number & >=0

Asin: func(x: number) -> number

Acos: func(x: number) -> number & >=0

Asinh: func(x: number) -> number

Atan: func(x: number) -> number

Atan2: func(y: number, x: number) -> number

Atanh: func(x: number) -> number

Cbrt: func(x: number) -> number

E: 2.71828182845904523536028747135266249775724709369995957496696763

Pi: 3.14159265358979323846264338327950288419716939937510582097494459

Phi: 1.61803398874989484820458683436563811772030917980576286213544861

Sqrt2: 1.41421356237309504880168872420969807856967187537694807317667974

SqrtE: 1.64872127070012814684865078781416357165377610071014801157507931

SqrtPi: 1.77245385090551602729816748334114518279754945612238712821380779

SqrtPhi: 1.27201964951406896425242246173749149171560804184009624861664038

Ln2: 0.693147180559945309417232121458176568075500134360255254120680009

Log2E: 1.442695040888963407359924681001892137426645954152985934135449408

Ln10: 2.3025850929940456840179914546843642076011014886287729760333278

Log10E: 0.43429448190325182765112891891660508229439700580366656611445378

Copysign: func(x: number, y: number) -> number

Dim: func(x: number, y: number) -> number & >=0

Erf: func(x: number) -> number & >=-1 & <=1

Erfc: func(x: number) -> number & >=0 & <=2

Erfinv: func(x: number) -> number

Erfcinv: func(x: number) -> number

Exp: func(x: number) -> number & >=0

Exp2: func(x: number) -> number & >=0

Expm1: func(x: number) -> number & >=-1

Gamma: func(x: number) -> number

Hypot: func(p: number, q: number) -> number & >=0

J0: func(x: number) -> number

Y0: func(x: number) -> number

J1: func(x: number) -> number

Y1: func(x: number) -> number

Jn: func(n: int, x: number) -> number

Yn: func(n: int, x: number) -> number

Ldexp: func(frac: number, exp: int) -> number

Log: func(x: number) -> number

Log10: func(x: number) -> number

Log2: func(x: number) -> number

Log1p: func(x: number) -> number

Logb: func(x: number) -> number

Ilogb: func(x: number) -> int & >=-9223372036854775808 & <=9223372036854775807

Mod: func(x: number, y: number) -> number

Pow: func(x: number, y: number) -> number

Pow10: func(n: int) -> number & >=0

Remainder: func(x: number, y: number) -> number

Signbit: validator(number) | (func(x: number) -> bool)

Cos: func(x: number) -> number & >=-1 & <=1

Sin: func(x: number) -> number & >=-1 & <=1

Sinh: func(x: number) -> number

Cosh: func(x: number) -> number & >=1

Sqrt: func(x: number) -> number & >=0

Tan: func(x: number) -> number

Tanh: func(x: number) -> number & >=-1 & <=1
```

<a id="package-math-bits"></a>

### `math/bits`

[API declarations and comments](../pkg/math/bits/pkg.cue)

```cue
@experiment(functions)
@pure()

package bits

Lsh: func(x: int, n: int) -> int

Rsh: func(x: int, n: int) -> int

At: func(x: int, i: int) -> 0 | 1

Set: func(x: int, i: int, bit: int) -> int

And: func(a: int, b: int) -> int

Or: func(a: int, b: int) -> int

Xor: func(a: int, b: int) -> int

Clear: func(a: int, b: int) -> int

OnesCount: func(x: int) -> int & >=0

Len: func(x: int) -> int & >=0
```

<a id="package-net"></a>

### `net`

[API declarations and comments](../pkg/net/pkg.cue)

```cue
@experiment(functions)
@pure()

package net

SplitHostPort: func(s: string) -> [string, string]

JoinHostPort: func(host: string | bytes | [...int], port: string | bytes | int) -> string

FQDN: validator(string) | (func(s: string) -> bool)

IPv4len: 4

IPv6len: 16

ParseIP: func(s: string) -> ([int & >=0 & <=255, int & >=0 & <=255, int & >=0 & <=255, int & >=0 & <=255] | [
	int & >=0 & <=255,
	int & >=0 & <=255,
	int & >=0 & <=255,
	int & >=0 & <=255,
	int & >=0 & <=255,
	int & >=0 & <=255,
	int & >=0 & <=255,
	int & >=0 & <=255,
	int & >=0 & <=255,
	int & >=0 & <=255,
	int & >=0 & <=255,
	int & >=0 & <=255,
	int & >=0 & <=255,
	int & >=0 & <=255,
	int & >=0 & <=255,
	int & >=0 & <=255,
])

IPv4: validator(#IP) | (func(ip: #IP) -> bool)

IPv6: validator(#IP) | (func(ip: #IP) -> bool)

IP: validator(#IP) | (func(ip: #IP) -> bool)

IPCIDR: validator(#CIDR) | (func(ip: #CIDR) -> bool)

LoopbackIP: validator(#IP) | (func(ip: #IP) -> bool)

MulticastIP: validator(#IP) | (func(ip: #IP) -> bool)

InterfaceLocalMulticastIP: validator(#IP) | (func(ip: #IP) -> bool)

LinkLocalMulticastIP: validator(#IP) | (func(ip: #IP) -> bool)

LinkLocalUnicastIP: validator(#IP) | (func(ip: #IP) -> bool)

GlobalUnicastIP: validator(#IP) | (func(ip: #IP) -> bool)

UnspecifiedIP: validator(#IP) | (func(ip: #IP) -> bool)

ToIP4: func(ip: string | bytes | [...int]) -> [int & >=0 & <=255, int & >=0 & <=255, int & >=0 & <=255, int & >=0 & <=255]

ToIP16: func(ip: string | bytes | [...int]) -> [
	int & >=0 & <=255,
	int & >=0 & <=255,
	int & >=0 & <=255,
	int & >=0 & <=255,
	int & >=0 & <=255,
	int & >=0 & <=255,
	int & >=0 & <=255,
	int & >=0 & <=255,
	int & >=0 & <=255,
	int & >=0 & <=255,
	int & >=0 & <=255,
	int & >=0 & <=255,
	int & >=0 & <=255,
	int & >=0 & <=255,
	int & >=0 & <=255,
	int & >=0 & <=255,
]

IPString: func(ip: string | bytes | [...int]) -> string

AddIP: func(ip: string | bytes | [...int], offset: int) -> string

AddIPCIDR: func(ip: string | bytes, offset: int) -> string

ParseCIDR: func(s: string) -> {
	prefix_mask:     string
	prefix_len:      int & >=0 & <=128
	prefix_addr:     string
	broadcast_addr?: string
}

InCIDR: (func(cidr: #CIDR) -> validator(#IP)) | (func(ip: #IP, cidr: #CIDR) -> bool)

CompareIP: func(ip1: string | bytes | [...int], ip2: string | bytes | [...int]) -> -1 | 0 | 1

PathEscape: func(s: string) -> string

PathUnescape: func(s: string) -> string

QueryEscape: func(s: string) -> string

QueryUnescape: func(s: string) -> string

URL: validator(string) | (func(s: string) -> bool)

AbsURL: validator(string) | (func(s: string) -> bool)

#IP: string | bytes | [...int]

#CIDR: string | bytes
```

<a id="package-path"></a>

### `path`

[API declarations and comments](../pkg/path/pkg.cue)

```cue
@experiment(functions)

package path

#OS: "unix" | "windows" | "plan9" | "aix" | "android" | "darwin" | "dragonfly" |
	"freebsd" | "hurd" | "illumos" | "ios" | "js" | "linux" | "nacl" | "netbsd" | "openbsd" | "solaris" | "zos"

Match: func(pattern: string, name: string, os: #OS = "unix") -> bool

Unix: "unix"

Windows: "windows"

Plan9: "plan9"

Clean: func(path: string, os: #OS = "unix") -> string

ToSlash: func(path: string, os: #OS) -> string

FromSlash: func(path: string, os: #OS) -> string

SplitList: func(path: string, os: #OS) -> [...string]

Split: func(path: string, os: #OS = "unix") -> [string, string]

Join: func(elem: [...string], os: #OS = "unix") -> string

Ext: func(path: string, os: #OS = "unix") -> string

Resolve: func(dir: string, sub: string, os: #OS = "unix") -> string

Rel: func(basepath: string, targpath: string, os: #OS = "unix") -> string

Base: func(path: string, os: #OS = "unix") -> string

Dir: func(path: string, os: #OS = "unix") -> string

IsAbs: func(path: string, os: #OS = "unix") -> bool

VolumeName: func(path: string, os: #OS = "windows") -> string
```

<a id="package-regexp"></a>

### `regexp`

[API declarations and comments](../pkg/regexp/pkg.cue)

```cue
@experiment(functions)
@pure()

package regexp

Find: func(pattern: string, s: string) -> string

FindAll: func(pattern: string, s: string, n: int) -> [...string]

FindAllNamedSubmatch: func(pattern: string, s: string, n: int) -> [...{[string]: string}]

FindAllSubmatch: func(pattern: string, s: string, n: int) -> [...[...string]]

FindNamedSubmatch: func(pattern: string, s: string) -> {[string]: string}

FindSubmatch: func(pattern: string, s: string) -> [...string]

ReplaceAll: func(pattern: string, src: string, repl: string) -> string

ReplaceAllLiteral: func(pattern: string, src: string, repl: string) -> string

Valid: validator(string) | (func(pattern: string) -> bool)

Match: func(pattern: string, s: string) -> bool

QuoteMeta: func(s: string) -> string
```

<a id="package-strconv"></a>

### `strconv`

[API declarations and comments](../pkg/strconv/pkg.cue)

```cue
@experiment(functions)
@pure()

package strconv

Unquote: func(s: string) -> string

ParseBool: validator(string) | (func(str: string) -> bool)

FormatBool: func(b: bool) -> string

ParseFloat: func(s: string, bitSize: int) -> number

ParseNumber: func(s: string) -> number

IntSize: 64

ParseUint: func(s: string, base: int, bitSize: int) -> int & >=0

ParseInt: func(s: string, base: int, bitSize: int) -> int

Atoi: func(s: string) -> int

FormatFloat: func(f: number, fmtVal: string | int, prec: int, bitSize: int) -> string

FormatUint: func(i: int, base: int) -> string

FormatInt: func(i: int, base: int) -> string

Quote: func(s: string) -> string

QuoteToASCII: func(s: string) -> string

QuoteToGraphic: func(s: string) -> string

QuoteRune: func(r: int) -> string

QuoteRuneToASCII: func(r: int) -> string

QuoteRuneToGraphic: func(r: int) -> string

IsPrint: validator(int) | (func(r: int) -> bool)

IsGraphic: validator(int) | (func(r: int) -> bool)
```

<a id="package-strings"></a>

### `strings`

[API declarations and comments](../pkg/strings/pkg.cue)

```cue
@experiment(functions)
@pure()

package strings

ByteAt: func(b: bytes | string, i: int) -> int & >=0 & <=255

ByteSlice: func(b: bytes | string, start: int, end: int) -> bytes

Runes: func(s: string) -> [...(int & >=0 & <=0x10ffff)]

Repeat: func(s: string, count: int) -> string

MinRunes: (func(min: int) -> validator(string)) | (func(s: string, min: int) -> bool)

MaxRunes: (func(max: int) -> validator(string)) | (func(s: string, max: int) -> bool)

ToTitle: func(s: string) -> string

ToCamel: func(s: string) -> string

SliceRunes: func(s: string, start: int, end: int) -> string

Compare: func(a: string, b: string) -> -1 | 0 | 1

Count: func(s: string, substr: string) -> int & >=0

Contains: func(s: string, substr: string) -> bool

ContainsAny: func(s: string, chars: string) -> bool

LastIndex: func(s: string, substr: string) -> int & >=-1

IndexAny: func(s: string, chars: string) -> int & >=-1

LastIndexAny: func(s: string, chars: string) -> int & >=-1

SplitN: func(s: string, sep: string, n: int) -> [...string]

SplitAfterN: func(s: string, sep: string, n: int) -> [...string]

Split: func(s: string, sep: string) -> [...string]

SplitAfter: func(s: string, sep: string) -> [...string]

Fields: func(s: string) -> [...string]

Join: func(elems: [...string], sep: string) -> string

HasPrefix: func(s: string, prefix: string) -> bool

HasSuffix: func(s: string, suffix: string) -> bool

ToUpper: func(s: string) -> string

ToLower: func(s: string) -> string

Trim: func(s: string, cutset: string) -> string

TrimLeft: func(s: string, cutset: string) -> string

TrimRight: func(s: string, cutset: string) -> string

TrimSpace: func(s: string) -> string

TrimPrefix: func(s: string, prefix: string) -> string

TrimSuffix: func(s: string, suffix: string) -> string

Replace: func(s: string, old: string, new: string, n: int) -> string

Index: func(s: string, substr: string) -> int & >=-1
```

<a id="package-struct"></a>

### `struct`

[API declarations and comments](../pkg/struct/pkg.cue)

```cue
@experiment(functions)
@pure()

package struct

MinFields: (func(n: int) -> validator({...})) | (func(object: {...}, n: int) -> bool)

MaxFields: (func(n: int) -> validator({...})) | (func(object: {...}, n: int) -> bool)
```

<a id="package-text-tabwriter"></a>

### `text/tabwriter`

[API declarations and comments](../pkg/text/tabwriter/pkg.cue)

```cue
@experiment(functions)
@pure()

package tabwriter

Write: func(data: string | bytes | [...(string | bytes)]) -> string
```

<a id="package-text-template"></a>

### `text/template`

[API declarations and comments](../pkg/text/template/pkg.cue)

```cue
@experiment(functions)
@pure()

package template

Execute: func(templ: string, data: null | bool | number | string | bytes | [...] | {...}) -> string

HTMLEscape: func(s: string) -> string

JSEscape: func(s: string) -> string
```

<a id="package-time"></a>

### `time`

[API declarations and comments](../pkg/time/pkg.cue)

```cue
@experiment(functions)
@pure()

package time

Nanosecond: 1

Microsecond: 1000

Millisecond: 1000000

Second: 1000000000

Minute: 60000000000

Hour: 3600000000000

Duration: validator(string) | (func(s: string) -> bool)

FormatDuration: func(d: int) -> string

ParseDuration: func(s: string) -> int & >=-9223372036854775808 & <=9223372036854775807

ANSIC: "Mon Jan _2 15:04:05 2006"

UnixDate: "Mon Jan _2 15:04:05 MST 2006"

RubyDate: "Mon Jan 02 15:04:05 -0700 2006"

RFC822: "02 Jan 06 15:04 MST"

RFC822Z: "02 Jan 06 15:04 -0700"

RFC850: "Monday, 02-Jan-06 15:04:05 MST"

RFC1123: "Mon, 02 Jan 2006 15:04:05 MST"

RFC1123Z: "Mon, 02 Jan 2006 15:04:05 -0700"

RFC3339: "2006-01-02T15:04:05Z07:00"

RFC3339Nano: "2006-01-02T15:04:05.999999999Z07:00"

RFC3339Date: "2006-01-02"

Kitchen: "3:04PM"

Kitchen24: "15:04"

January: 1

February: 2

March: 3

April: 4

May: 5

June: 6

July: 7

August: 8

September: 9

October: 10

November: 11

December: 12

Sunday: 0

Monday: 1

Tuesday: 2

Wednesday: 3

Thursday: 4

Friday: 5

Saturday: 6

Time: validator(string) | (func(s: string) -> bool)

Format: (func(layout: string) -> validator(string)) |
	(func(value: string, layout: string) -> bool)

FormatString: func(layout: string, value: string) -> string

Parse: func(layout: string, value: string) -> string

Unix: func(sec: int, nsec: int) -> string

ToUnix: func(value: string) -> int & >=-9223372036854775808 & <=9223372036854775807

ToUnixNano: func(value: string) -> int & >=-9223372036854775808 & <=9223372036854775807

Split: func(t: string) -> {
	year:       int
	month:      int & >=1 & <=12
	day:        int & >=1 & <=31
	hour:       int & >=0 & <=23
	minute:     int & >=0 & <=59
	second:     int & >=0 & <=59
	nanosecond: int & >=0 & <1000000000
}
```

<a id="package-tool"></a>

### `tool`

[API declarations and comments](../pkg/tool/pkg.cue)

```cue
package tool

Command: {

	Tasks

	$usage?: string

	$short?: string

	$long?: string
}

Tasks: Task | {
	[Name]: Tasks
}

Name: =~#"^\PL([-](\PL|\PN))*$"#

Task: {

	$id: =~#"\."#

	$after?: Task | [...Task]
}
```

<a id="package-tool-cli"></a>

### `tool/cli`

[API declarations and comments](../pkg/tool/cli/pkg.cue)

```cue
package cli

Print: {
	$id: _id
	_id: *"tool/cli.Print" | "print"

	text: string
}

Ask: {
	$id: _id
	_id: "tool/cli.Ask"

	prompt: string

	response: string | bool
}
```

<a id="package-tool-exec"></a>

### `tool/exec`

[API declarations and comments](../pkg/tool/exec/pkg.cue)

```cue
package exec

Run: {
	$id: _id
	_id: *"tool/exec.Run" | "exec"

	cmd: string | [string, ...string]

	dir?: string

	env: *{[string]: string} | [...=~"="]

	stdout: *null | string | bytes

	stderr: *null | string | bytes

	stdin: *null | string | bytes

	success: bool

	mustSucceed: bool | *true
}
```

<a id="package-tool-file"></a>

### `tool/file`

[API declarations and comments](../pkg/tool/file/pkg.cue)

```cue
package file

Read: {
	$id: _id
	_id: "tool/file.Read"

	filename: !=""

	contents: *bytes | string
}

Append: {
	$id: _id
	_id: "tool/file.Append"

	filename: !=""

	permissions: int | *0o666

	contents: bytes | string
}

Create: {
	$id: _id
	_id: "tool/file.Create"

	filename: !=""

	permissions: int | *0o666

	contents: bytes | string
}

Symlink: {
	$id: _id
	_id: "tool/file.Symlink"

	filename: !=""

	target: !=""
}

Glob: {
	$id: _id
	_id: "tool/file.Glob"

	glob:  !=""
	files: [...string]
}

Mkdir: {
	$id: _id
	_id: "tool/file.Mkdir"

	path: string

	createParents: bool | *false

	permissions: int | *0o777
}

MkdirAll: Mkdir & {
	createParents: true
}

MkdirTemp: {
	$id: _id
	_id: "tool/file.MkdirTemp"

	dir: string | *""

	pattern: string | *""

	path: string
}

RemoveAll: {
	$id: _id
	_id: "tool/file.RemoveAll"

	path: string

	success: bool
}
```

<a id="package-tool-http"></a>

### `tool/http`

[API declarations and comments](../pkg/tool/http/pkg.cue)

```cue
package http

Get:    Do & {method: "GET"}
Post:   Do & {method: "POST"}
Put:    Do & {method: "PUT"}
Delete: Do & {method: "DELETE"}

Do: {
	$id: _id
	_id: *"tool/http.Do" | "http"

	method: string
	url:    string

	followRedirects: *true | bool

	timeout?: string

	tls: {

		verify: *true | bool

		caCert?: bytes | string
	}

	request: {
		body?: bytes | string
		header: [string]:  string | [...string]
		trailer: [string]: string | [...string]
	}
	response: {
		status:     string
		statusCode: int

		body: *bytes | string
		header: [string]:  string | [...string]
		trailer: [string]: string | [...string]
	}
}

Serve: {
	$id: _id
	_id: "tool/http.Serve"

	listenAddr!: string

	routing: {

		path: *"/" | =~"^/"

		method?: string
	}

	request: {

		method: string

		url: string

		body: *bytes | string

		value?: _

		pathValues: [string]: string

		form: [string]: [string, ...string]

		header: [string]: [string, ...string]

		trailer: [string]: [string, ...string]
	}

	response: {

		statusCode?: int & >=100 & <=999

		body?: *bytes | string

		header?: [string]: string | [string, ...string]

		trailer?: [string]: string | [string, ...string]
	}
}
```

<a id="package-tool-os"></a>

### `tool/os`

[API declarations and comments](../pkg/tool/os/pkg.cue)

```cue
package os

Value: bool | number | *string | null

Name: !="" & !~"^[$]"

Setenv: {
	$id: _id
	_id: "tool/os.Setenv"

	{[Name]: Value}
}

Getenv: {
	$id: _id
	_id: "tool/os.Getenv"

	{[Name]: Value}
}

Environ: {
	$id: _id
	_id: "tool/os.Environ"

	{[Name]: Value}
}

Clearenv: {
	$id: _id
	_id: "tool/os.Clearenv"
}
```

<a id="package-uuid"></a>

### `uuid`

[API declarations and comments](../pkg/uuid/pkg.cue)

```cue
@experiment(functions)
@pure()

package uuid

Valid: validator(string) | (func(s: string) -> true)

Parse: func(s: string) -> string

URN: func(x: string) -> string

FromInt: func(i: int) -> string

ToInt: func(x: string) -> int & >=0

Variant: func(x: string) -> int & >=0 & <=4

Version: func(x: string) -> int & >=0 & <=15

SHA1: func(space: string, data: bytes | string) -> string

MD5: func(space: string, data: bytes | string) -> string

ns: {
	DNS:  "6ba7b810-9dad-11d1-80b4-00c04fd430c8"
	URL:  "6ba7b811-9dad-11d1-80b4-00c04fd430c8"
	OID:  "6ba7b812-9dad-11d1-80b4-00c04fd430c8"
	X500: "6ba7b814-9dad-11d1-80b4-00c04fd430c8"
	Nil:  "00000000-0000-0000-0000-000000000000"
}

variants: Invalid: 0

variants: RFC4122: 1

variants: Reserved: 2

variants: Microsoft: 3

variants: Future: 4
```

<!-- END GENERATED PACKAGE TYPES -->

## Maintaining this reference

The package catalogue is generated from the same `pkg/*/pkg.cue` declarations
used by editor tooling, via `pkg.ImportPaths()` and `pkg.Source()`. Edit those
declarations first, then regenerate from the repository root:

```sh
go run ./internal/cmd/gentypes
go run ./internal/cmd/gentypes -check
go test ./internal/cmd/gentypes ./pkg
go test ./cue -run 'TestBuiltin|TestOperator|TestStdlib'
```

`TestReferenceCurrent` detects catalogue drift, and `TestReferenceExamples`
checks every example above, including rejected programs and stated results.
The `pkg` tests verify that every registered package has declarations, that
all declarations are embedded, and that signatures agree with native checking
evidence, labels, defaults, and validator forms. The native inventory test
checks each registered function's calls, partials, argument domains, and result
contract. These tests complement the
[operator matrix](../cue/operator_checking_test.go) and
[quantified corpus](../cue/testdata/quantified/README.md).

The predeclared and operator sections are maintained by hand against the linked
compiler sources and the operator matrix. When changing language rules, update
those sections and their examples as well as the package catalogue.
