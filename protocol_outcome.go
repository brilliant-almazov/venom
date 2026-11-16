package venom

const (
	protocolOutcomeSep     = "--"
	protocolOutcomeOK      = "ok"
	protocolOutcomeFailed  = "failed"
	protocolOutcomePartial = "partial"
)

func protocolOutcome(tc *TestCase) string {
	switch tc.Status {
	case StatusPass:
		return protocolOutcomeOK
	case StatusFail:
		for _, r := range tc.TestStepResults {
			for _, a := range r.AssertionsApplied.Assertions {
				if a.IsOK {
					return protocolOutcomePartial
				}
			}
		}
		return protocolOutcomeFailed
	}
	return ""
}

func protocolFile(tc *TestCase) string {
	name := protocolFileName(tc.Name)
	if o := protocolOutcome(tc); o != "" {
		name += protocolOutcomeSep + o
	}
	return name + ".md"
}
