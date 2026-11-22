package venom

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func selectCase(name string, steps ...string) TestCase {
	var tc TestCase
	tc.Name = name
	for _, s := range steps {
		tc.RawTestSteps = append(tc.RawTestSteps, json.RawMessage(s))
	}
	return tc
}

func selectRun(names []string, suites ...TestSuite) (*Venom, error) {
	v := New()
	v.TestCaseNames = names
	v.Tests.TestSuites = suites
	return v, v.selectTestCases()
}

func keptNames(v *Venom) map[string][]string {
	kept := map[string][]string{}
	for _, ts := range v.Tests.TestSuites {
		for _, tc := range ts.TestCases {
			kept[ts.Filepath] = append(kept[ts.Filepath], tc.Name)
		}
	}
	return kept
}

func selectFixture() []TestSuite {
	return []TestSuite{
		{Filepath: "a.yml", TestCases: []TestCase{
			selectCase("setup stand"),
			selectCase("C1 login", `{"type":"http","vars":{"token":{"from":"result.body"}}}`),
			selectCase("C2 profile", `{"type":"http","url":"{{.c1-login.token}}"}`),
			selectCase("C3 other"),
			selectCase("teardown stand"),
		}},
		{Filepath: "b.yml", TestCases: []TestCase{selectCase("C4 billing")}},
	}
}

func TestSelectNoNameKeepsEverything(t *testing.T) {
	v, err := selectRun(nil, selectFixture()...)
	require.NoError(t, err)
	require.Equal(t, map[string][]string{
		"a.yml": {"setup stand", "C1 login", "C2 profile", "C3 other", "teardown stand"},
		"b.yml": {"C4 billing"},
	}, keptNames(v))
}

func TestSelectKeepsNamedSetupTeardownAndDependencies(t *testing.T) {
	v, err := selectRun([]string{"C2 profile"}, selectFixture()...)
	require.NoError(t, err)
	require.Equal(t, map[string][]string{
		"a.yml": {"setup stand", "C1 login", "C2 profile", "teardown stand"},
	}, keptNames(v))
}

func TestSelectBySlugAcrossSuites(t *testing.T) {
	v, err := selectRun([]string{"c3-other", "C4 billing"}, selectFixture()...)
	require.NoError(t, err)
	require.Equal(t, map[string][]string{
		"a.yml": {"setup stand", "C3 other", "teardown stand"},
		"b.yml": {"C4 billing"},
	}, keptNames(v))
}

func TestSelectKeepsDependenciesTransitively(t *testing.T) {
	v, err := selectRun([]string{"C3"}, TestSuite{Filepath: "c.yml", TestCases: []TestCase{
		selectCase("C1"),
		selectCase("C2", `{"type":"http","url":"{{.c1.id}}"}`),
		selectCase("C3", `{"type":"http","url":"{{.c2.id}}"}`),
		selectCase("C4"),
	}})
	require.NoError(t, err)
	require.Equal(t, map[string][]string{"c.yml": {"C1", "C2", "C3"}}, keptNames(v))
}

func TestSelectUnknownNameFails(t *testing.T) {
	_, err := selectRun([]string{"C2 profile", "C9 missing"}, selectFixture()...)
	require.ErrorIs(t, err, ErrNoSuchTestCase)
	require.ErrorContains(t, err, `--testcase "C9 missing"`)
	require.ErrorContains(t, err, "a.yml, b.yml")
}
