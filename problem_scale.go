package venom

import "fmt"

// ProblemLevel is one level of a scale of the problems of a run: a criticality or a kind of scenario, its name as a
// testcase declares it and what it means, as venomrc.base.yaml or a .venomrc sets them.
type ProblemLevel struct {
	Name  string `json:"name" yaml:"name"`
	Means string `json:"means" yaml:"means"`
}

// ProblemNotSet is the criticality or the kind of a testcase that declares none.
const ProblemNotSet = "not set"

// The info keys a testcase declares its criticality and the kind of its scenario by.
const (
	infoCriticality = "criticality"
	infoKind        = "kind"
)

// problemCriticality and problemKinds are the scales of the problems from the first to fix, the keys criticality and
// kinds of venomrc.base.yaml until a .venomrc sets its own (SetProblemScales).
var problemCriticality, problemKinds = mustBase().Criticality, mustBase().Kinds

// SetProblemScales sets the criticality scale and the kinds of a scenario, each from the first to fix, the keys
// criticality and kinds of a .venomrc; nil keeps the scale as it is. A level with no name is an error.
func SetProblemScales(criticality, kinds []ProblemLevel) error {
	for _, l := range append(append([]ProblemLevel{}, criticality...), kinds...) {
		if l.Name == "" {
			return fmt.Errorf("a level of criticality or kinds has no name: %+v", l)
		}
	}
	if criticality != nil {
		problemCriticality = criticality
	}
	if kinds != nil {
		problemKinds = kinds
	}
	return nil
}

// CriticalityRank and KindRank are the place of a declared level in its scale, 0 the first to fix; a level the scale
// does not have and «not set» come after every level, in that order.
func CriticalityRank(name string) int { return levelRank(problemCriticality, name) }

func KindRank(name string) int { return levelRank(problemKinds, name) }

// CriticalityText and KindText are a declared level with what it means, «critical (blocks the product, fixed at
// once)», a level the scale does not have as «<name> (not in the scale)», «not set».
func CriticalityText(name string) string { return levelText(problemCriticality, name) }

func KindText(name string) string { return levelText(problemKinds, name) }

func levelRank(scale []ProblemLevel, name string) int {
	if name == ProblemNotSet {
		return len(scale) + 1
	}
	for i, l := range scale {
		if l.Name == name {
			return i
		}
	}
	return len(scale)
}

func levelText(scale []ProblemLevel, name string) string {
	if name == ProblemNotSet {
		return name
	}
	for _, l := range scale {
		if l.Name == name {
			if l.Means == "" {
				return name
			}
			return name + " (" + l.Means + ")"
		}
	}
	return name + " (not in the scale)"
}

// declaredLevel is the first value of the info key of the testcase, «not set» when it declares none.
func declaredLevel(info map[string][]string, key string) string {
	if v := info[key]; len(v) > 0 && v[0] != "" {
		return v[0]
	}
	return ProblemNotSet
}
