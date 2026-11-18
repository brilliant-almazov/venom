package venom

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The criticality scale and the kinds come from venomrc.base.yaml; a .venomrc replaces either, a level with no name is
// an error; a level the scale does not have and «not set» go after every level.
func TestProblemScalesFromConfig(t *testing.T) {
	t.Cleanup(func() { problemCriticality, problemKinds = mustBase().Criticality, mustBase().Kinds })

	assert.Equal(t, 0, CriticalityRank("critical"))
	assert.Equal(t, 2, CriticalityRank("tolerable"))
	assert.Equal(t, 3, CriticalityRank("unknown"))
	assert.Equal(t, 4, CriticalityRank(ProblemNotSet))
	assert.Equal(t, 0, KindRank("business"))
	assert.Equal(t, "qa (a scenario of QA only, may wait)", KindText("qa"))
	assert.Equal(t, "unknown (not in the scale)", KindText("unknown"))

	require.NoError(t, SetProblemScales([]ProblemLevel{{Name: "minor"}, {Name: "blocker", Means: "at once"}}, nil))
	assert.Equal(t, 1, CriticalityRank("blocker"))
	assert.Equal(t, "blocker (at once)", CriticalityText("blocker"))
	assert.Equal(t, 0, KindRank("business"))

	assert.Error(t, SetProblemScales(nil, []ProblemLevel{{Means: "no name"}}))
}
