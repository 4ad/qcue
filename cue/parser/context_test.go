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

package parser_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"cuelang.org/go/cue/parser"
)

type cancelAfterChecks struct {
	context.Context
	cancel    context.CancelFunc
	remaining int
}

func (c *cancelAfterChecks) Err() error {
	c.remaining--
	if c.remaining == 0 {
		c.cancel()
	}
	return c.Context.Err()
}

func TestContext(t *testing.T) {
	for _, expr := range []bool{false, true} {
		t.Run(map[bool]string{false: "file", true: "expr"}[expr], func(t *testing.T) {
			parse := func(ctx context.Context) error {
				src := "[" + strings.Repeat("1,", 10000) + "]"
				if expr {
					_, err := parser.ParseExpr("", src, parser.WithContext(ctx))
					return err
				}
				_, err := parser.ParseFile("", src, parser.WithContext(ctx))
				return err
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			controlled := &cancelAfterChecks{Context: ctx, cancel: cancel, remaining: 100}
			if err := parse(controlled); !errors.Is(err, context.Canceled) {
				t.Fatalf("got %v, want canceled", err)
			}
			if controlled.remaining < -10 {
				t.Fatalf("continued parsing after cancellation: %d", controlled.remaining)
			}
			expired, stop := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
			defer stop()
			if err := parse(expired); !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("got %v, want deadline", err)
			}
			cause := errors.New("request ended")
			caused, stopCause := context.WithCancelCause(context.Background())
			stopCause(cause)
			if err := parse(caused); !errors.Is(err, cause) || !errors.Is(err, context.Canceled) {
				t.Fatalf("cause lost: %v", err)
			}
			if err := parse(context.Background()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

type unreadable struct{ t *testing.T }

func (r unreadable) Read([]byte) (int, error) { r.t.Fatal("read after cancellation"); return 0, nil }

func TestContextBeforeRead(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := parser.ParseFile("", unreadable{t}, parser.WithContext(ctx)); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := parser.ParseExpr("", unreadable{t}, parser.WithContext(ctx)); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestContextNil(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("nil context did not panic")
		}
	}()
	parser.WithContext(nil)
}
