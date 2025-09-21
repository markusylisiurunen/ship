package reconcile

import (
	"context"
	"os"
	"os/exec"
	"strings"
)

var _ Reconciler = (*Node)(nil)

type Node struct {
	GlobalPackages []string
}

func (r *Node) Reconcile(ctx context.Context) error {
	// Based on the official instructions at: https://nodejs.org/en/download
	cmds := []string{
		`curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.40.3/install.sh | bash`,
		`export NVM_DIR="$HOME/.nvm"; source "$NVM_DIR/nvm.sh"; nvm install --lts`,
		`export NVM_DIR="$HOME/.nvm"; source "$NVM_DIR/nvm.sh"; node -v && npm -v`,
	}
	for _, cmd := range cmds {
		if err := r.execRun(ctx, "bash", "-lc", cmd); err != nil {
			return err
		}
		if err := r.execRun(ctx, "sudo", "-u", "deploy", "bash", "-lc", cmd); err != nil {
			return err
		}
	}

	// Install global npm packages for both root and deploy user
	npmInstallCmd := `export NVM_DIR="$HOME/.nvm"; source "$NVM_DIR/nvm.sh"; npm install -g ` +
		strings.Join(r.GlobalPackages, " ")
	if err := r.execRun(ctx, "bash", "-lc", npmInstallCmd); err != nil {
		return err
	}
	if err := r.execRun(ctx, "sudo", "-u", "deploy", "bash", "-lc", npmInstallCmd); err != nil {
		return err
	}

	return nil
}

func (r *Node) execRun(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
