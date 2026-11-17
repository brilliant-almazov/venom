// Package schemes is the venom schemes command: the zoom set of a scenario tree drawn with no testcase run.
package schemes

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/ovh/venom"
	"github.com/ovh/venom/executors"
	"github.com/ovh/venom/runartifacts"
)

var (
	runsFlag   *string
	outFlag    *string
	libDirFlag *string
)

func init() {
	runsFlag = Cmd.Flags().String("runs", "", "Runs folder of the set, <runs>/<YYYY-MM>/<DD-n>/REPORT.md: the history of every block and the bar of every testcase; none gives «no run» bars")
	outFlag = Cmd.Flags().String("out", "", "Folder the zoom set is written to, as <out>/schemes/ of a run folder (required)")
	libDirFlag = Cmd.Flags().String("lib-dir", "", "Lib Directory: user executors besides the lib folders of the trees")
	_ = Cmd.MarkFlagRequired("out")
}

// Cmd schemes writes the zoom set of the testsuites of the trees with no testcase run.
var Cmd = &cobra.Command{
	Use:   "schemes <scenario tree>...",
	Short: "Write the zoom schemes of a scenario tree with no testcase run",
	Long: `Read every testsuite of the trees as written — no var filled, no step run, no network — and write the zoom set a
run folder carries: schemes/00-run.md, 01-system.md, 02-paths/<path>.md, 03-suites/<testsuite>.md, a testsuite of 8
testcases and more split into 03-suites/<testsuite>/<NN>-<testcase>.md, and an INDEX.md on every level. Each step is
drawn down to its executor, method and url, SQL text, queues and the steps of the user executors it calls. The bars and
the history come from the runs of the set under --runs; a testcase no run has gets «no run».`,
	Example: `  venom schemes scenarios/supabase --runs cases/supabase/runs --out cases/supabase/plan
  venom schemes scenarios/billing --runs cases/billing/runs --out cases/billing/plan
  venom schemes scenarios/crawler --runs cases/crawler/runs --out cases/crawler/plan`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		v := venom.New()

		for name, executorFunc := range executors.Registry {
			v.RegisterExecutorBuiltin(name, executorFunc())
		}

		v.LibDir = *libDirFlag
		ctx := context.Background()

		if err := v.ReadPlan(ctx, args); err != nil {
			return err
		}

		if err := runartifacts.WritePlan(ctx, v, *outFlag, *runsFlag); err != nil {
			return err
		}

		fmt.Fprintf(cmd.OutOrStdout(), "schemes: %d testsuites written to %s/%s\n", len(v.Tests.TestSuites), *outFlag,
			runartifacts.SchemesDir)

		return nil
	},
}
