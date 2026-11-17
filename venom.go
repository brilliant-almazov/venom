package venom

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"plugin"
	"reflect"
	"sort"
	"strings"

	"github.com/confluentinc/bincover"
	"github.com/fatih/color"
	"github.com/ovh/venom/assertions"
	"github.com/pkg/errors"
	"github.com/spf13/cast"
)

var (
	// Version is set with -ldflags "-X github.com/ovh/venom/venom.Version=$(VERSION)"
	Version = "snapshot"
	IsTest  = ""
)

// OSExit is a wrapper for os.Exit
func OSExit(exitCode int) {
	if IsTest != "" {
		bincover.ExitCode = exitCode
	} else {
		os.Exit(exitCode)
	}
}

// ContextKey can be added in context to store contextual infos. Also used by logger.
type ContextKey string

// New instantiates a new venom on venom run cmd
func New() *Venom {
	v := &Venom{
		LogOutput:        os.Stdout,
		PrintFunc:        fmt.Printf,
		executorsBuiltin: map[string]Executor{},
		executorsPlugin:  map[string]Executor{},
		executorsUser:    map[string]Executor{},
		variables:        map[string]interface{}{},
		secrets:          map[string]interface{}{},
		OutputFormat:     "xml",
	}
	return v
}

type Venom struct {
	LogOutput io.Writer

	PrintFunc        func(format string, a ...interface{}) (n int, err error)
	executorsBuiltin map[string]Executor
	executorsPlugin  map[string]Executor
	executorsUser    map[string]Executor

	Tests     Tests
	variables H
	secrets   H

	LibDir        string
	OutputFormat  string
	OutputDir     string
	StopOnFailure bool
	HtmlReport    bool
	Verbose       int
	// Parallel is the number of testcases of one testsuite run at once; 0 or 1 is sequential.
	Parallel int
	// ParallelSuites is the number of testsuites run at once; 0 or 1 is sequential.
	ParallelSuites int
	// ProtocolDir, when set, receives one protocol file per testcase, whatever its outcome (protocol.go).
	ProtocolDir string
	// Artifacts, when set, receives every testcase of the run and the run once it ends (run_artifacts.go).
	Artifacts RunArtifacts
	// DryRun loads and checks the testsuites with no step run (dryrun.go).
	DryRun bool
	// AsWritten reads the testsuites with no var filled, each {{.x}} kept as the scenario writes it (plan.go).
	AsWritten bool
}

var trace = color.New(color.Attribute(90)).SprintFunc()

func (v *Venom) Print(format string, a ...interface{}) {
	v.PrintFunc(format, a...) // nolint
}

func (v *Venom) Println(format string, a ...interface{}) {
	v.PrintFunc(format+"\n", a...) // nolint
}

func (v *Venom) PrintlnTrace(s string) {
	v.PrintlnIndentedTrace(s, "")
}

func (v *Venom) PrintlnIndentedTrace(s string, indent string) {
	v.Println("\t  %s%s %s", indent, trace("[trac]"), trace(s)) // nolint
}

func (v *Venom) AddVariables(variables map[string]interface{}) {
	for k, variable := range variables {
		v.variables[k] = variable
	}
}

func (v *Venom) AddSecrets(secrets map[string]interface{}) {
	for k, s := range secrets {
		v.secrets[k] = s
	}
}

// RegisterExecutorBuiltin register builtin executors
func (v *Venom) RegisterExecutorBuiltin(name string, e Executor) {
	v.executorsBuiltin[name] = e
}

// RegisterExecutorPlugin register plugin executors
func (v *Venom) RegisterExecutorPlugin(name string, e Executor) {
	v.executorsPlugin[name] = e
}

// RegisterExecutorUser registers an user executor
func (v *Venom) RegisterExecutorUser(name string, e Executor) error {
	if existing, ok := v.executorsUser[name]; ok {
		return fmt.Errorf("executor %q already exists (from file %q)", name, existing.(UserExecutor).Filename)
	}

	v.executorsUser[name] = e
	return nil
}

// GetExecutorRunner initializes a test according to its type
// if no type is provided, exec is default
func (v *Venom) GetExecutorRunner(ctx context.Context, ts TestStep, h H) (context.Context, ExecutorRunner, error) {
	name, _ := ts.StringValue("type")
	script, _ := ts.StringValue("script")
	command, _ := ts.StringSliceValue("command")
	if name == "" && (script != "" || len(command) != 0) {
		name = "exec"
	}
	retry, err := ts.IntValue("retry")
	if err != nil {
		return nil, nil, err
	}
	retryIf, err := ts.StringSliceValue("retry_if")
	if err != nil {
		return nil, nil, err
	}
	delay, err := ts.DurationValue("delay")
	if err != nil {
		return nil, nil, err
	}
	timeout, err := ts.DurationValue("timeout")
	if err != nil {
		return nil, nil, err
	}

	info, _ := ts.StringSliceValue("info")
	vars, err := DumpStringPreserveCase(h)
	if err != nil {
		return ctx, nil, err
	}

	allKeys := []string{}
	for k, v := range vars {
		ctx = context.WithValue(ctx, ContextKey("var."+k), v)
		allKeys = append(allKeys, k)
	}
	ctx = context.WithValue(ctx, ContextKey("vars"), allKeys)

	if name == "" {
		return ctx, newExecutorRunner(nil, name, "builtin", retry, retryIf, delay, timeout, info), nil
	}

	if ex, ok := v.executorsBuiltin[name]; ok {
		return ctx, newExecutorRunner(ex, name, "builtin", retry, retryIf, delay, timeout, info), nil
	}

	if ex, ok := v.executorsUser[name]; ok {
		return ctx, newExecutorRunner(ex, name, "user", retry, retryIf, delay, timeout, info), nil
	}

	// testcases may run in parallel (--parallel): plugin loading and the plugin map are serialized
	pluginMu.Lock()
	if err := v.registerPlugin(ctx, name, vars); err != nil {
		Debug(ctx, "executor %q is not implemented as plugin - err:%v", name, err)
	}

	// then add the executor plugin to the map to not have to load it on each step
	ex, ok := v.executorsPlugin[name]
	pluginMu.Unlock()
	if ok {
		return ctx, newExecutorRunner(ex, name, "plugin", retry, retryIf, delay, timeout, info), nil
	}
	return ctx, nil, fmt.Errorf("user executor %q not found - loaded executors are: %v", name, reflect.ValueOf(v.executorsUser).MapKeys())
}

// userExecutorFile is a user executor file and the lib directory it was found in.
type userExecutorFile struct {
	path, lib string
}

// getUserExecutorFilesPath lists the user executor files, by lib directory in this order: every --lib-dir, then for
// each testsuite every "lib" directory from its own directory up to the repository root (a directory with .git) or the
// working directory, nearest first, then "lib" of the working directory. Scenarios keep executors in scenarios/lib while
// suites sit in scenarios/billing/…, so a suite finds them with no --lib-dir. A lib directory that is a symlink is
// resolved: filepath.Walk does not descend a symlinked root.
func (v *Venom) getUserExecutorFilesPath(ctx context.Context) []userExecutorFile {
	var libpaths []string
	seen := make(map[string]struct{})
	add := func(p string) {
		p = strings.TrimSpace(p)
		if p == "" {
			return
		}
		abs, err := filepath.Abs(p)
		if err != nil {
			return
		}
		if real, err := filepath.EvalSymlinks(abs); err == nil {
			abs = real
		}
		if _, ok := seen[abs]; ok {
			return
		}
		seen[abs] = struct{}{}
		libpaths = append(libpaths, abs)
	}

	if v.LibDir != "" {
		for _, lp := range strings.Split(v.LibDir, string(os.PathListSeparator)) {
			add(lp)
		}
	}
	cwd, _ := os.Getwd()
	for i := range v.Tests.TestSuites {
		for _, lib := range suiteLibDirs(v.Tests.TestSuites[i].WorkDir, cwd) {
			add(lib)
		}
	}
	add(filepath.Join(cwd, "lib"))

	var files []userExecutorFile
	for _, p := range libpaths {
		if _, err := os.Stat(p); err != nil {
			continue
		}
		var inLib []string
		err := filepath.Walk(p, func(fp string, f os.FileInfo, err error) error {
			switch filepath.Ext(fp) {
			case ".yml", ".yaml":
				inLib = append(inLib, fp)
			}
			return nil
		})
		if err != nil {
			Warn(ctx, "Unable to list files in lib directory %q: %v", p, err)
		}
		sort.Strings(inLib)
		for _, fp := range inLib {
			files = append(files, userExecutorFile{path: fp, lib: p})
		}
	}

	if len(files) == 0 {
		Warn(ctx, "no user executor yml file selected")
	}
	return files
}

// suiteLibDirs is every "lib" directory from the testsuite directory up, nearest first: up to and including the
// repository root (a directory holding .git) or the working directory, whichever comes first. A testsuite under
// neither gets only its own lib.
func suiteLibDirs(workdir, cwd string) []string {
	if workdir == "" {
		return nil
	}
	dir := filepath.Clean(workdir)
	var dirs []string
	for {
		dirs = append(dirs, dir)
		if dir == cwd {
			break
		}
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			dirs = dirs[:1]
			break
		}
		dir = parent
	}
	libs := make([]string, 0, len(dirs))
	for _, d := range dirs {
		lib := filepath.Join(d, "lib")
		if fi, err := os.Stat(lib); err == nil && fi.IsDir() {
			libs = append(libs, lib)
		}
	}
	return libs
}

func (v *Venom) registerUserExecutors(ctx context.Context) error {
	executorLibs := map[string]string{}
	for _, file := range v.getUserExecutorFilesPath(ctx) {
		f := file.path
		Debug(ctx, "Reading %v", f)
		content, err := os.ReadFile(f)
		if err != nil {
			return errors.Wrapf(err, "unable to read file %q", f)
		}

		ex := readPartialYML(content, "executor")
		if len(ex) == 0 {
			return errors.Errorf("missing key 'executor' in %q", f)
		}

		name := strings.Replace(ex, "executor:", "", 1)
		name = strings.TrimSpace(name)

		if err := lintUserExecutor(f, name, content); err != nil {
			return err
		}

		inputs := readPartialYML(content, "input")

		ux := UserExecutor{
			Filename:  f,
			Executor:  name,
			RawInputs: []byte(inputs),
			Raw:       content,
		}

		if lib, ok := executorLibs[ux.Executor]; ok && lib != file.lib {
			// the executor of a nearer lib (or of --lib-dir) shadows the one of the same name further up
			Debug(ctx, "User executor %q of %q shadowed by the one of %q", ux.Executor, f, lib)
			continue
		}
		executorLibs[ux.Executor] = file.lib
		err = v.RegisterExecutorUser(ux.Executor, ux)
		if err != nil {
			return errors.Wrapf(err, "unable to register user executor %q from file %q", ux.Executor, f)
		}
		Info(ctx, "User executor %q registered", ux.Executor)

		if strings.HasPrefix(ux.Executor, "Should") {
			err = v.registerUserAssertFunc(ctx, ux.Executor)
			if err != nil {
				return errors.Wrapf(err, "unable to register user executor %q from file %q as user assertion", ux.Executor, f)
			}
		}
	}

	return nil
}

func (v *Venom) registerUserAssertFunc(ctx context.Context, name string) error {
	_, e, err := v.GetExecutorRunner(ctx, TestStep{"type": name}, H{})
	if err != nil {
		return errors.Wrapf(err, "unable to load user executor %q", name)
	}

	err = assertions.RegisterUserAssertFunc(name, func(actual interface{}, expected ...interface{}) error {
		// Prepare a minimal context in which the user executor can be run
		// We only need to register the operands in the var set
		// as parent contexts will already have been templated
		tc := &TestCase{TestCaseInput: TestCaseInput{Name: name}}
		tc.TestSuiteVars.Add("a", actual)
		if len(expected) > 0 {
			tc.TestSuiteVars.Add("b", expected[0])
		}
		tc.TestSuiteVars.Add("argv", expected)
		ts := &TestStepResult{}

		_, err := v.RunUserExecutor(ctx, e, tc, ts, TestStep{})

		// We reformat any sub-assertions errors to avoid redundant texts
		if len(ts.Errors) > 0 {
			msg := make([]string, len(ts.Errors))
			for _, e := range ts.Errors {
				msg = append(msg, fmt.Sprintf(`  %d: Sub-assertion %q failed. %s`,
					e.StepNumber,
					RemoveNotPrintableChar(e.Assertion),
					RemoveNotPrintableChar(e.Error.Error()),
				))
			}
			return errors.New(strings.Join(msg, "\n"))
		} else if err != nil {
			return errors.Wrapf(err, "user assertion failed during execution")
		}

		return nil
	})
	if err != nil {
		return errors.Wrapf(err, "unable to register user assertion function %q", name)
	}

	return nil
}

func (v *Venom) registerPlugin(ctx context.Context, name string, vars map[string]string) error {
	workdir := vars["venom.testsuite.workdir"]
	// try to load from testsuite path
	p, err := plugin.Open(path.Join(workdir, "lib", name+".so"))
	if err != nil {
		// try to load from venom binary path
		p, err = plugin.Open(path.Join("lib", name+".so"))
		if err != nil {
			return fmt.Errorf("unable to load plugin %q.so", name)
		}
	}

	symbolExecutor, err := p.Lookup("Plugin")
	if err != nil {
		return err
	}

	executor := symbolExecutor.(Executor)
	v.RegisterExecutorPlugin(name, executor)

	return nil
}

func VarFromCtx(ctx context.Context, varname string) interface{} {
	i := ctx.Value(ContextKey("var." + varname))
	return i
}

func StringVarFromCtx(ctx context.Context, varname string) string {
	i := ctx.Value(ContextKey("var." + varname))
	return cast.ToString(i)
}

func StringSliceVarFromCtx(ctx context.Context, varname string) []string {
	i := ctx.Value(ContextKey("var." + varname))
	return cast.ToStringSlice(i)
}

func IntVarFromCtx(ctx context.Context, varname string) int {
	i := ctx.Value(ContextKey("var." + varname))
	return cast.ToInt(i)
}

func BoolVarFromCtx(ctx context.Context, varname string) bool {
	i := ctx.Value(ContextKey("var." + varname))
	return cast.ToBool(i)
}

func StringMapInterfaceVarFromCtx(ctx context.Context, varname string) map[string]interface{} {
	i := ctx.Value(ContextKey("var." + varname))
	return cast.ToStringMap(i)
}

func StringMapStringVarFromCtx(ctx context.Context, varname string) map[string]string {
	i := ctx.Value(ContextKey("var." + varname))
	return cast.ToStringMapString(i)
}

func AllVarsFromCtx(ctx context.Context) H {
	i := ctx.Value(ContextKey("vars"))
	allKeys := cast.ToStringSlice(i)
	res := H{}
	for _, k := range allKeys {
		res.Add(k, VarFromCtx(ctx, k))
	}
	return res
}

func JSONUnmarshal(btes []byte, i interface{}) error {
	d := json.NewDecoder(bytes.NewReader(btes))
	d.UseNumber()
	return d.Decode(i)
}
