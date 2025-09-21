package agent

import (
	"context"
	_ "embed"

	"github.com/markusylisiurunen/ship/internal/reconcile"
	"github.com/urfave/cli/v3"
)

//go:embed script/install_docker.sh
var installDockerSh string

//go:embed script/setup_fail2ban.sh
var setupFail2banSh string

//go:embed script/setup_fzf.sh
var setupFzfSh string

//go:embed script/setup_sshd_config.sh
var setupSshdConfigSh string

type UpAction struct {
	version string
}

func NewUpAction(version string) *UpAction {
	return &UpAction{version: version}
}

func (a *UpAction) Action(ctx context.Context, cmd *cli.Command) error {
	steps := []reconcile.Reconciler{}
	// Install a set of packages on the system
	steps = append(steps, &reconcile.AptGet{
		Upgrade: true,
		Packages: []string{
			"ca-certificates",
			"curl",
			"fail2ban",
			"jq",
			"tree",
			"ufw",
			"unzip",
		},
	})
	// Setup `ufw` firewall with some basic rules (allowing only SSH, HTTP, HTTPS)
	steps = append(steps, &reconcile.Ufw{
		AllowedTcpPorts: []int{67, 80, 443},
	})
	// Setup the SSH daemon configuration for better security
	steps = append(steps, &reconcile.RawScript{
		Script: setupSshdConfigSh,
	})
	// Setup `fail2ban` to protect against brute-force attacks
	steps = append(steps, &reconcile.RawScript{
		Script: setupFail2banSh,
	})
	// Install and setup `fzf` command-line fuzzy finder
	steps = append(steps, &reconcile.RawScript{
		Script: setupFzfSh,
	})
	// Install Docker and add the `deploy` user to the `docker` group
	steps = append(steps, &reconcile.RawScript{
		Script: installDockerSh,
	})
	// Make sure Caddy is installed and running
	steps = append(steps, &reconcile.Caddy{})
	// Install Node.js and some global npm packages
	steps = append(steps, &reconcile.Node{
		GlobalPackages: []string{
			"npm@latest",
			"@openai/codex@latest",
			"@anthropic-ai/claude-code@latest",
		},
	})

	// Execute all the steps in order
	for _, step := range steps {
		if err := step.Reconcile(ctx); err != nil {
			return err
		}
	}

	return nil
}
