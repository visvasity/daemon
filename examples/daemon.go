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
)

var background = flag.Bool("background", false, "When true, runs in background")

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

	// ...initialize the service...

	// Report successful initialization.
	if err := initstatus.Report(ctx, os.Getenv(daemonizeEnvKey), nil /* status */); err != nil {
		log.Fatal(err)
	}

	<-ctx.Done()
}
