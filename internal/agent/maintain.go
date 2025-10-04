package agent

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/exec"

	"github.com/urfave/cli/v3"
)

type MaintainAction struct {
	version string
}

func NewMaintainAction(version string) *MaintainAction {
	return &MaintainAction{version: version}
}

func (a *MaintainAction) Action(ctx context.Context, cmd *cli.Command) error {
	// Update and upgrade the system
	for _, c := range [][]string{
		{"apt-get", "update"},
		{"apt-get", "-y", "upgrade"},
		{"apt-get", "-y", "autoremove"},
		{"apt-get", "-y", "clean"},
	} {
		if err := a.execRun(ctx, c[0], c[1:]...); err != nil {
			return err
		}
	}

	// Prune unused Docker resources
	if err := a.execRun(ctx, "docker", "system", "prune", "-f", "--filter", "until=168h"); err != nil {
		return err
	}

	// Check if a reboot is required, and if so, schedule a reboot in 1 minute
	if cmd.Bool("allow-reboot") {
		if _, err := os.Stat("/var/run/reboot-required"); err == nil {
			fmt.Printf("Reboot required, scheduling reboot in 1 minute...\n")
			if err := a.execRun(ctx, "shutdown", "--reboot", "+1"); err != nil {
				return err
			}
		} else if !os.IsNotExist(err) {
			return err
		} else {
			fmt.Printf("No reboot required.\n")
		}
	} else {
		fmt.Printf("Reboot not allowed, skipping reboot check.\n")
	}

	return nil
}

func (a *MaintainAction) execRun(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
