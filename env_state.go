package venom

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// envState is the resets of the testcases running now, and whether a reset failed: the stand is then off its base
// state and the testcases after it do not run. The copies of Venom a parallel run makes share it.
type envState struct {
	mu      sync.Mutex
	running map[*envResets]struct{}
	dirty   bool
}

func (s *envState) begin() *envResets {
	rs := &envResets{}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running == nil {
		s.running = map[*envResets]struct{}{}
	}
	s.running[rs] = struct{}{}

	return rs
}

func (s *envState) end(rs *envResets) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.running, rs)
}

func (s *envState) markDirty() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.dirty = true
}

func (s *envState) isDirty() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.dirty
}

// pending is the resets of every testcase running now, the last registered of each first.
func (s *envState) pending() []EnvReset {
	s.mu.Lock()
	defer s.mu.Unlock()

	var all []EnvReset
	for rs := range s.running {
		all = append(all, rs.reversed()...)
	}

	return all
}

// beginEnvResets gives ctx the resets of the testcase about to run.
func (v *Venom) beginEnvResets(ctx context.Context) (context.Context, *envResets) {
	rs := v.env.begin()

	return context.WithValue(ctx, envResetsKey{}, rs), rs
}

// restoreEnv runs the resets of tc, the last registered first, each as a step result after the steps of the scenario;
// a failed one fails tc and stops the run. It runs on a context that is never canceled: the reset goes even when the
// testcase was canceled.
func (v *Venom) restoreEnv(ctx context.Context, tc *TestCase, rs *envResets) {
	defer v.env.end(rs)

	ctx = context.WithoutCancel(ctx)

	for i, r := range rs.reversed() {
		start := time.Now()
		out, errOut, fault := r.run(ctx)
		res := TestStepResult{Name: "env reset " + r.Service, Number: len(tc.RawTestSteps) + i + 1, Status: StatusPass,
			Systemout: out, Systemerr: errOut, Start: start, End: time.Now()}
		res.Duration = res.End.Sub(res.Start).Seconds()

		if fault != "" {
			res.appendError(fmt.Errorf("base state not restored: %s: %s", r.Service, fault))
			v.env.markDirty()
		}

		tc.TestStepResults = append(tc.TestStepResults, res)
	}
}

// RestoreEnv runs at once the resets of every testcase still running, for a run ended by a signal before its
// testcases reach their end. It returns one line per service not brought back: "<service>: <why>".
func (v *Venom) RestoreEnv() []string {
	ctx := context.Background()

	var failed []string
	for _, r := range v.env.pending() {
		if _, _, fault := r.run(ctx); fault != "" {
			failed = append(failed, r.Service+": "+fault)
		}
	}

	return failed
}
