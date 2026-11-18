package runartifacts

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ovh/venom"
)

// The problems go by the kind of their scenario, then by their criticality, as venomrc.base.yaml sets them: business
// first, then technical, then qa, then not set; within a kind critical, important, tolerable, not set.
func TestProblemsByPriority(t *testing.T) {
	failed := func(name, kind, criticality string) row {
		return row{testcase: name, state: stateDone, view: venom.ProtocolView{Mark: venom.MarkFailed, Kind: kind, Criticality: criticality}}
	}
	rows := []row{
		failed("SB-01 qa", "qa", "critical"),
		failed("SB-02 none", "", ""),
		{testcase: "SB-03 green", state: stateDone, view: venom.ProtocolView{Mark: venom.MarkPassed, Kind: "business"}},
		failed("SB-04 business tolerable", "business", "tolerable"),
		failed("SB-05 technical", "technical", "important"),
		failed("SB-06 business critical", "business", "critical"),
	}

	var names []string
	for _, r := range problemsOf(rows) {
		names = append(names, r.testcase)
	}

	assert.Equal(t, []string{"SB-06 business critical", "SB-04 business tolerable", "SB-05 technical", "SB-01 qa", "SB-02 none"}, names)
	assert.Equal(t, "P-SB-02-none", problemID(rows[1]))
	assert.Equal(t, "problem: P-SB-02-none · kind: not set · criticality: not set", problemLine(rows[1]))
	assert.Equal(t, "problem: P-SB-06-business-critical · kind: business (a business scenario, in priority) · criticality: critical (blocks the product, fixed at once)",
		problemLine(rows[5]))
}

// The culprits of the traces of a testcase in two services make the zone of fix «both».
func TestFixBoth(t *testing.T) {
	one := bothOf([]fixer{{service: "svc-a", span: "A/x", trace: "t1"}, {service: "svc-a", span: "A/y", trace: "t2"}})
	assert.Equal(t, "svc-a · A/x", one.String())

	both := bothOf([]fixer{{service: "svc-a", span: "A/x", trace: "t1"}, {service: "svc-b", span: "B/y", trace: "t2"}})
	assert.Equal(t, "both: svc-a + svc-b · A/x + B/y", both.String())
	assert.Equal(t, "both: svc-a + svc-b · A/x + B/y — the deepest failed span of each trace of the testcase",
		fixDetail(row{state: stateDone, fixer: both, view: venom.ProtocolView{Mark: venom.MarkFailed}}))
}
