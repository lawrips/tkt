package main

import (
	"fmt"
	"os"

	"github.com/lawrips/tkt/internal/cli"
)

// Set via -ldflags at build time: go build -ldflags "-X main.version=v0.1.0"
var version = "dev"

func main() {
	cli.SetVersion(version)
	if err := cli.Run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
