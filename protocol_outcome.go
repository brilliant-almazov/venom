package venom

import (
	_ "embed"
	"fmt"

	"github.com/rockbears/yaml"
)

const (
	protocolOutcomeSep     = "--"
	protocolOutcomeOK      = "ok"
	protocolOutcomeFailed  = "failed"
	protocolOutcomePartial = "partial"
	protocolOutcomeNotRun  = "not-run"
)

// ProtocolOutcomeFailed is the outcome of a protocol whose testcase ended failed with no assertion held.
const ProtocolOutcomeFailed = protocolOutcomeFailed

// ProtocolOutcomeNotRun is the outcome of a protocol whose testcase did not run or has not ended: skipped, interrupted,
// never started, pending; it is never failed.
const ProtocolOutcomeNotRun = protocolOutcomeNotRun

// The outcomes ok and partial of a protocol, as a testsuite folder of the run reads them from its testcases.
const (
	ProtocolOutcomeOK      = protocolOutcomeOK
	ProtocolOutcomePartial = protocolOutcomePartial
)

// protocolOutcomes are every outcome a testcase may have.
var protocolOutcomes = []string{protocolOutcomeOK, protocolOutcomePartial, protocolOutcomeFailed, protocolOutcomeNotRun}

// baseConfig is the base configuration of Venom, venomrc.base.yaml: what a run takes when no .venomrc sets the key.
//
//go:embed venomrc.base.yaml
var baseConfig []byte

// baseRC is venomrc.base.yaml as read: the outcomes, the criticality scale and the kinds of a scenario.
type baseRC struct {
	Outcomes    []string       `yaml:"outcomes"`
	Criticality []ProblemLevel `yaml:"criticality"`
	Kinds       []ProblemLevel `yaml:"kinds"`
}

// mustBase is venomrc.base.yaml as read; the binary carries it, so an error in it is a broken build.
func mustBase() baseRC {
	var base baseRC
	if err := yaml.Unmarshal(baseConfig, &base); err != nil {
		panic(fmt.Sprintf("venomrc.base.yaml: %v", err))
	}
	return base
}

// writtenOutcomes are the outcomes written in the names of the protocols and of their testsuite folders, the key
// outcomes of venomrc.base.yaml until a .venomrc sets its own (SetProtocolOutcomes).
var writtenOutcomes = mustBaseOutcomes()

func mustBaseOutcomes() map[string]bool {
	written, err := outcomeSet(mustBase().Outcomes)
	if err != nil {
		panic(fmt.Sprintf("venomrc.base.yaml: %v", err))
	}
	return written
}

// SetProtocolOutcomes sets the outcomes written in the names of the protocols and of their testsuite folders, the key
// outcomes of a .venomrc; an outcome left out is not written. An outcome that is none of ok, partial, failed, not-run
// is an error.
func SetProtocolOutcomes(outcomes []string) error {
	written, err := outcomeSet(outcomes)
	if err != nil {
		return err
	}
	writtenOutcomes = written
	return nil
}

func outcomeSet(outcomes []string) (map[string]bool, error) {
	set := map[string]bool{}
	for _, o := range outcomes {
		known := false
		for _, k := range protocolOutcomes {
			known = known || o == k
		}
		if !known {
			return nil, fmt.Errorf("unknown outcome %q: the outcomes are %v", o, protocolOutcomes)
		}
		set[o] = true
	}
	return set, nil
}

// protocolOutcome is the outcome of every testcase, never empty: ok when it passed, partial when it failed and at least
// one assertion held, failed when it failed and none held, not-run when it did not run or has not ended — skipped,
// interrupted, never started.
func protocolOutcome(tc *TestCase) string {
	if tc.Status == StatusPass {
		return protocolOutcomeOK
	}
	if tc.Status != StatusFail {
		return protocolOutcomeNotRun
	}
	for _, r := range tc.TestStepResults {
		for _, a := range r.AssertionsApplied.Assertions {
			if a.IsOK {
				return protocolOutcomePartial
			}
		}
	}
	return protocolOutcomeFailed
}

func protocolFile(tc *TestCase) string {
	o := protocolOutcome(tc)
	if !writtenOutcomes[o] {
		return protocolFileName(tc.Name) + ".md"
	}
	return protocolFileName(tc.Name) + protocolOutcomeSep + o + ".md"
}
