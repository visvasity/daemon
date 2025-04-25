// Copyright (c) 2025 Visvasity LLC

//go:build ignore

package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"

	"github.com/visvasity/daemon"
	"github.com/visvasity/daemon/initstatus"
	"github.com/visvasity/daemon/monitor"
)

var background = flag.Bool("background", false, "When true, runs in background")
var selfMonitor = flag.Bool("self-monitor", false, "When true, restarts automatically")

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	flag.Parse()

	daemonizeEnvKey := "DAEMONIZE_ENVKEY"
	if *background {
		addrURL, receiver, closer := initstatus.Receiver(ctx)
		defer closer()

		foreground, err := daemon.Daemonize(ctx, daemonizeEnvKey, addrURL, receiver)
		if err != nil {
			log.Fatal(err)
		}
		if foreground {
			return
		}
		// Background process continues forward.
	}

	monitorEnvKey := "SELFMONITOR_ENVKEY"
	if *selfMonitor {
		if err := monitor.SelfMonitor(ctx, monitorEnvKey, nil /* Options */); err != nil {
			log.Fatal(err)
		}
	}

	// ...acquire a lock on the data directory and perform initializations...

	// Report successful initialization to the foreground process.
	if *background {
		if err := initstatus.Report(ctx, os.Getenv(daemonizeEnvKey), nil /* status */); err != nil {
			log.Fatal(err)
		}
	}

	// Report successful initialization to the monitoring process.
	if *selfMonitor {
		if err := initstatus.Report(ctx, os.Getenv(monitorEnvKey), nil /* status */); err != nil {
			log.Fatal(err)
		}
	}

	<-ctx.Done()
}
