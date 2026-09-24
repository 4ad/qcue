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

package cue_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/build"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/parser"
	"cuelang.org/go/internal/core/adt"
)

func TestContextCancellationBuild(t *testing.T) {
	file, err := parser.ParseFile("test.cue", "a: 1")
	if err != nil {
		t.Fatal(err)
	}
	expr := ast.NewIdent("int")
	inst := build.NewContext().NewInstance("", nil)
	if err := inst.AddSyntax(file); err != nil {
		t.Fatal(err)
	}
	builds := map[string]func(*cue.Context) cue.Value{
		"string":     func(c *cue.Context) cue.Value { return c.CompileString("a: 1") },
		"bytes":      func(c *cue.Context) cue.Value { return c.CompileBytes([]byte("a: 1")) },
		"file":       func(c *cue.Context) cue.Value { return c.BuildFile(file) },
		"expr":       func(c *cue.Context) cue.Value { return c.BuildExpr(expr) },
		"instance":   func(c *cue.Context) cue.Value { return c.BuildInstance(inst) },
		"encode":     func(c *cue.Context) cue.Value { return c.Encode(map[string]int{"a": 1}) },
		"encodeType": func(c *cue.Context) cue.Value { return c.EncodeType(struct{ A int }{}) },
		"list":       func(c *cue.Context) cue.Value { return c.NewList() },
	}
	for name, fn := range builds {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			c := cuecontext.New(cuecontext.WithContext(ctx))
			cancel()
			if err := fn(c).Err(); !errors.Is(err, context.Canceled) {
				t.Fatalf("got %v, want canceled", err)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := cuecontext.New(cuecontext.WithContext(ctx))
	for _, instances := range [][]*build.Instance{nil, {inst}} {
		if _, err := c.BuildInstances(instances); !errors.Is(err, context.Canceled) {
			t.Fatalf("BuildInstances: %v", err)
		}
	}
	// A canceled build must not poison the input instance for another runtime.
	if err := cuecontext.New().BuildInstance(inst).Err(); err != nil {
		t.Fatal(err)
	}
}

func TestContextCancellationValues(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := cuecontext.New(cuecontext.WithContext(ctx))
	v := c.CompileString(`a: 1, b: [1,2], c: "hello", d: true`)
	number := v.LookupPath(cue.ParsePath("a"))
	list := v.LookupPath(cue.ParsePath("b"))
	text := v.LookupPath(cue.ParsePath("c"))
	boolean := v.LookupPath(cue.ParsePath("d"))
	cancel()
	tests := map[string]func() error{
		"err":      v.Err,
		"validate": func() error { return v.Validate(cue.Concrete(true)) },
		"subsume":  func() error { return v.Subsume(v) },
		"marshal":  func() error { _, err := v.MarshalJSON(); return err },
		"decode":   func() error { var x any; return v.Decode(&x) },
		"number":   func() error { _, err := number.Int64(); return err },
		"string":   func() error { _, err := text.String(); return err },
		"bool":     func() error { _, err := boolean.Bool(); return err },
		"fields":   func() error { _, err := v.Fields(); return err },
		"list":     func() error { _, err := list.List(); return err },
		"unify":    func() error { return v.Unify(v).Err() },
		"fill":     func() error { return v.FillPath(cue.ParsePath("a"), 1).Err() },
		"lookup":   func() error { return v.LookupPath(cue.ParsePath("a")).Err() },
		"default":  func() error { v, _ := v.Default(); return v.Err() },
	}
	for name, fn := range tests {
		t.Run(name, func(t *testing.T) {
			if err := fn(); !errors.Is(err, context.Canceled) {
				t.Fatalf("got %v, want canceled", err)
			}
		})
	}
}

func TestContextCancellationDuringEvaluation(t *testing.T) {
	type key struct{}
	for _, src := range []string{
		`stop()`,
		`stop() | 1`,
		`*1 | stop()`,
		`{if stop() {a: 1}}`,
		`[for x in [1,2,3] if stop() {x}]`,
		`@experiment(try)
{try x = stop() {a: x} else {a: false}}`,
		`exists (n in 1 | 2) {a: stop(), b: n}`,
		`{a: stop(), b: {c: 1}}`,
		`[stop(), {a: 1}]`,
	} {
		t.Run(src, func(t *testing.T) {
			parent := context.WithValue(context.Background(), key{}, "request")
			ctx, cancel := context.WithCancelCause(parent)
			defer cancel(nil)
			cause := errors.New("request stopped")
			c := cuecontext.New(cuecontext.WithContext(ctx))
			called := false
			builtin := c.Encode(&adt.Builtin{Name: "stop", Result: adt.BoolKind, Func: func(call adt.BuiltinCallContext) adt.Expr {
				called = true
				if call.OpContext().Context().Value(key{}) != "request" {
					t.Error("context value lost")
				}
				cancel(cause)
				return &adt.Bool{B: true}
			}})
			scope := c.CompileString("stop: _").FillPath(cue.ParsePath("stop"), builtin)
			if err := scope.Err(); err != nil {
				t.Fatal(err)
			}
			v := c.CompileString(src, cue.Scope(scope))
			if !called {
				t.Fatalf("evaluation did not reach builtin: %v", v.Err())
			}
			if err := v.Err(); !errors.Is(err, context.Canceled) || !errors.Is(err, cause) {
				t.Fatalf("cancellation lost: %v", err)
			}
		})
	}
}

func TestContextDeadlineEvaluation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	c := cuecontext.New(cuecontext.WithContext(ctx))
	start := time.Now()
	v := c.CompileString(`import "list"
[for x in list.Range(0,1000,1) for y in list.Range(0,1000,1) {x+y}]`)
	if err := v.Err(); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %v, want deadline", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("cancellation took %v", elapsed)
	}
}

func TestContextCancellationIndependentRuntimes(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := cuecontext.New(cuecontext.WithContext(parent))
	other := cuecontext.New()
	schema := other.CompileString(`a: int`)
	value := c.CompileString(`a: 1`)
	combined := schema.Unify(value)
	if err := combined.Err(); err != nil {
		t.Fatal(err)
	}
	cancel()
	if err := combined.Validate(cue.Concrete(true)); err != nil {
		t.Fatal(err)
	}
	if err := schema.Unify(other.CompileString(`a: 2`)).Err(); err != nil {
		t.Fatal(err)
	}
	if err := value.Unify(schema).Err(); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
}

func TestContextCancellationConcurrent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	c := cuecontext.New(cuecontext.WithContext(ctx))
	v := c.CompileString(`a: 1, b: [1,2,3]`)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 50 {
				err := v.Validate()
				if err != nil && !errors.Is(err, context.Canceled) {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
	cancel()
	wg.Wait()
}

func TestContextCancellationNil(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("nil context did not panic")
		}
	}()
	cuecontext.WithContext(nil)
}

func ExampleContext_cancellation() {
	ctx, cancel := context.WithCancel(context.Background())
	c := cuecontext.New(cuecontext.WithContext(ctx))
	cancel()
	err := c.CompileString("a: 1").Err()
	fmt.Println(errors.Is(err, context.Canceled))
	// Output: true
}

// Keep compilation of large already-parsed inputs covered separately from the
// parser, and ensure a deadline's cause survives the public error wrappers.
func TestContextDeadlineBuildFile(t *testing.T) {
	file, err := parser.ParseFile("", "["+strings.Repeat("{a: 1},", 100000)+"]")
	if err != nil {
		t.Fatal(err)
	}
	cause := errors.New("compilation budget")
	ctx, cancel := context.WithTimeoutCause(context.Background(), time.Millisecond, cause)
	defer cancel()
	c := cuecontext.New(cuecontext.WithContext(ctx))
	v := c.BuildFile(file)
	if err := v.Err(); !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, cause) {
		t.Fatalf("got %v", err)
	}
}

func TestContextCancellationIterator(t *testing.T) {
	for _, src := range []string{`[1,2,3]`, `a: 1, b: 2, c: 3`} {
		t.Run(src, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			c := cuecontext.New(cuecontext.WithContext(ctx))
			v := c.CompileString(src)
			var iter *cue.Iterator
			if v.Kind() == cue.ListKind {
				x, err := v.List()
				if err != nil {
					t.Fatal(err)
				}
				iter = &x
			} else {
				var err error
				iter, err = v.Fields()
				if err != nil {
					t.Fatal(err)
				}
			}
			if !iter.Next() || iter.Err() != nil {
				t.Fatal("iteration did not start")
			}
			cancel()
			for range 2 {
				if iter.Next() {
					t.Fatal("iteration continued after cancellation")
				}
				if err := iter.Err(); !errors.Is(err, context.Canceled) {
					t.Fatalf("got %v", err)
				}
			}
		})
	}
	var zero cue.Iterator
	if zero.Next() || zero.Err() != nil {
		t.Fatal("zero iterator failed")
	}
}

// Cancellation while a Go callback runs must be reported by the outer API,
// even if that callback itself returns successfully.
type cancelUnmarshaler struct{ cancel context.CancelFunc }

func (x *cancelUnmarshaler) UnmarshalJSON([]byte) error { x.cancel(); return nil }

type cancelMarshaler struct{ cancel context.CancelFunc }

func (x cancelMarshaler) MarshalJSON() ([]byte, error) { x.cancel(); return []byte(`1`), nil }

func TestContextCancellationCallbacks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := cuecontext.New(cuecontext.WithContext(ctx))
	if err := c.CompileString(`1`).Decode(&cancelUnmarshaler{cancel}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Decode: %v", err)
	}
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	c = cuecontext.New(cuecontext.WithContext(ctx))
	if err := c.Encode(cancelMarshaler{cancel}).Err(); !errors.Is(err, context.Canceled) {
		t.Fatalf("Encode: %v", err)
	}
}

func TestContextCancellationAfterIteration(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	v := cuecontext.New(cuecontext.WithContext(ctx)).CompileString(`[1]`)
	iter, err := v.List()
	if err != nil {
		t.Fatal(err)
	}
	if !iter.Next() || iter.Next() || iter.Err() != nil {
		t.Fatal("iteration did not finish normally")
	}
	cancel()
	if iter.Next() || iter.Err() != nil {
		t.Fatal("cancellation changed an exhausted iterator")
	}
}
