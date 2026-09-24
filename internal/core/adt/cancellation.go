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

package adt

import (
	"context"

	"cuelang.org/go/cue/errors"
	"cuelang.org/go/internal"
)

// SetContext sets the Go context before an operation starts. Contexts without
// cancellation keep the evaluator's default fast path.
func (c *OpContext) SetContext(ctx context.Context) {
	c.goContext = ctx
	c.done = ctx.Done()
}

// Context returns the Go context for this operation, including request values.
func (c *OpContext) Context() context.Context {
	if c.goContext == nil {
		return context.Background()
	}
	return c.goContext
}

// Cancelled reports cancellation independently of CUE errors: a disjunction or
// a try clause may discard a CUE error, but cannot undo cancellation. It does
// not modify vertices, including completed vertices shared by other runtimes.
func (c *OpContext) Cancelled() *Bottom {
	if c == nil || c.done == nil {
		return nil
	}
	if c.cancelled != nil {
		return c.cancelled
	}
	select {
	case <-c.done:
		c.cancelled = &Bottom{Err: errors.Promote(internal.ContextError(c.goContext), "")}
		return c.cancelled
	default:
		return nil
	}
}
