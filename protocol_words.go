package venom

// The words of a protocol, in one place: the writer prints them and the tests compare against these names. The fork
// is English only, its protocols too (owner 2026-11-15).
const (
	protocolHeadRight      = "## Expected behaviour"
	protocolHeadWrong      = "## What is wrong"
	protocolHeadBefore     = "## State before"
	protocolHeadPresence   = "## Data presence"
	protocolHeadRequest    = "## Request"
	protocolHeadAnswer     = "## Response"
	protocolHeadAfter      = "## State after"
	protocolHeadTrace      = "## Trace"
	protocolHeadVerdict    = "## Verdict"
	protocolPassed         = "✅ passed"
	protocolFailed         = "❌ failed"
	protocolSkipped        = "⏭ skipped"
	protocolWordPresent    = "present"
	protocolWordAbsent     = "absent"
	protocolDataPresent    = "data present"
	protocolDataAbsent     = "data absent"
	protocolNotConfirmed   = "❌ not confirmed"
	protocolNoPrecondition = "no precondition"
	protocolPresenceHeader = "| entity | key | request | present/absent | trace id | key in the tested call |"
	protocolKeySent        = "✅ sent"
	protocolKeyNotSent     = "❌ not sent"
	protocolStepLine       = "- step: %d \"%s\""
	protocolAfterStep      = "### step %d \"%s\" %s"
	protocolMethodLine     = "- method: "
	protocolMasked         = "*** (%d chars)"
	protocolNoTrace        = "no trace"
	protocolNoTree         = "- no tree:"
)
