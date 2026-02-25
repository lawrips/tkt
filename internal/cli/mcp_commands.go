package cli

import (
	"fmt"

	"github.com/lawrips/tkt/internal/mcp"
)

func runMCP(ctx context, args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("usage: tkt mcp")
	}

	srv, err := mcp.NewServerFromCwd()
	if err != nil {
		return fmt.Errorf("mcp server init: %w", err)
	}

	return srv.ServeStdio()
}
