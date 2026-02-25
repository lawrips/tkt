package cli

import "fmt"

func runVersion(ctx context, args []string) error {
	if ctx.json {
		return emitJSON(ctx, map[string]any{"version": versionString})
	}
	_, _ = fmt.Fprintf(ctx.stdout, "tkt %s\n", versionString)
	return nil
}
