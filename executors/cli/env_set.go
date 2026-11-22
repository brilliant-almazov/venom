package cli

import (
	"context"
	"errors"
	"maps"
	"slices"

	"github.com/mitchellh/mapstructure"

	"github.com/ovh/venom"
)

// EnvSetName is the executor that sets ENV values of one stand service and restarts it.
const EnvSetName = venom.EnvSetType

// standVar is the var naming the stand when the step gives no env; every runner passes --var stand=<env>.
const standVar = "stand"

// EnvSet sets ENV values of one stand service and restarts it through the stand utility, with no shell:
// bin var set <service> KEY=VAL… --restart --env <env>; the utility returns once the service is healthy, rc 0, or not,
// rc 10. Before the call it registers bin var reset <service> --restart --env <env> on the testcase: Venom runs it once
// the testcase ends, whatever its outcome, and on SIGINT or SIGTERM.
type EnvSet struct {
	Bin     string            `json:"bin" yaml:"bin" mapstructure:"bin"`
	Service string            `json:"service" yaml:"service" mapstructure:"service"`
	Values  map[string]string `json:"values" yaml:"values" mapstructure:"values"`
	Env     string            `json:"env" yaml:"env" mapstructure:"env"`
}

// NewEnvSet returns the env_set executor.
func NewEnvSet() venom.Executor {
	return &EnvSet{}
}

// ZeroValueResult returns an empty result.
func (EnvSet) ZeroValueResult() interface{} {
	return Result{}
}

// GetDefaultAssertions: the service took the values and is healthy.
func (EnvSet) GetDefaultAssertions() *venom.StepAssertions {
	return &venom.StepAssertions{Assertions: []venom.Assertion{"result.code ShouldEqual 0"}}
}

// Run registers the reset of the service on the testcase, then sets its values and restarts it.
func (EnvSet) Run(ctx context.Context, step venom.TestStep) (interface{}, error) {
	var e EnvSet
	if err := mapstructure.WeakDecode(step, &e); err != nil {
		return nil, err
	}
	e.Env = e.envOf(ctx)
	set, reset, err := e.argv()
	if err != nil {
		return nil, err
	}
	bin := e.Bin
	if bin == "" {
		bin = DefaultBin
	}
	r := venom.EnvReset{Service: e.Service, Bin: bin, Dir: venom.StringVarFromCtx(ctx, workdirVar), Argv: reset}
	if err := venom.RegisterEnvReset(ctx, r); err != nil {
		return nil, err
	}
	return run(ctx, bin, set)
}

// envOf is the env of the step, the var stand when the step gives none.
func (e EnvSet) envOf(ctx context.Context) string {
	if e.Env != "" {
		return e.Env
	}
	return venom.StringVarFromCtx(ctx, standVar)
}

// argv is the argv of var set, the keys sorted, and of var reset.
func (e EnvSet) argv() ([]string, []string, error) {
	if e.Service == "" {
		return nil, nil, errors.New("env_set: service is empty")
	}
	if len(e.Values) == 0 {
		return nil, nil, errors.New("env_set: values is empty")
	}
	if e.Env == "" {
		return nil, nil, errors.New("env_set: env is empty and the var stand is not set")
	}
	set := []string{"var", "set", e.Service}
	for _, k := range slices.Sorted(maps.Keys(e.Values)) {
		set = append(set, k+"="+e.Values[k])
	}
	set = append(set, "--restart", "--env", e.Env)
	return set, []string{"var", "reset", e.Service, "--restart", "--env", e.Env}, nil
}
