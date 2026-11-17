package run

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/ovh/venom"
	"github.com/ovh/venom/runartifacts"
)

// signalExit is the exit code of a run ended by a signal: 128 and the signal number, as a shell gives it.
const signalExit = 128

// runFolder closes the run folder of --run-dir whatever way the run ends: a signal, an error of its setup, a crash of
// the runner. With no --run-dir it does nothing.
type runFolder struct {
	writer *runartifacts.Writer
}

// abort closes the run folder with the reason the run ended early.
func (f runFolder) abort(reason string) {
	if f.writer == nil {
		return
	}

	if err := f.writer.Abort(context.Background(), reason); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
	}
}

// fail prints err, closes the run folder with it and exits 2.
func (f runFolder) fail(stage string, err error) {
	fmt.Fprintf(os.Stderr, "%v\n", err)
	f.abort(stage + ": " + err.Error())
	venom.OSExit(2)
}

// onSignal closes the run folder when the run is interrupted (SIGINT) or terminated (SIGTERM), then exits 128 and
// the signal number. A SIGKILL cannot be caught: the folder then holds what was written as each testcase began and ended.
func (f runFolder) onSignal() {
	if f.writer == nil {
		return
	}

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)

	go func() {
		s := <-ch
		f.abort("interrupted by signal " + s.String())

		code := signalExit + int(syscall.SIGINT)

		if s == syscall.SIGTERM {
			code = signalExit + int(syscall.SIGTERM)
		}

		os.Exit(code)
	}()
}

// onCrash closes the run folder when the runner panics, then lets the panic go on.
func (f runFolder) onCrash() {
	if r := recover(); r != nil {
		f.abort(fmt.Sprintf("runner crashed: panic: %v", r))
		panic(r)
	}
}
