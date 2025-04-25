// Copyright (c) 2025 Visvasity LLC

// Package monitor implements a self-monitoring auto-restart mechanism.
package monitor

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"slices"
	"sync"
	"time"

	"github.com/visvasity/daemon/initstatus"
)

// Options defines the user configurable values for the Monitor.
type Options struct {
	ShutdownSignal    os.Signal     // Defaults to os.Interrupt
	ShutdownTimeout   time.Duration // Defaults to 10 seconds.
	MinBackoffTimeout time.Duration // Defaults to one second.
	MaxBackoffTimeout time.Duration // Defaults to one minute.
}

func (v *Options) setDefaults() {
	if v.ShutdownSignal == nil {
		v.ShutdownSignal = os.Interrupt
	}
	if v.ShutdownTimeout == 0 {
		v.ShutdownTimeout = 10 * time.Second
	}
	if v.MinBackoffTimeout == 0 {
		v.MinBackoffTimeout = time.Second
	}
	if v.MaxBackoffTimeout == 0 {
		v.MaxBackoffTimeout = time.Minute
	}
}

func (v *Options) check() error {
	if v.MaxBackoffTimeout < v.MinBackoffTimeout {
		return fmt.Errorf("max backoff timeout is smaller than min backoff timeout")
	}
	return nil
}

// SelfMonitor creates another instance of the current program and watches it
// to auto-restart on failures till the input context is expired. When the
// input context is expired, existing child process will be signaled to
// shutdown and the function returns a non-nil error.
//
// The input `envKey` must be an application-specific, unique non-empty
// environment variable name, which is used internally to distinguish between
// the monitor instance and the monitored instance. A temporary http server URL
// is stored in the envKey value, which can optionally receive child processes'
// initialization status.
func SelfMonitor(ctx context.Context, envKey string, opts *Options) error {
	if opts == nil {
		opts = new(Options)
	}
	opts.setDefaults()
	if err := opts.check(); err != nil {
		return err
	}

	if len(envKey) == 0 {
		return os.ErrInvalid
	}

	if v := os.Getenv(envKey); len(v) != 0 {
		return nil // Child process.
	}

	var wg sync.WaitGroup
	defer wg.Wait()

	for i := 0; ctx.Err() == nil; i++ {
		binPath, err := os.Executable()
		if err != nil {
			return fmt.Errorf("failed to lookup current executable: %w", err)
		}

		childArgs := slices.Clone(os.Args)
		childArgs[0] = binPath

		func() {
			childCtx, childCancel := context.WithCancelCause(ctx)
			defer childCancel(nil)

			addrURL, receiver, closer := initstatus.Receiver(ctx)
			defer closer()

			childEnvItem := fmt.Sprintf("%s=%s", envKey, addrURL)

			cmd := exec.CommandContext(childCtx, childArgs[0], childArgs[1:]...)
			cmd.Env = append(slices.Clone(os.Environ()), childEnvItem)
			cmd.WaitDelay = opts.ShutdownTimeout
			cmd.Cancel = func() error { return cmd.Process.Signal(opts.ShutdownSignal) }
			cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr

			wg.Add(1)
			go func() {
				defer wg.Done()

				if err := cmd.Run(); err != nil {
					log.Printf("child has died with status: %v", err)
					childCancel(err)
				}
				childCancel(nil)
			}()

			if err := receiver(childCtx); err != nil {
				childCancel(err)

				timeout := min(opts.MinBackoffTimeout<<time.Duration(i), opts.MaxBackoffTimeout)
				log.Printf("waiting for %v before attempting to restart the child: %v", timeout, childArgs)

				select {
				case <-ctx.Done():
				case <-time.After(timeout):
				}
				return
			}

			// Reset the backoff counter.
			i = 0
			log.Printf("child is initialized successfully")
			select {
			case <-ctx.Done():
			case <-childCtx.Done():
			}
		}()
	}
	return context.Cause(ctx)
}
