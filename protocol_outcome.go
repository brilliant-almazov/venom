package venom

// The outcome a protocol file name ends with, after protocolOutcomeSep: the reader sees the result of the run in the
// list of files (owner 2026-11-16: «где статус в название файла? ok, failed. partial»).
const (
	protocolOutcomeSep     = "--"
	protocolOutcomeOK      = "ok"
	protocolOutcomeFailed  = "failed"
	protocolOutcomePartial = "partial"
)

// protocolOutcome is the outcome of a testcase for its file name: ok when it passed, partial when it failed while some
// of its assertions held, failed when it failed and none held; empty for a skipped testcase.
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

// protocolFile is the file name of the protocol of a testcase: protocolFileName, then the outcome when there is one.
func protocolFile(tc *TestCase) string {
	name := protocolFileName(tc.Name)
	if o := protocolOutcome(tc); o != "" {
		name += protocolOutcomeSep + o
	}
	return name + ".md"
}
