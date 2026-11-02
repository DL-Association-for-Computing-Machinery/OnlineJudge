// Command localdev initializes and serves the isolated local development stack.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/DL-Association-for-Computing-Machinery/OnlineJudge/internal/localdev"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var err error
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: localdev init|serve")
		os.Exit(1)
	}
	switch os.Args[1] {
	case "init":
		startup, cancel := context.WithTimeout(ctx, time.Minute)
		defer cancel()
		err = localdev.Initialize(startup, localdev.StateDirectory())
	case "serve":
		err = localdev.Serve(ctx, localdev.StateDirectory(), "/site")
	default:
		err = fmt.Errorf("unknown local development command")
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "local development failed; private state and data preserved; inspect initialization and service availability")
		os.Exit(1)
	}
}
