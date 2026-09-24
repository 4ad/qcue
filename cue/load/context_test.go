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

package load_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"cuelang.org/go/cue/ast"
	"cuelang.org/go/cue/load"
	"cuelang.org/go/cue/parser"
	"cuelang.org/go/mod/modconfig"
	"cuelang.org/go/mod/module"
)

func TestInstancesContextCanceled(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancel", true: "deadline"}[deadline], func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			want := context.Canceled
			if deadline {
				cancel()
				ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
				want = context.DeadlineExceeded
			}
			cancel()
			// Cancellation must precede config validation and disk access.
			instances := load.InstancesContext(ctx, nil, &load.Config{Dir: "/does/not/exist"})
			if len(instances) != 1 || !errors.Is(instances[0].Err, want) {
				t.Fatalf("got %v, want %v", instances, want)
			}
		})
	}
}

func TestInstancesContextLocal(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fsys := fstest.MapFS{
		"a.cue": {Data: []byte("package p\na: 1")},
		"b.cue": {Data: []byte("package p\nb: 2")},
	}
	calls := 0
	cfg := &load.Config{FS: fsys, SkipImports: true,
		ParseFile: func(name string, src any, cfg parser.Config) (*ast.File, error) {
			calls++
			cancel()
			return parser.ParseFile(name, src, cfg)
		},
	}
	instances := load.InstancesContext(ctx, nil, cfg)
	if len(instances) != 1 || !errors.Is(instances[0].Err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", instances)
	}
	if calls != 1 {
		t.Fatalf("parsed %d files after cancellation", calls)
	}
	// Loading must not mutate the caller's config or poison a later load.
	cfg.ParseFile = nil
	instances = load.Instances(nil, cfg)
	if len(instances) != 1 || instances[0].Err != nil || len(instances[0].Files) != 2 {
		t.Fatalf("subsequent load failed: %v", instances)
	}
}

type blockingRegistry struct {
	modconfig.Registry
	entered chan context.Context
	once    sync.Once
}

func (r *blockingRegistry) Fetch(ctx context.Context, m module.Version) (module.SourceLoc, error) {
	r.once.Do(func() { r.entered <- ctx })
	<-ctx.Done()
	return module.SourceLoc{}, ctx.Err()
}

func TestInstancesContextRegistry(t *testing.T) {
	type key struct{}
	ctx, cancel := context.WithTimeout(context.WithValue(context.Background(), key{}, "request"), 10*time.Second)
	defer cancel()
	registry := &blockingRegistry{entered: make(chan context.Context, 1)}
	cfg := &load.Config{Registry: registry, FS: fstest.MapFS{
		"cue.mod/module.cue": {Data: []byte(`module: "main.test@v0"
language: version: "v0.9.0"
deps: "dep.test@v0": v: "v0.0.1"`)},
		"main.cue": {Data: []byte(`package main
import "dep.test"
x: dep.x`)},
	}}
	done := make(chan error, 1)
	go func() {
		instances := load.InstancesContext(ctx, nil, cfg)
		if len(instances) != 1 {
			done <- errors.New("unexpected instance count")
			return
		}
		done <- instances[0].Err
	}()
	select {
	case received := <-registry.entered:
		if received.Value(key{}) != "request" {
			t.Error("context value lost")
		}
		if _, ok := received.Deadline(); !ok {
			t.Error("deadline lost")
		}
	case err := <-done:
		t.Fatalf("returned before registry call: %v", err)
	case <-ctx.Done():
		t.Fatal("registry was not called")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("got %v, want cancellation", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("load did not stop")
	}
}

func TestInstancesContextNil(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("nil context did not panic")
		}
	}()
	load.InstancesContext(nil, nil, nil)
}
