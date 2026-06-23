package cli

import (
	"flag"
	"fmt"

	"github.com/lawrips/tkt/internal/doctor"
)

func runDoctor(ctx context, args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(ctx.stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("usage: tkt doctor")
	}

	report := doctor.Run(doctor.Options{
		CWD:             currentWorkingDir(),
		ProjectOverride: ctx.projectOverride,
	})
	if ctx.json {
		return emitJSON(ctx, report)
	}
	_, _ = fmt.Fprint(ctx.stdout, doctor.FormatText(report))
	return nil
}
