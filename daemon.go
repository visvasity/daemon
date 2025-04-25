// Copyright (c) 2023 BVK Chaitanya

// Package daemon implements a mechanism to run a long running process in
// background.
package daemon

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"

	"golang.org/x/sys/unix"
)

// ReadyFunc is a callback function, run in the foreground process that waits
// for the successful or unsuccessful initialization of the background
// process. Input context is canceled (with a cause) if the background process
// dies for any reason.
type ReadyFunc = func(ctx context.Context) error

// Daemonize starts another instance of the current program (aka foreground
// process) in the background as a daemon. It must be called in both foreground
// and background processes. It should be invoked early during the program
// startup, before performing any other significant work, like opening
// databases, opening network connections, etc.
//
// Background process is started with the same command-line arguments as the
// foreground process, but with an additional `envKey` environment variable --
// which *must* be empty in the foreground processes' environment. Note that
// background process is started with a limited environment: PATH, USER, HOME
// and the envKey.
//
// The input `envKey` name must be application-specific, unique and
// non-empty. Users are free to choose any non-empty value for the environment
// variable as they see fit (eg: an URL to report ready status).
//
// In addition to the limited environment, standard input and outputs of the
// background process are replaced with `/dev/null`. Standard library's log
// output is redirected to the [io.Discard] backend. Current working directory
// of the background process is changed to the root directory.
//
// If check parameter is non-nil, parent process will use it to wait for the
// background process to initialize or die.
//
// When successful, Daemonize returns nil to both foreground and background
// processes. When unsuccessful or if the input context expires, Daemonize
// returns a non-nil error to the foreground process and may kill the child
// process if it has started.
func Daemonize(ctx context.Context, envKey, envValue string, check ReadyFunc) (foreground bool, err error) {
	if len(envKey) == 0 || len(envValue) == 0 {
		return true, os.ErrInvalid
	}

	if v := os.Getenv(envKey); len(v) == 0 {
		if err := daemonizeParent(ctx, envKey, envValue, check); err != nil {
			return true, err
		}
		return true, nil
	}

	if err := daemonizeChild(envKey); err != nil {
		return false, err
	}
	return false, nil
}

func daemonizeParent(ctx context.Context, envKey, envValue string, check ReadyFunc) (status error) {
	binaryPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to lookup binary: %w", err)
	}

	file, err := os.OpenFile("/dev/null", os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("failed to open /dev/null: %w", err)
	}
	defer file.Close()

	attr := &os.ProcAttr{
		Dir: "/",
		Env: []string{
			fmt.Sprintf("PATH=%s", os.Getenv("PATH")),
			fmt.Sprintf("USER=%s", os.Getenv("USER")),
			fmt.Sprintf("HOME=%s", os.Getenv("HOME")),
			fmt.Sprintf("%s=%s", envKey, envValue),
		},
		Files: []*os.File{file, file, file},
		//Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
	}
	proc, err := os.StartProcess(binaryPath, os.Args, attr)
	if err != nil {
		return fmt.Errorf("failed to start process: %w", err)
	}

	// Receive signal when child-process dies.
	ctx, cancelCause := context.WithCancelCause(ctx)
	defer cancelCause(nil)

	go func() {
		state, err := proc.Wait()
		if err != nil {
			cancelCause(fmt.Errorf("could not wait on child process: %w", err))
			return
		}
		if state.Success() {
			cancelCause(fmt.Errorf("child exited successfully with exit code 0"))
			return
		}
		code := state.ExitCode()
		if code == -1 {
			cancelCause(fmt.Errorf("child is terminated by a signal"))
			return
		}
		cancelCause(fmt.Errorf("child exited with exit code %d", code))
	}()

	if check != nil {
		if err := check(ctx); err != nil {
			log.Printf("could not initialize the background process: %v", err)
			proc.Kill()
			return err
		}
		log.Printf("background process is initialized successfully")
	}
	return nil
}

func daemonizeChild(envKey string) error {
	if _, err := unix.Setsid(); err != nil {
		return fmt.Errorf("could not set session id: %w", err)
	}

	log.SetOutput(io.Discard)
	return nil
}
