// Package cli runs the stand utility from declared fields with no script text, and exposes that as the `cli`, `seed`,
// `stand_status` and `env_set` executors (owner 2026-11-15: scenarios are declarative only).
//
// The utility is started directly, never through a shell: argv is the words of `action`, then `args`, then
// `--env <env>`. `seed` is the action seed with `--block <block>`, `stand_status` the action status; both take the
// same fields as `cli`. The result carries systemout, its parsed JSON when it is JSON (the read-back of a seed),
// systemerr and the exit code; `output_file` writes systemout, then systemerr, under the venom output dir.
//
// An action health also gives result.red, its ✗ rows of systemout and systemerr, the leading spaces and the trailing
// duration removed, sorted; and result.red_new, the red rows not in the red rows of `baseline`, the output of an earlier
// health step. Both are one row per line, empty when none.
package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/mitchellh/mapstructure"

	"github.com/ovh/venom"
	"github.com/ovh/venom/executors/evidence"
)

// Names of the executors.
const (
	Name            = "cli"
	SeedName        = "seed"
	StandStatusName = "stand_status"
)

// DefaultBin is the stand utility when the step gives no bin, relative to the testsuite workdir.
const DefaultBin = "bin/cli"

// workdirVar is the var holding the testsuite workdir, where the utility starts.
const workdirVar = "venom.testsuite.workdir"

// New returns the cli executor.
func New() venom.Executor {
	return &Executor{}
}

// NewSeed returns the seed executor: action seed with --block.
func NewSeed() venom.Executor {
	return &Executor{fixed: SeedName}
}

// NewStandStatus returns the stand_status executor: action status.
func NewStandStatus() venom.Executor {
	return &Executor{fixed: "status"}
}

// Executor runs one action of the stand utility: bin, action, args, env, block (seed), output_file.
type Executor struct {
	Bin        string   `json:"bin" yaml:"bin" mapstructure:"bin"`
	Action     string   `json:"action" yaml:"action" mapstructure:"action"`
	Args       []string `json:"args" yaml:"args" mapstructure:"args"`
	Env        string   `json:"env" yaml:"env" mapstructure:"env"`
	Block      string   `json:"block" yaml:"block" mapstructure:"block"`
	OutputFile string   `json:"output_file" yaml:"output_file" mapstructure:"output_file"`
	Baseline   string   `json:"baseline" yaml:"baseline" mapstructure:"baseline"`
	fixed      string
}

// Result of the executor.
type Result struct {
	Systemout     string      `json:"systemout" yaml:"systemout"`
	SystemoutJSON interface{} `json:"systemoutjson,omitempty" yaml:"systemoutjson,omitempty"`
	Systemerr     string      `json:"systemerr" yaml:"systemerr"`
	Code          string      `json:"code" yaml:"code"`
	OutputFile    string      `json:"output_file,omitempty" yaml:"output_file,omitempty"`
	Red           string      `json:"red,omitempty" yaml:"red,omitempty"`
	RedNew        string      `json:"red_new,omitempty" yaml:"red_new,omitempty"`
}

// healthAction is the action whose result carries the red rows.
const healthAction = "health"

// redDuration is the trailing duration of a health row, " 0.42 s".
var redDuration = regexp.MustCompile(` *[0-9.]* s$`)

// Red is the ✗ rows of a health output, the leading spaces and the trailing duration removed, sorted.
func Red(out string) []string {
	var rows []string
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, "✗") {
			rows = append(rows, redDuration.ReplaceAllString(strings.TrimLeft(l, " \t"), ""))
		}
	}
	sort.Strings(rows)
	return rows
}

// RedNew is the rows of red not in the red rows of baseline.
func RedNew(red []string, baseline string) []string {
	seen := map[string]bool{}
	for _, r := range Red(baseline) {
		seen[r] = true
	}
	var rows []string
	for _, r := range red {
		if !seen[r] {
			rows = append(rows, r)
		}
	}
	return rows
}

// ZeroValueResult returns an empty result.
func (Executor) ZeroValueResult() interface{} {
	return Result{}
}

// GetDefaultAssertions: the utility exits 0.
func (Executor) GetDefaultAssertions() *venom.StepAssertions {
	return &venom.StepAssertions{Assertions: []venom.Assertion{"result.code ShouldEqual 0"}}
}

// Run starts the utility with the argv of the step.
func (x Executor) Run(ctx context.Context, step venom.TestStep) (interface{}, error) {
	var e Executor
	if err := mapstructure.WeakDecode(step, &e); err != nil {
		return nil, err
	}
	e.fixed = x.fixed
	argv, err := e.argv()
	if err != nil {
		return nil, err
	}
	bin := e.Bin
	if bin == "" {
		bin = DefaultBin
	}
	r, err := run(ctx, bin, argv)
	if err != nil {
		return nil, err
	}
	if e.fixed == "" && strings.Fields(e.Action)[0] == healthAction {
		red := Red(r.Systemout + "\n" + r.Systemerr)
		r.Red, r.RedNew = strings.Join(red, "\n"), strings.Join(RedNew(red, e.Baseline), "\n")
	}
	if r.OutputFile, err = evidence.Write(ctx, e.OutputFile, r.Systemout+r.Systemerr); err != nil {
		return nil, err
	}
	return r, nil
}

// run starts bin with argv in the testsuite workdir, with no shell: the result carries its stdout, its parsed JSON when
// it is JSON, its stderr and its exit code.
func run(ctx context.Context, bin string, argv []string) (Result, error) {
	cmd := exec.CommandContext(ctx, bin, argv...)
	cmd.Dir = venom.StringVarFromCtx(ctx, workdirVar)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	r := Result{Code: "0"}
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			return Result{}, fmt.Errorf("cli: %s: %w", bin, err)
		}
		r.Code = strconv.Itoa(exit.ExitCode())
	}
	r.Systemout = venom.RemoveNotPrintableChar(strings.TrimRight(stdout.String(), "\n"))
	r.Systemerr = venom.RemoveNotPrintableChar(strings.TrimRight(stderr.String(), "\n"))
	var parsed interface{}
	if err := venom.JSONUnmarshal([]byte(r.Systemout), &parsed); err == nil {
		r.SystemoutJSON = parsed
	}
	return r, nil
}

// argv is the words of the action, the args, --block for seed and --env.
func (e Executor) argv() ([]string, error) {
	action := e.Action
	if e.fixed != "" {
		action = e.fixed
	}
	words := strings.Fields(action)
	if len(words) == 0 {
		return nil, errors.New("cli: action is empty")
	}
	if e.Env == "" {
		return nil, errors.New("cli: env is empty")
	}
	argv := append(words, e.Args...)
	if e.fixed == SeedName {
		if e.Block == "" {
			return nil, errors.New("seed: block is empty")
		}
		argv = append(argv, "--block", e.Block)
	}
	return append(argv, "--env", e.Env), nil
}
