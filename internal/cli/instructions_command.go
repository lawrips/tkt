package cli

import (
	_ "embed"
	"fmt"
)

//go:embed agent-instructions.txt
var agentInstructions string

//go:embed setup.md
var agentSetup string

func runInstructions(ctx context, args []string) error {
	fmt.Fprint(ctx.stdout, agentInstructions)
	return nil
}

func runSetup(ctx context, args []string) error {
	fmt.Fprint(ctx.stdout, agentSetup)
	return nil
}
