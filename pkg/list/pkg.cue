// Copyright 2026 The CUE Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// This file is maintained by hand: it is the authoritative description
// of the package's API for tooling such as editor completion and hover.
// Its consistency with the registered builtins is checked by the tests
// in cuelang.org/go/pkg.

@experiment(functions)
@pure()

// Package list contains functions for manipulating and examining lists.
package list

// Drop reports the suffix of list x after the first n elements,
// or [] if n > len(x).
//
// For instance:
//
// 	Drop([1, 2, 3, 4], 2)
//
// results in
//
// 	[3, 4]
Drop: forall (A) func(x: [...A], n: int) -> [...A]

// FlattenN reports a flattened sequence of the list xs by expanding any elements
// depth levels deep. If depth is negative all elements are expanded.
//
// For instance:
//
// 	FlattenN([1, [[2, 3], []], [4]], 1)
//
// results in
//
// 	[1, [2, 3], [], 4]
FlattenN: func(xs: _, depth: int) -> [...]

// Repeat returns a new list consisting of count copies of list x.
//
// For instance:
//
// 	Repeat([1, 2], 2)
//
// results in
//
// 	[1, 2, 1, 2]
Repeat: forall (A) func(x: [...A], count: int) -> [...A]

// Concat takes a list of lists and concatenates them.
//
// Concat([a, b, c]) is equivalent to
//
// 	[for x in a {x}, for x in b {x}, for x in c {x}]
Concat: forall (A) func(a: [...[...A]]) -> [...A]

// Take reports the prefix of length n of list x, or x itself if n > len(x).
//
// For instance:
//
// 	Take([1, 2, 3, 4], 2)
//
// results in
//
// 	[1, 2]
Take: forall (A) func(x: [...A], n: int) -> [...A]

// Slice extracts the consecutive elements from list x starting from position i
// up till, but not including, position j, where 0 <= i < j <= len(x).
//
// For instance:
//
// 	Slice([1, 2, 3, 4], 1, 3)
//
// results in
//
// 	[2, 3]
Slice: forall (A) func(x: [...A], i: int, j: int) -> [...A]

// Reverse reverses a list.
//
// For instance:
//
// 	Reverse([1, 2, 3, 4])
//
// results in
//
// 	[4, 3, 2, 1]
Reverse: forall (A) func(x: [...A]) -> [...A]

// MinItems reports whether a has at least n items.
MinItems: (func(n: int) -> validator([...])) | (func(list: [...], n: int) -> bool)

// MaxItems reports whether a has at most n items.
MaxItems: (func(n: int) -> validator([...])) | (func(list: [...], n: int) -> bool)

// UniqueItems reports whether all elements in the list are unique.
UniqueItems: validator([...]) | (func(a: [...]) -> bool)

// Contains reports whether v is contained in a. The value must be a
// comparable and concrete value.
// For non-concrete values, you can use [MatchN] with >0.
Contains: func(a: [...], v: _) -> bool

// MatchN is a validator that checks that the number of elements in the given
// list that unifies with the schema "matchValue" matches "n".
// "n" may be a number constraint and does not have to be a concrete number.
// Likewise, "matchValue" will usually be a non-concrete value.
MatchN: (func(n: _ @schema(), matchValue: _ @schema()) -> validator([...])) | (func(list: [...], n: _ @schema(), matchValue: _ @schema()) -> bool)

// Avg returns the average value of a non empty list xs.
Avg: func(xs: [...number]) -> number

// Max returns the maximum value of a non empty list xs.
Max: forall (A: number) func(xs: [...A]) -> A

// Min returns the minimum value of a non empty list xs.
Min: forall (A: number) func(xs: [...A]) -> A

// Product returns the product of a non empty list xs.
Product: (func(xs: [...number]) -> number) & (func(xs: [...int]) -> int)

// Range generates a list of numbers using a start value, a limit value, and a
// step value.
//
// For instance:
//
// 	Range(0, 5, 2)
//
// results in
//
// 	[0, 2, 4]
Range: (func(start: number, limit: number, step: number) -> [...number]) & (func(start: int, limit: int, step: int) -> [...int])

// Sum returns the sum of a list non empty xs.
Sum: (func(xs: [...number]) -> number) & (func(xs: [...int]) -> int)

// Sort sorts data while keeping the original order of equal elements.
// It does O(n*log(n)) comparisons.
//
// cmp is a struct of the form {T: _, x: T, y: T, less: bool}, where
// less should reflect x < y.
//
// Example:
//
// 	Sort([2, 3, 1], list.Ascending)
//
// 	Sort([{a: 2}, {a: 3}, {a: 1}], {x: {}, y: {}, less: x.a < y.a})
Sort: forall (A) func(list: [...A], cmp: {x: _, y: _, less: bool}) -> [...A]

// Deprecated: use [Sort], which is always stable
SortStable: forall (A) func(list: [...A], cmp: {x: _, y: _, less: bool}) -> [...A]

// SortStrings sorts a list of strings in increasing order.
SortStrings: forall (A: string) func(a: [...A]) -> [...A]

// IsSorted tests whether a list is sorted.
//
// See Sort for an example comparator.
IsSorted: func(list: [...], cmp: {x: _, y: _, less: bool}) -> bool

// IsSortedStrings tests whether a list is a sorted list of strings.
IsSortedStrings: validator([...string]) | (func(a: [...string]) -> bool)

// A Comparer specifies whether one value is strictly less than another value.
Comparer: {
	T:    _
	x:    T
	y:    T
	less: bool // true if x < y
}

// Ascending defines a Comparer to sort comparable values in increasing order.
//
// Example:
//     list.Sort(a, list.Ascending)
Ascending: {
	Comparer
	T:    number | string
	x:    T
	y:    T
	less: x < y
}

// Descending defines a Comparer to sort comparable values in decreasing order.
//
// Example:
//     list.Sort(a, list.Descending)
Descending: {
	Comparer
	T:    number | string
	x:    T
	y:    T
	less: x > y
}
