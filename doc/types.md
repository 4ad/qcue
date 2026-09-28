# Function types of builtins, operators, and the standard library

The contracts used by this fork's quantified function checker. Arrows describe
successful returns; `&` combines supported interfaces. Operator names below
label typing rules, not first-class operator values.

- [Builtins](#builtins)
- [Binary operators](#binary-operators)
- [Unary operators](#unary-operators)
- [Selection, slicing, and interpolation](#selection-slicing-and-interpolation)
- [Standard library](#standard-library)

## Builtins

<!-- contracts: builtin -->
```cue
and: (func([...]) -> _) &
    (forall (A) func([A, ...A]) -> A) &
    (forall (A, B) func([A, B, ...]) -> (A & B))

or: (forall (A) func([...A]) -> A) &
    (forall (A, B) func([A, B]) -> (A | B))

close: forall (A: {...}) func(A) -> A
len:   func(string | bytes | [...] | {...}) -> (int & >=0)

error: forall (A) func(string) -> A
div:   func(int, int) -> int
mod:   func(int, int) -> int
quo:   func(int, int) -> int
rem:   func(int, int) -> int

matchN:  func(int, [...]) -> validator(_)
matchIf: func(_, _, _) -> validator(_)
```

`and` needs a mandatory element to guarantee result `A`: the type
`forall (A) func([...A]) -> A` is **not** valid for `and`. For longer known
lists, its result intersects the mandatory element types; `or` unions the
possible element types. `len` additionally retains known lengths and bounds.
`close` retains input constraints while adding closedness.
`error` never returns successfully; the quantified arrow is its call-checking
rule. Direct arrow attachment to this special builtin is not supported.

`matchN` and `matchIf` are validator-constructor forms; their arguments may be
schemas. `validator(A)` records a validator's domain. `self` is a contextual
reference. Internal closing/testing helpers have syntax-sensitive rules in
[the compiler](../internal/core/compile/builtin.go), not ordinary library
function interfaces. Predeclared data types and numeric ranges are unchanged.

## Binary operators

<!-- contracts: binary -->
```cue
#Numeric: (func(int, int) -> int) &
    (func(float, float) -> float) &
    (func(number, number) -> number)

#Ordered: (func(number, number) -> bool) &
    (func(string, string) -> bool) &
    (func(bytes, bytes) -> bool)

#Data: null | bool | number | string | bytes | [...] | {...}
#Equality: (func(#Data, #Data) -> bool) &
    (func(_, null) -> bool) & (func(null, _) -> bool)

"&": forall (A, B) func(A, B) -> (A & B)
"|": forall (A, B) func(A, B) -> (A | B)

"+": #Numeric &
    (func(string, string) -> string) &
    (func(bytes, bytes) -> bytes)
"-": #Numeric
"*": #Numeric &
    (func(string, int) -> string) & (func(int, string) -> string) &
    (func(bytes, int) -> bytes) & (func(int, bytes) -> bytes)
"/": func(number, number) -> number

"==": #Equality
"!=": #Equality
"<":  #Ordered
"<=": #Ordered
">":  #Ordered
">=": #Ordered

"=~": func(string, string | bytes) -> bool
"!~": func(string, string | bytes) -> bool
"&&": func(bool, bool) -> bool
"||": func(bool, bool) -> bool
```

`#Numeric`, `#Ordered`, `#Data`, and `#Equality` are local abbreviations for
this listing. The equality signatures assume `structcmp`. Without it, equality
admits same-kind non-record data, numeric pairs, and comparisons with `null`.
The checker can refine numeric and literal results beyond these base contracts.

## Unary operators

<!-- contracts: unary -->
```cue
#Bound: (func(number) -> number) &
    (func(string) -> string) & (func(bytes) -> bytes)

"+": forall (A: number) func(A) -> A
"-": (func(int) -> int) & (func(float) -> float) &
    (func(number) -> number)
"!": func(bool) -> bool

"<":  #Bound
"<=": #Bound
">":  #Bound
">=": #Bound
"!=": #Bound & (func(bool) -> bool) &
    (func(null) -> (!=null)) &
    (func({...}) -> {...}) & (func([...]) -> [...])
"=~": (func(string) -> string) & (func(bytes) -> bytes)
"!~": (func(string) -> string) & (func(bytes) -> bytes)
```

Unary `-` preserves numeric kind, not arbitrary numeric subtypes. Bounds and
regex constraints retain their operand-dependent predicates. The default
marker in `*x | y` preserves `x`'s type; it has no separate callable type.

## Selection, slicing, and interpolation

<!-- contracts: selection -->
```cue
"xs[i]": forall (A) func([...A], int) -> A
"tuple[i]": forall (A, B) func([A, B], int) -> (A | B)
"x.field": forall (A) func({field: A}) -> A
"x[\"field\"]": forall (A) func({field: A}, "field") -> A

"xs[lo:hi]": forall (A) func([...A], int, int) -> [...A]
"b[lo:hi]": func(bytes, int, int) -> bytes

"string interpolation": func(number | string | bytes | bool) -> string
"bytes interpolation":  func(number | string | bytes | bool) -> bytes
```

For dynamic record selection, every possible label needs field evidence; the
result is the union of its field types. Postfix spread `x...` changes
closedness, and optional-reference `x?` changes presence handling in `try`;
these use contextual rules rather than standalone function types.

## Standard library

Every function signature follows, including `forall` relationships, overloads,
validator forms, parameter labels, defaults, and result refinements. Supporting
schemas and constants are collapsed beneath each package. `@schema()` marks
implementation-declared schema parameters. Sorting additionally checks the
comparator's `less` expression with `x` and `y` instantiated at the element
type; its record shape alone is insufficient.

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

[Source](../pkg/crypto/ed25519/pkg.cue)

```cue
Valid: (func(message: bytes | string, signature: bytes | string) -> validator(bytes | string)) |
	(func(publicKey: bytes | string, message: bytes | string, signature: bytes | string) -> bool)
```

<details>
<summary>Supporting schemas and constants</summary>

```cue
PublicKeySize: 32
```

</details>

<a id="package-crypto-hmac"></a>

### `crypto/hmac`

[Source](../pkg/crypto/hmac/pkg.cue)

```cue
Sign: func(hashName: string, key: bytes | string, data: bytes | string) -> bytes
```

<details>
<summary>Supporting schemas and constants</summary>

```cue
MD5:        "MD5"
SHA1:       "SHA1"
SHA224:     "SHA224"
SHA256:     "SHA256"
SHA384:     "SHA384"
SHA512:     "SHA512"
SHA512_224: "SHA512_224"
SHA512_256: "SHA512_256"
```

</details>

<a id="package-crypto-md5"></a>

### `crypto/md5`

[Source](../pkg/crypto/md5/pkg.cue)

```cue
Sum: func(data: bytes | string) -> bytes
```

<details>
<summary>Supporting schemas and constants</summary>

```cue
Size:      16
BlockSize: 64
```

</details>

<a id="package-crypto-sha1"></a>

### `crypto/sha1`

[Source](../pkg/crypto/sha1/pkg.cue)

```cue
Sum: func(data: bytes | string) -> bytes
```

<details>
<summary>Supporting schemas and constants</summary>

```cue
Size:      20
BlockSize: 64
```

</details>

<a id="package-crypto-sha256"></a>

### `crypto/sha256`

[Source](../pkg/crypto/sha256/pkg.cue)

```cue
Sum256: func(data: bytes | string) -> bytes
Sum224: func(data: bytes | string) -> bytes
```

<details>
<summary>Supporting schemas and constants</summary>

```cue
Size:      32
Size224:   28
BlockSize: 64
```

</details>

<a id="package-crypto-sha512"></a>

### `crypto/sha512`

[Source](../pkg/crypto/sha512/pkg.cue)

```cue
Sum512:     func(data: bytes | string) -> bytes
Sum384:     func(data: bytes | string) -> bytes
Sum512_224: func(data: bytes | string) -> bytes
Sum512_256: func(data: bytes | string) -> bytes
```

<details>
<summary>Supporting schemas and constants</summary>

```cue
Size:      64
Size224:   28
Size256:   32
Size384:   48
BlockSize: 128
```

</details>

<a id="package-encoding-base64"></a>

### `encoding/base64`

[Source](../pkg/encoding/base64/pkg.cue)

```cue
EncodedLen: func(encoding: null, n: int) -> int & >=-9223372036854775808 & <=9223372036854775807
DecodedLen: func(encoding: null, x: int) -> int & >=-9223372036854775808 & <=9223372036854775807
Encode:     func(encoding: null, src: bytes | string) -> string
Decode:     func(encoding: null, s: string) -> bytes
```

<a id="package-encoding-csv"></a>

### `encoding/csv`

[Source](../pkg/encoding/csv/pkg.cue)

```cue
Encode: func(x: [...[...(null | bool | number | string | bytes | [...] | {...})]]) -> string
Decode: func(r: bytes | string) -> [...[...string]]
```

<a id="package-encoding-hex"></a>

### `encoding/hex`

[Source](../pkg/encoding/hex/pkg.cue)

```cue
EncodedLen: func(n: int) -> int & >=-9223372036854775808 & <=9223372036854775807
DecodedLen: func(x: int) -> int & >=-9223372036854775808 & <=9223372036854775807
Decode:     func(s: string) -> bytes
Dump:       func(data: bytes | string) -> string
Encode:     func(src: bytes | string) -> string
```

<a id="package-encoding-json"></a>

### `encoding/json`

[Source](../pkg/encoding/json/pkg.cue)

```cue
Valid:           validator(bytes | string) | (func(data: bytes | string) -> bool)
Compact:         func(src: bytes | string) -> string
Indent:          func(src: bytes | string, prefix: string, indent: string) -> string
HTMLEscape:      func(src: bytes | string) -> string
Marshal:         func(v: null | bool | number | string | bytes | [...] | {...}) -> string
MarshalStream:   func(v: [...]) -> string
UnmarshalStream: func(data: bytes | string) -> [...]
Unmarshal:       func(b: bytes | string) -> null | bool | number | string | [...] | {...}
Validate: (func(v: _ @schema()) -> validator(bytes | string)) |
	(func(b: bytes | string, v: _ @schema()) -> bool)
```

<a id="package-encoding-openapi"></a>

### `encoding/openapi`

[Source](../pkg/encoding/openapi/pkg.cue)

```cue
MarshalSchema: func(config: {version: string, selfContained?: bool, expandReferences?: bool, info?: {...}}, schema: _ @schema()) -> string
```

<details>
<summary>Supporting schemas and constants</summary>

```cue
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

</details>

<a id="package-encoding-toml"></a>

### `encoding/toml`

[Source](../pkg/encoding/toml/pkg.cue)

```cue
Marshal:   func(v: {...}) -> string
Unmarshal: func(data: bytes | string) -> {...}
```

<a id="package-encoding-yaml"></a>

### `encoding/yaml`

[Source](../pkg/encoding/yaml/pkg.cue)

```cue
Marshal:         func(v: null | bool | number | string | bytes | [...] | {...}) -> string
MarshalStream:   func(v: [...]) -> string
Unmarshal:       func(data: bytes | string) -> null | bool | number | string | bytes | [...] | {...}
UnmarshalStream: func(data: bytes | string) -> [...]
Validate: (func(v: _ @schema()) -> validator(bytes | string)) |
	(func(b: bytes | string, v: _ @schema()) -> bool)
ValidatePartial: (func(v: _ @schema()) -> validator(bytes | string)) |
	(func(b: bytes | string, v: _ @schema()) -> bool)
```

<a id="package-html"></a>

### `html`

[Source](../pkg/html/pkg.cue)

```cue
Escape:   func(s: string) -> string
Unescape: func(s: string) -> string
```

<a id="package-list"></a>

### `list`

[Source](../pkg/list/pkg.cue)

```cue
Drop:        forall (A) func(x: [...A], n: int) -> [...A]
FlattenN:    func(xs: [...], depth: int) -> [...]
Repeat:      forall (A) func(x: [...A], count: int) -> [...A]
Concat:      forall (A) func(a: [...[...A]]) -> [...A]
Take:        forall (A) func(x: [...A], n: int) -> [...A]
Slice:       forall (A) func(x: [...A], i: int, j: int) -> [...A]
Reverse:     forall (A) func(x: [...A]) -> [...A]
MinItems:    (func(n: int) -> validator([...])) | (func(list: [...], n: int) -> bool)
MaxItems:    (func(n: int) -> validator([...])) | (func(list: [...], n: int) -> bool)
UniqueItems: validator([...]) | (func(a: [...]) -> bool)
Contains:    func(a: [...], v: _) -> bool
MatchN: (func(n: _ @schema(), matchValue: _ @schema()) -> validator([...])) |
	(func(list: [...], n: _ @schema(), matchValue: _ @schema()) -> bool)
Avg:     func(xs: [...number]) -> number
Max:     forall (A: number) func(xs: [...A]) -> A
Min:     forall (A: number) func(xs: [...A]) -> A
Product: (func(xs: [...number]) -> number) & (func(xs: [...int]) -> int)
Range: (func(start: number, limit: number, step: number) -> [...number]) &
	(func(start: int, limit: int, step: int) -> [...int])
Sum:             (func(xs: [...number]) -> number) & (func(xs: [...int]) -> int)
Sort:            forall (A) func(list: [...A], cmp: {x: _, y: _, less: bool}) -> [...A]
SortStable:      forall (A) func(list: [...A], cmp: {x: _, y: _, less: bool}) -> [...A]
SortStrings:     forall (A: string) func(a: [...A]) -> [...A]
IsSorted:        func(list: [...], cmp: {x: _, y: _, less: bool}) -> bool
IsSortedStrings: validator([...string]) | (func(a: [...string]) -> bool)
```

<details>
<summary>Supporting schemas and constants</summary>

```cue
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

</details>

<a id="package-math"></a>

### `math`

[Source](../pkg/math/pkg.cue)

```cue
Jacobi:      func(x: int, y: int) -> -1 | 0 | 1
Floor:       func(x: number) -> int
Ceil:        func(x: number) -> int
Trunc:       func(x: number) -> int
Round:       func(x: number) -> int
RoundToEven: func(x: number) -> int
MultipleOf:  (func(y: number) -> validator(number)) | (func(x: number, y: number) -> bool)
Abs:         (func(x: number) -> (number & >=0)) & (func(x: int) -> (int & >=0))
Acosh:       func(x: number) -> number & >=0
Asin:        func(x: number) -> number
Acos:        func(x: number) -> number & >=0
Asinh:       func(x: number) -> number
Atan:        func(x: number) -> number
Atan2:       func(y: number, x: number) -> number
Atanh:       func(x: number) -> number
Cbrt:        func(x: number) -> number
Copysign:    func(x: number, y: number) -> number
Dim:         func(x: number, y: number) -> number & >=0
Erf:         func(x: number) -> number & >=-1 & <=1
Erfc:        func(x: number) -> number & >=0 & <=2
Erfinv:      func(x: number) -> number
Erfcinv:     func(x: number) -> number
Exp:         func(x: number) -> number & >=0
Exp2:        func(x: number) -> number & >=0
Expm1:       func(x: number) -> number & >=-1
Gamma:       func(x: number) -> number
Hypot:       func(p: number, q: number) -> number & >=0
J0:          func(x: number) -> number
Y0:          func(x: number) -> number
J1:          func(x: number) -> number
Y1:          func(x: number) -> number
Jn:          func(n: int, x: number) -> number
Yn:          func(n: int, x: number) -> number
Ldexp:       func(frac: number, exp: int) -> number
Log:         func(x: number) -> number
Log10:       func(x: number) -> number
Log2:        func(x: number) -> number
Log1p:       func(x: number) -> number
Logb:        func(x: number) -> number
Ilogb:       func(x: number) -> int & >=-9223372036854775808 & <=9223372036854775807
Mod:         func(x: number, y: number) -> number
Pow:         func(x: number, y: number) -> number
Pow10:       func(n: int) -> number & >=0
Remainder:   func(x: number, y: number) -> number
Signbit:     validator(number) | (func(x: number) -> bool)
Cos:         func(x: number) -> number & >=-1 & <=1
Sin:         func(x: number) -> number & >=-1 & <=1
Sinh:        func(x: number) -> number
Cosh:        func(x: number) -> number & >=1
Sqrt:        func(x: number) -> number & >=0
Tan:         func(x: number) -> number
Tanh:        func(x: number) -> number & >=-1 & <=1
```

<details>
<summary>Supporting schemas and constants</summary>

```cue
MaxExp:        2147483647
MinExp:        -2147483648
MaxPrec:       4294967295
ToNearestEven: 0
ToNearestAway: 1
ToZero:        2
AwayFromZero:  3
ToNegativeInf: 4
ToPositiveInf: 5
Below:         -1
Exact:         0
Above:         1
MaxBase:       62
E:             2.71828182845904523536028747135266249775724709369995957496696763
Pi:            3.14159265358979323846264338327950288419716939937510582097494459
Phi:           1.61803398874989484820458683436563811772030917980576286213544861
Sqrt2:         1.41421356237309504880168872420969807856967187537694807317667974
SqrtE:         1.64872127070012814684865078781416357165377610071014801157507931
SqrtPi:        1.77245385090551602729816748334114518279754945612238712821380779
SqrtPhi:       1.27201964951406896425242246173749149171560804184009624861664038
Ln2:           0.693147180559945309417232121458176568075500134360255254120680009
Log2E:         1.442695040888963407359924681001892137426645954152985934135449408
Ln10:          2.3025850929940456840179914546843642076011014886287729760333278
Log10E:        0.43429448190325182765112891891660508229439700580366656611445378
```

</details>

<a id="package-math-bits"></a>

### `math/bits`

[Source](../pkg/math/bits/pkg.cue)

```cue
Lsh:       func(x: int, n: int) -> int
Rsh:       func(x: int, n: int) -> int
At:        func(x: int, i: int) -> 0 | 1
Set:       func(x: int, i: int, bit: int) -> int
And:       func(a: int, b: int) -> int
Or:        func(a: int, b: int) -> int
Xor:       func(a: int, b: int) -> int
Clear:     func(a: int, b: int) -> int
OnesCount: func(x: int) -> int & >=0
Len:       func(x: int) -> int & >=0
```

<a id="package-net"></a>

### `net`

[Source](../pkg/net/pkg.cue)

```cue
SplitHostPort: func(s: string) -> [string, string]
JoinHostPort:  func(host: string | bytes | [...int], port: string | bytes | int) -> string
FQDN:          validator(string) | (func(s: string) -> bool)
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
IPv4:                      validator(#IP) | (func(ip: #IP) -> bool)
IPv6:                      validator(#IP) | (func(ip: #IP) -> bool)
IP:                        validator(#IP) | (func(ip: #IP) -> bool)
IPCIDR:                    validator(#CIDR) | (func(ip: #CIDR) -> bool)
LoopbackIP:                validator(#IP) | (func(ip: #IP) -> bool)
MulticastIP:               validator(#IP) | (func(ip: #IP) -> bool)
InterfaceLocalMulticastIP: validator(#IP) | (func(ip: #IP) -> bool)
LinkLocalMulticastIP:      validator(#IP) | (func(ip: #IP) -> bool)
LinkLocalUnicastIP:        validator(#IP) | (func(ip: #IP) -> bool)
GlobalUnicastIP:           validator(#IP) | (func(ip: #IP) -> bool)
UnspecifiedIP:             validator(#IP) | (func(ip: #IP) -> bool)
ToIP4:                     func(ip: string | bytes | [...int]) -> [int & >=0 & <=255, int & >=0 & <=255, int & >=0 & <=255, int & >=0 & <=255]
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
IPString:  func(ip: string | bytes | [...int]) -> string
AddIP:     func(ip: string | bytes | [...int], offset: int) -> string
AddIPCIDR: func(ip: string | bytes, offset: int) -> string
ParseCIDR: func(s: string) -> {
	prefix_mask:     string
	prefix_len:      int & >=0 & <=128
	prefix_addr:     string
	broadcast_addr?: string
}
InCIDR:        (func(cidr: #CIDR) -> validator(#IP)) | (func(ip: #IP, cidr: #CIDR) -> bool)
CompareIP:     func(ip1: string | bytes | [...int], ip2: string | bytes | [...int]) -> -1 | 0 | 1
PathEscape:    func(s: string) -> string
PathUnescape:  func(s: string) -> string
QueryEscape:   func(s: string) -> string
QueryUnescape: func(s: string) -> string
URL:           validator(string) | (func(s: string) -> bool)
AbsURL:        validator(string) | (func(s: string) -> bool)
```

<details>
<summary>Supporting schemas and constants</summary>

```cue
IPv4len: 4
IPv6len: 16
#IP:     string | bytes | [...int]
#CIDR:   string | bytes
```

</details>

<a id="package-path"></a>

### `path`

[Source](../pkg/path/pkg.cue)

```cue
Match:      func(pattern: string, name: string, os: #OS = "unix") -> bool
Clean:      func(path: string, os: #OS = "unix") -> string
ToSlash:    func(path: string, os: #OS) -> string
FromSlash:  func(path: string, os: #OS) -> string
SplitList:  func(path: string, os: #OS) -> [...string]
Split:      func(path: string, os: #OS = "unix") -> [string, string]
Join:       func(elem: [...string], os: #OS = "unix") -> string
Ext:        func(path: string, os: #OS = "unix") -> string
Resolve:    func(dir: string, sub: string, os: #OS = "unix") -> string
Rel:        func(basepath: string, targpath: string, os: #OS = "unix") -> string
Base:       func(path: string, os: #OS = "unix") -> string
Dir:        func(path: string, os: #OS = "unix") -> string
IsAbs:      func(path: string, os: #OS = "unix") -> bool
VolumeName: func(path: string, os: #OS = "windows") -> string
```

<details>
<summary>Supporting schemas and constants</summary>

```cue
#OS: "unix" | "windows" | "plan9" | "aix" | "android" | "darwin" | "dragonfly" |
	"freebsd" | "hurd" | "illumos" | "ios" | "js" | "linux" | "nacl" | "netbsd" | "openbsd" | "solaris" | "zos"
Unix:    "unix"
Windows: "windows"
Plan9:   "plan9"
```

</details>

<a id="package-regexp"></a>

### `regexp`

[Source](../pkg/regexp/pkg.cue)

```cue
Find:                 func(pattern: string, s: string) -> string
FindAll:              func(pattern: string, s: string, n: int) -> [...string]
FindAllNamedSubmatch: func(pattern: string, s: string, n: int) -> [...{[string]: string}]
FindAllSubmatch:      func(pattern: string, s: string, n: int) -> [...[...string]]
FindNamedSubmatch:    func(pattern: string, s: string) -> {[string]: string}
FindSubmatch:         func(pattern: string, s: string) -> [...string]
ReplaceAll:           func(pattern: string, src: string, repl: string) -> string
ReplaceAllLiteral:    func(pattern: string, src: string, repl: string) -> string
Valid:                validator(string) | (func(pattern: string) -> bool)
Match:                func(pattern: string, s: string) -> bool
QuoteMeta:            func(s: string) -> string
```

<a id="package-strconv"></a>

### `strconv`

[Source](../pkg/strconv/pkg.cue)

```cue
Unquote:            func(s: string) -> string
ParseBool:          validator(string) | (func(str: string) -> bool)
FormatBool:         func(b: bool) -> string
ParseFloat:         func(s: string, bitSize: int) -> number
ParseNumber:        func(s: string) -> number
ParseUint:          func(s: string, base: int, bitSize: int) -> int & >=0
ParseInt:           func(s: string, base: int, bitSize: int) -> int
Atoi:               func(s: string) -> int
FormatFloat:        func(f: number, fmtVal: string | int, prec: int, bitSize: int) -> string
FormatUint:         func(i: int, base: int) -> string
FormatInt:          func(i: int, base: int) -> string
Quote:              func(s: string) -> string
QuoteToASCII:       func(s: string) -> string
QuoteToGraphic:     func(s: string) -> string
QuoteRune:          func(r: int) -> string
QuoteRuneToASCII:   func(r: int) -> string
QuoteRuneToGraphic: func(r: int) -> string
IsPrint:            validator(int) | (func(r: int) -> bool)
IsGraphic:          validator(int) | (func(r: int) -> bool)
```

<details>
<summary>Supporting schemas and constants</summary>

```cue
IntSize: 64
```

</details>

<a id="package-strings"></a>

### `strings`

[Source](../pkg/strings/pkg.cue)

```cue
ByteAt:       func(b: bytes | string, i: int) -> int & >=0 & <=255
ByteSlice:    func(b: bytes | string, start: int, end: int) -> bytes
Runes:        func(s: string) -> [...(int & >=0 & <=0x10ffff)]
Repeat:       func(s: string, count: int) -> string
MinRunes:     (func(min: int) -> validator(string)) | (func(s: string, min: int) -> bool)
MaxRunes:     (func(max: int) -> validator(string)) | (func(s: string, max: int) -> bool)
ToTitle:      func(s: string) -> string
ToCamel:      func(s: string) -> string
SliceRunes:   func(s: string, start: int, end: int) -> string
Compare:      func(a: string, b: string) -> -1 | 0 | 1
Count:        func(s: string, substr: string) -> int & >=0
Contains:     func(s: string, substr: string) -> bool
ContainsAny:  func(s: string, chars: string) -> bool
LastIndex:    func(s: string, substr: string) -> int & >=-1
IndexAny:     func(s: string, chars: string) -> int & >=-1
LastIndexAny: func(s: string, chars: string) -> int & >=-1
SplitN:       func(s: string, sep: string, n: int) -> [...string]
SplitAfterN:  func(s: string, sep: string, n: int) -> [...string]
Split:        func(s: string, sep: string) -> [...string]
SplitAfter:   func(s: string, sep: string) -> [...string]
Fields:       func(s: string) -> [...string]
Join:         func(elems: [...string], sep: string) -> string
HasPrefix:    func(s: string, prefix: string) -> bool
HasSuffix:    func(s: string, suffix: string) -> bool
ToUpper:      func(s: string) -> string
ToLower:      func(s: string) -> string
Trim:         func(s: string, cutset: string) -> string
TrimLeft:     func(s: string, cutset: string) -> string
TrimRight:    func(s: string, cutset: string) -> string
TrimSpace:    func(s: string) -> string
TrimPrefix:   func(s: string, prefix: string) -> string
TrimSuffix:   func(s: string, suffix: string) -> string
Replace:      func(s: string, old: string, new: string, n: int) -> string
Index:        func(s: string, substr: string) -> int & >=-1
```

<a id="package-struct"></a>

### `struct`

[Source](../pkg/struct/pkg.cue)

```cue
MinFields: (func(n: int) -> validator({...})) | (func(object: {...}, n: int) -> bool)
MaxFields: (func(n: int) -> validator({...})) | (func(object: {...}, n: int) -> bool)
```

<a id="package-text-tabwriter"></a>

### `text/tabwriter`

[Source](../pkg/text/tabwriter/pkg.cue)

```cue
Write: func(data: string | bytes | [...(string | bytes)]) -> string
```

<a id="package-text-template"></a>

### `text/template`

[Source](../pkg/text/template/pkg.cue)

```cue
Execute:    func(templ: string, data: null | bool | number | string | bytes | [...] | {...}) -> string
HTMLEscape: func(s: string) -> string
JSEscape:   func(s: string) -> string
```

<a id="package-time"></a>

### `time`

[Source](../pkg/time/pkg.cue)

```cue
Duration:       validator(string) | (func(s: string) -> bool)
FormatDuration: func(d: int) -> string
ParseDuration:  func(s: string) -> int & >=-9223372036854775808 & <=9223372036854775807
Time:           validator(string) | (func(s: string) -> bool)
Format: (func(layout: string) -> validator(string)) |
	(func(value: string, layout: string) -> bool)
FormatString: func(layout: string, value: string) -> string
Parse:        func(layout: string, value: string) -> string
Unix:         func(sec: int, nsec: int) -> string
ToUnix:       func(value: string) -> int & >=-9223372036854775808 & <=9223372036854775807
ToUnixNano:   func(value: string) -> int & >=-9223372036854775808 & <=9223372036854775807
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

<details>
<summary>Supporting schemas and constants</summary>

```cue
Nanosecond:  1
Microsecond: 1000
Millisecond: 1000000
Second:      1000000000
Minute:      60000000000
Hour:        3600000000000
ANSIC:       "Mon Jan _2 15:04:05 2006"
UnixDate:    "Mon Jan _2 15:04:05 MST 2006"
RubyDate:    "Mon Jan 02 15:04:05 -0700 2006"
RFC822:      "02 Jan 06 15:04 MST"
RFC822Z:     "02 Jan 06 15:04 -0700"
RFC850:      "Monday, 02-Jan-06 15:04:05 MST"
RFC1123:     "Mon, 02 Jan 2006 15:04:05 MST"
RFC1123Z:    "Mon, 02 Jan 2006 15:04:05 -0700"
RFC3339:     "2006-01-02T15:04:05Z07:00"
RFC3339Nano: "2006-01-02T15:04:05.999999999Z07:00"
RFC3339Date: "2006-01-02"
Kitchen:     "3:04PM"
Kitchen24:   "15:04"
January:     1
February:    2
March:       3
April:       4
May:         5
June:        6
July:        7
August:      8
September:   9
October:     10
November:    11
December:    12
Sunday:      0
Monday:      1
Tuesday:     2
Wednesday:   3
Thursday:    4
Friday:      5
Saturday:    6
```

</details>

<a id="package-tool"></a>

### `tool`

[Source](../pkg/tool/pkg.cue)

No function declarations.


<details>
<summary>Supporting schemas and constants</summary>

```cue
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

</details>

<a id="package-tool-cli"></a>

### `tool/cli`

[Source](../pkg/tool/cli/pkg.cue)

No function declarations.


<details>
<summary>Supporting schemas and constants</summary>

```cue
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

</details>

<a id="package-tool-exec"></a>

### `tool/exec`

[Source](../pkg/tool/exec/pkg.cue)

No function declarations.


<details>
<summary>Supporting schemas and constants</summary>

```cue
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

</details>

<a id="package-tool-file"></a>

### `tool/file`

[Source](../pkg/tool/file/pkg.cue)

No function declarations.


<details>
<summary>Supporting schemas and constants</summary>

```cue
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

</details>

<a id="package-tool-http"></a>

### `tool/http`

[Source](../pkg/tool/http/pkg.cue)

No function declarations.


<details>
<summary>Supporting schemas and constants</summary>

```cue
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

</details>

<a id="package-tool-os"></a>

### `tool/os`

[Source](../pkg/tool/os/pkg.cue)

No function declarations.


<details>
<summary>Supporting schemas and constants</summary>

```cue
Value: bool | number | *string | null
Name:  !="" & !~"^[$]"
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

</details>

<a id="package-uuid"></a>

### `uuid`

[Source](../pkg/uuid/pkg.cue)

```cue
Valid:   validator(string) | (func(s: string) -> true)
Parse:   func(s: string) -> string
URN:     func(x: string) -> string
FromInt: func(i: int) -> string
ToInt:   func(x: string) -> int & >=0
Variant: func(x: string) -> int & >=0 & <=4
Version: func(x: string) -> int & >=0 & <=15
SHA1:    func(space: string, data: bytes | string) -> string
MD5:     func(space: string, data: bytes | string) -> string
```

<details>
<summary>Supporting schemas and constants</summary>

```cue
ns: {
	DNS:  "6ba7b810-9dad-11d1-80b4-00c04fd430c8"
	URL:  "6ba7b811-9dad-11d1-80b4-00c04fd430c8"
	OID:  "6ba7b812-9dad-11d1-80b4-00c04fd430c8"
	X500: "6ba7b814-9dad-11d1-80b4-00c04fd430c8"
	Nil:  "00000000-0000-0000-0000-000000000000"
}
variants: Invalid:   0
variants: RFC4122:   1
variants: Reserved:  2
variants: Microsoft: 3
variants: Future:    4
```

</details>

<!-- END GENERATED PACKAGE TYPES -->

Generated from the published `pkg.cue` interfaces. Regenerate with
`go run ./internal/cmd/gentypes`; verify with
`go test ./internal/cmd/gentypes ./pkg`.
See the [implementation guide](implementation.md) for checking details.
