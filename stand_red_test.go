package venom

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// A failed testcase is red for the stand, not for the product, when it prepares the stand, a step of the stand utility
// failed, a precondition did not hold, or its only failure is the span tree of the call not saved.
func TestStandRed(t *testing.T) {
	suite := &TestSuite{Filepath: "scenarios/supabase/SB-01.yml"}
	step := func(typ string, status Status, info []string, errs ...string) TestStepResult {
		r := TestStepResult{Name: typ, Status: status, Interpolated: []byte("type: " + typ + "\n"), ComputedInfo: info}
		for _, e := range errs {
			r.Errors = append(r.Errors, Failure{Value: e})
		}
		return r
	}
	failed := func(name string, steps ...TestStepResult) *TestCase {
		tc := &TestCase{TestCaseInput: TestCaseInput{Name: name}, Status: StatusFail, TestStepResults: steps}
		return tc
	}
	gate := "trace 0af7651916cd43dd8448eb211c80319c " + traceGateWords + ": /tmp/x.md"

	cases := []struct {
		name  string
		suite *TestSuite
		tc    *TestCase
		want  string
	}{
		{"setup testcase", suite, failed("Setup seed", step("http", StatusFail, nil, "expected 200 got 500")), StandSetupCase},
		{"preflight file", &TestSuite{Filepath: "x/scenarios/preflight/health.yml"},
			failed("SB-00 health", step("http", StatusFail, nil, "expected 200 got 503")), StandSetupCase},
		{"stand step", suite, failed("SB-01-01 seed", step("seed", StatusFail, nil, "exit 1"), step("http", StatusPass, nil)),
			`the stand step 1 "seed" failed`},
		{"precondition", suite, failed("SB-01-02 read", step("http", StatusFail, []string{"precondition: link · id present"},
			"expected 200 got 404")), "the precondition link · id did not hold"},
		{"trace not saved", suite, failed("SB-01-03 call", step("http", StatusFail, nil, gate)), StandNoTree},
		{"product defect", suite, failed("SB-01-04 call", step("http", StatusFail, nil, gate, "expected 200 got 409")), ""},
		{"passed", suite, &TestCase{TestCaseInput: TestCaseInput{Name: "SB-01-05"}, Status: StatusPass}, ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, standRed(c.suite, c.tc))
		})
	}
}
