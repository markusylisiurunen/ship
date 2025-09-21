package reconcile

import (
	"context"
	"os"
	"os/exec"
)

var _ Reconciler = (*RawScript)(nil)

type RawScript struct {
	Script string
}

func (r *RawScript) Reconcile(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "bash", "-euxo", "pipefail", "-c", r.Script)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
