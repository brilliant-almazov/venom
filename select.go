package venom

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gosimple/slug"
)

// ErrNoSuchTestCase is returned when a name given by --testcase matches no testcase of the testsuites read.
var ErrNoSuchTestCase = errors.New("no such testcase")

// selectTestCases keeps, in every testsuite, the testcases named in v.TestCaseNames, then the setup… and teardown…
// testcases of that testsuite, then every earlier testcase whose exported vars a kept testcase reads
// ({{.<testcase>.<var>}}), transitively. A testsuite holding none of the names is dropped from the run. A name
// matching no testcase of any testsuite is an error naming it and the testsuites read. No name keeps everything.
// A name matches a testcase name as written or as its slug.
func (v *Venom) selectTestCases() error {
	if len(v.TestCaseNames) == 0 {
		return nil
	}

	found := make(map[string]bool, len(v.TestCaseNames))
	kept := make([]TestSuite, 0, len(v.Tests.TestSuites))
	files := make([]string, 0, len(v.Tests.TestSuites))

	for i := range v.Tests.TestSuites {
		ts := v.Tests.TestSuites[i]
		files = append(files, ts.Filepath)

		cases := selectedTestCases(&ts, v.TestCaseNames, found)
		if len(cases) == 0 {
			continue
		}

		ts.TestCases = cases
		kept = append(kept, ts)
	}

	for _, name := range v.TestCaseNames {
		if !found[name] {
			return fmt.Errorf("--testcase %q: %w in %s", name, ErrNoSuchTestCase, strings.Join(files, ", "))
		}
	}

	v.Tests.TestSuites = kept

	return nil
}

// selectedTestCases returns the testcases of ts kept for names, in file order; none when no name matches in ts.
// Every name matched is set in found.
func selectedTestCases(ts *TestSuite, names []string, found map[string]bool) []TestCase {
	keep := make([]bool, len(ts.TestCases))
	named := false

	for i := range ts.TestCases {
		for _, name := range names {
			if testCaseNamed(ts.TestCases[i].Name, name) {
				keep[i], named = true, true
				found[name] = true
			}
		}
	}

	if !named {
		return nil
	}

	for i := range ts.TestCases {
		if isStandCase(ts, &ts.TestCases[i]) {
			keep[i] = true
		}
	}

	keepDependencies(ts, keep)

	cases := make([]TestCase, 0, len(ts.TestCases))
	for i := range ts.TestCases {
		if keep[i] {
			cases = append(cases, ts.TestCases[i])
		}
	}

	return cases
}

// keepDependencies keeps every earlier testcase whose exported vars a kept testcase reads, until nothing changes.
// Exported vars are named "<testcase slug>.<var>", so a read holds ".<testcase slug>.".
func keepDependencies(ts *TestSuite, keep []bool) {
	for changed := true; changed; {
		changed = false

		for i := range ts.TestCases {
			if !keep[i] {
				continue
			}

			text := testCaseText(&ts.TestCases[i])

			for j := 0; j < i; j++ {
				name := slug.Make(ts.TestCases[j].Name)
				if !keep[j] && name != "" && strings.Contains(text, "."+name+".") {
					keep[j], changed = true, true
				}
			}
		}
	}
}

// testCaseNamed reports whether a testcase name matches a name of --testcase, as written or as slug.
func testCaseNamed(testCase, name string) bool {
	return testCase == name || slug.Make(testCase) == slug.Make(name)
}
