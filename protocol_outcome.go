package venom

const (
	protocolOutcomeSep     = "--"
	protocolOutcomeOK      = "ok"
	protocolOutcomeFailed  = "failed"
	protocolOutcomePartial = "partial"
)

// ProtocolOutcomeFailed is the outcome of a protocol whose testcase proved nothing: failed with no assertion held,
// skipped, interrupted before any assertion held, never started.
const ProtocolOutcomeFailed = protocolOutcomeFailed

// The outcomes ok and partial of a protocol, as a testsuite folder of the run reads them from its testcases.
const (
	ProtocolOutcomeOK      = protocolOutcomeOK
	ProtocolOutcomePartial = protocolOutcomePartial
)

// protocolOutcome is the outcome of every testcase, never empty: ok when it passed, partial when it did not pass and at
// least one assertion held, failed otherwise — skipped, interrupted and not run are not passed (nothing is skipped).
func protocolOutcome(tc *TestCase) string {
	if tc.Status == StatusPass {
		return protocolOutcomeOK
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
	return protocolFileName(tc.Name) + protocolOutcomeSep + protocolOutcome(tc) + ".md"
}
