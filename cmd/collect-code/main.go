package main

import (
	"context"
	"os"

	"github.com/dxinli/collect-code/internal/app"
	"github.com/dxinli/collect-code/internal/clock/realclock"
	"github.com/dxinli/collect-code/internal/sim"
)

func main() {
	os.Exit(app.Run(context.Background(),
		app.Deps{Clock: realclock.New(), Simulate: sim.Run},
		os.Args[1:], os.Stdout, os.Stderr, os.LookupEnv))
}
