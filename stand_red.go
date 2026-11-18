package venom

import (
	"fmt"
	"path/filepath"
	"strings"
)

// The reasons a failed testcase is red for the stand set up wrong, not for the product (owner 2026-11-18: «Фейлит,
// потому что криво настроен стенд, это не фейлит»): PROBLEMS.md leaves it out.
const (
	StandSetupCase    = "a setup or teardown testcase failed"
	StandStepFailed   = "the stand step %d %q failed"
	StandPrecondition = "the precondition %s · %s did not hold"
	StandNoTree       = "the span tree of the call was not saved"
)

// traceGateWords are the words of the failure traceGate gives a call whose span tree was not saved, traceGateFailure
// the failure.
const (
	traceGateWords   = "returned by the call has no saved span tree"
	traceGateFailure = "trace %s " + traceGateWords + ": %s"
)

// standStepTypes are the steps of the stand utility (executors/cli: cli, seed, stand_status).
var standStepTypes = map[string]bool{"cli": true, "seed": true, "stand_status": true}

// isStandCase: a setup… or teardown… testcase, or any testcase of a file under scenarios/preflight/, prepares the
// stand and tests no product.
func isStandCase(ts *TestSuite, tc *TestCase) bool {
	name := strings.ToLower(tc.Name)

	return strings.HasPrefix(name, "setup") || strings.HasPrefix(name, "teardown") ||
		strings.Contains(filepath.ToSlash(ts.Filepath), "scenarios/preflight/")
}

// standRed is why the failed testcase is red for the stand and not for the product: a setup, teardown or preflight
// testcase, a failed step of the stand utility, a precondition that did not hold, or a call whose only failure is its
// span tree not saved; empty for a testcase that passed or failed on the product.
func standRed(ts *TestSuite, tc *TestCase) string {
	if tc.Status != StatusFail {
		return ""
	}

	if isStandCase(ts, tc) {
		return StandSetupCase
	}

	for i := range tc.TestStepResults {
		r := &tc.TestStepResults[i]
		if typ, _ := protocolStep(r)["type"].(string); r.Status == StatusFail && standStepTypes[typ] {
			return fmt.Sprintf(StandStepFailed, i+1, r.Name)
		}
	}

	for _, row := range protocolPresenceRows(tc) {
		if !protocolPreconditionHeld(tc, row.step) {
			return fmt.Sprintf(StandPrecondition, row.entity, row.key)
		}
	}

	if onlyTraceGate(tc) {
		return StandNoTree
	}

	return ""
}

// onlyTraceGate: every failure of the testcase is that of traceGate.
func onlyTraceGate(tc *TestCase) bool {
	seen := false

	for i := range tc.TestStepResults {
		for _, e := range tc.TestStepResults[i].Errors {
			if !strings.Contains(e.Value, traceGateWords) {
				return false
			}

			seen = true
		}
	}

	return seen
}
