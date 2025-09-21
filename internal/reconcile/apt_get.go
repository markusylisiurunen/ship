package reconcile

import (
	"context"
	"os"
	"os/exec"
)

var _ Reconciler = (*AptGet)(nil)

type AptGet struct {
	Upgrade  bool
	Packages []string
}

func (r *AptGet) Reconcile(ctx context.Context) error {
	if err := r.execAptGet(ctx, "update"); err != nil {
		return err
	}

	if r.Upgrade {
		if err := r.execAptGet(ctx, "upgrade", "-y"); err != nil {
			return err
		}
		if err := r.execAptGet(ctx, "dist-upgrade", "-y"); err != nil {
			return err
		}
	}

	args := append([]string{"install", "-y"}, r.Packages...)
	if err := r.execAptGet(ctx, args...); err != nil {
		return err
	}

	return nil
}

func (r *AptGet) execAptGet(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "apt-get", args...)
	cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
