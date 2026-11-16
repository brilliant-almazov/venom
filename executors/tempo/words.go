package tempo

const (
	wordHeading  = "### Trace `%s`"
	wordTable    = "| trace_id | root | duration, ms | spans | ❌ | source |"
	wordSummary  = "summary: ✅ %d spans · ❌ %d spans · %s"
	wordFirst    = "first problem: "
	wordNone     = "no problems"
	wordProblem  = "problem: "
	wordEvent    = "event: "
	wordMs       = "ms"
	wordSQL      = "SQL: "
	wordSQLAfter = "     "
	wordSQLSame  = "same SQL as the span above"
)
