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

package pkg_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"cuelang.org/go/internal/core/adt"
	"cuelang.org/go/internal/core/eval"
	"cuelang.org/go/internal/core/runtime"
	"cuelang.org/go/internal/pkg"

	_ "cuelang.org/go/pkg/list"
	_ "cuelang.org/go/pkg/math"
)

// cancelCompileRuntime cancels when builtin initialization creates a compiler,
// after parsing has succeeded. This avoids depending on scheduling or deadlines.
type cancelCompileRuntime struct {
	*runtime.Runtime
	cancel context.CancelFunc
}

func (r *cancelCompileRuntime) ConfigureOpCtx(ctx *adt.OpContext) {
	r.Runtime.ConfigureOpCtx(ctx)
	if r.cancel != nil {
		r.cancel()
		r.cancel = nil
	}
}

func TestBuiltinCompilationCancellation(t *testing.T) {
	for name, p := range map[string]*pkg.Package{
		"CUE":      {CUE: "value: 1"},
		"constant": {Native: []*pkg.Builtin{{Name: "Value", Const: "42"}}},
	} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancelCause(context.Background())
			defer cancel(nil)
			cause := errors.New("request stopped during builtin compilation")
			r := &cancelCompileRuntime{Runtime: runtime.New()}
			r.SetContext(ctx)
			opCtx := eval.NewContext(r, nil)
			r.cancel = func() { cancel(cause) }
			v, err := p.MustCompile(opCtx, "test/builtin")
			if r.cancel != nil {
				t.Fatal("builtin compiler was not reached")
			}
			if v != nil {
				t.Fatal("canceled initialization returned a partial package")
			}
			if !errors.Is(err, context.Canceled) || !errors.Is(err, cause) {
				t.Fatalf("cancellation lost: %v", err)
			}
		})
	}
}

// Parsing checks Err directly; evaluation checks Done. Cancel at the first
// parser check, after MustCompile has started with a live context.
type cancelParseContext struct {
	context.Context
	cancel context.CancelFunc
	once   sync.Once
}

func (c *cancelParseContext) Err() error {
	c.once.Do(c.cancel)
	return c.Context.Err()
}

func TestBuiltinParsingCancellation(t *testing.T) {
	for name, p := range map[string]*pkg.Package{
		"CUE":      {CUE: "value: 1"},
		"constant": {Native: []*pkg.Builtin{{Name: "Value", Const: "42"}}},
	} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancelCause(context.Background())
			defer cancel(nil)
			cause := errors.New("request stopped during builtin parsing")
			r := runtime.New()
			r.SetContext(&cancelParseContext{Context: ctx, cancel: func() { cancel(cause) }})
			v, err := p.MustCompile(eval.NewContext(r, nil), "test/builtin")
			if v != nil {
				t.Fatal("canceled initialization returned a partial package")
			}
			if !errors.Is(err, context.Canceled) || !errors.Is(err, cause) {
				t.Fatalf("cancellation lost: %v", err)
			}
		})
	}
}

func TestBuiltinFinalizationCancellation(t *testing.T) {
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	cause := errors.New("request stopped during builtin finalization")
	called := false
	p := &pkg.Package{
		Native: []*pkg.Builtin{{
			Name:   "Stop",
			Result: adt.BoolKind,
			Func: func(c *pkg.CallCtxt) {
				called = true
				cancel(cause)
				c.Ret = true
			},
		}},
		CUE: "Stop: _\nready: Stop()",
	}
	r := runtime.New()
	r.SetContext(ctx)
	v, err := p.MustCompile(eval.NewContext(r, nil), "test/builtin")
	if !called {
		t.Fatal("builtin finalization did not invoke Stop")
	}
	if v != nil {
		t.Fatal("canceled initialization returned a partial package")
	}
	if !errors.Is(err, context.Canceled) || !errors.Is(err, cause) {
		t.Fatalf("cancellation lost: %v", err)
	}
}

func TestBuiltinLoadCancellation(t *testing.T) {
	cause := errors.New("request ended")
	canceled, cancel := context.WithCancelCause(context.Background())
	cancel(cause)
	expired, stop := context.WithDeadlineCause(context.Background(), time.Now().Add(-time.Second), cause)
	defer stop()
	for _, tc := range []struct {
		name string
		ctx  context.Context
		want error
	}{
		{"canceled", canceled, context.Canceled},
		{"deadline", expired, context.DeadlineExceeded},
	} {
		for _, importPath := range []string{"list", "math"} {
			t.Run(tc.name+"/"+importPath, func(t *testing.T) {
				r := runtime.New()
				r.SetContext(tc.ctx)
				// Exercise the registered callback and runtime error wrapping.
				v := r.LoadBuiltin(importPath)
				if v == nil {
					t.Fatal("builtin not registered")
				}
				b := v.Bottom()
				if b == nil || !errors.Is(b.Err, tc.want) || !errors.Is(b.Err, cause) {
					t.Fatalf("cancellation lost: %v", b)
				}
				if r.LoadInstance(r.BuiltinPackageInstance(importPath)) != nil {
					t.Fatal("canceled initialization was cached")
				}
				// The shared builtin definition remains usable by other runtimes.
				other := runtime.New()
				if b := other.LoadBuiltin(importPath).Err(eval.NewContext(other, nil)); b != nil {
					t.Fatal(b.Err)
				}
			})
		}
	}
}

func TestInvalidBuiltinPanics(t *testing.T) {
	for name, p := range map[string]*pkg.Package{
		"parse":            {CUE: "value: )"},
		"compile":          {CUE: "value: missing"},
		"finalize":         {CUE: "value: 1 & 2"},
		"constant parse":   {Native: []*pkg.Builtin{{Name: "Value", Const: ")"}}},
		"constant compile": {Native: []*pkg.Builtin{{Name: "Value", Const: "missing"}}},
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Error("invalid builtin definition did not panic")
				}
			}()
			p.MustCompile(eval.NewContext(runtime.New(), nil), "test/invalid")
		})
	}
}
