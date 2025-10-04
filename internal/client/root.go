package client

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/bramvdbogaerde/go-scp"
	"github.com/urfave/cli/v3"
	"golang.org/x/crypto/ssh"
)

func Execute(ctx context.Context, version string) {
	cmd := &cli.Command{
		Name:    "ship",
		Usage:   "deploy apps to a VPS on Hetzner",
		Version: version,
		Commands: []*cli.Command{
			{
				Name:  "machine",
				Usage: "manage machines on Hetzner",
				Commands: []*cli.Command{
					{
						Name:  "create",
						Usage: "create a new machine on Hetzner",
						Flags: []cli.Flag{
							&cli.StringFlag{Name: "token", Usage: "Hetzner API token", Required: true},
							&cli.StringFlag{Name: "ssh-key-name", Usage: "Hetzner SSH key name", Required: true},
							&cli.StringFlag{Name: "name", Usage: "Hetzner server name"},
							&cli.StringFlag{Name: "size", Usage: "Hetzner server size", Value: "cx22"},
							&cli.StringFlag{Name: "location", Usage: "Hetzner location", Value: "hel1"},
						},
						Action: NewMachineCreateAction(version).Action,
					},
					{
						Name:  "up",
						Usage: "reconcile a machine on Hetzner to an up-to-date state",
						Flags: []cli.Flag{
							&cli.StringFlag{Name: "token", Usage: "Hetzner API token", Required: true},
							&cli.StringFlag{Name: "ssh-private-key", Usage: "SSH private key file path", Required: true},
							&cli.StringFlag{Name: "name", Usage: "Hetzner server name", Required: true},
						},
						Action: NewMachineUpAction(version).Action,
					},
					{
						Name:  "maintain",
						Usage: "run maintenance tasks on a machine on Hetzner",
						Flags: []cli.Flag{
							&cli.StringFlag{Name: "token", Usage: "Hetzner API token", Required: true},
							&cli.StringFlag{Name: "ssh-private-key", Usage: "SSH private key file path", Required: true},
							&cli.StringFlag{Name: "name", Usage: "Hetzner server name", Required: true},
							&cli.BoolFlag{Name: "allow-reboot", Usage: "reboot the machine if necessary", Value: false},
						},
						Action: NewMachineMaintainAction(version).Action,
					},
				},
			},
			{
				Name:  "secret",
				Usage: "manage secrets on a machine on Hetzner",
				Commands: []*cli.Command{
					{
						Name:  "set",
						Usage: "set a secret on a machine on Hetzner",
						Flags: []cli.Flag{
							&cli.StringFlag{Name: "token", Usage: "Hetzner API token", Required: true},
							&cli.StringFlag{Name: "ssh-private-key", Usage: "SSH private key file path", Required: true},
							&cli.StringFlag{Name: "server-name", Usage: "Hetzner server name", Required: true},
							&cli.StringFlag{Name: "app-name", Usage: "application name", Required: true},
							&cli.StringFlag{Name: "secret-name", Usage: "secret name", Required: true},
							&cli.StringFlag{Name: "secret-value", Usage: "secret value", Required: true},
						},
						Action: NewSecretSetAction(version).Action,
					},
				},
			},
			{
				Name:  "deploy",
				Usage: "deploy an app to a machine on Hetzner",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "token", Usage: "Hetzner API token", Required: true},
					&cli.StringFlag{Name: "ssh-private-key", Usage: "SSH private key file path", Required: true},
					&cli.StringFlag{Name: "server-name", Usage: "Hetzner server name", Required: true},
					&cli.StringFlag{Name: "app-name", Usage: "application name", Required: true},
					&cli.StringFlag{Name: "app-version", Usage: "application version", Required: true},
					&cli.StringSliceFlag{Name: "volume-name", Usage: "volume name (can be specified multiple times)"},
				},
				Action: NewDeployAction(version).Action,
			},
		},
	}
	if err := cmd.Run(ctx, os.Args); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}

// copyDevAgentBinaryToServer builds and copies the `agent` binary to the server.
func copyDevAgentBinaryToServer(
	ctx context.Context,
	ssh *ssh.Client,
	root bool,
) error {
	const version = "dev"

	// Create a temporary directory to build the binary in
	tempDir, err := os.MkdirTemp("", "ship")
	if err != nil {
		return fmt.Errorf("create temp dir for agent build: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Build the `agent` binary
	fmt.Printf("Building agent binary...\n")
	cmd := exec.CommandContext(ctx, "go", "build",
		"-ldflags=-s -w",
		"-trimpath",
		"-o", fmt.Sprintf("%s/agent", tempDir),
		"./cmd/agent",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(),
		"CGO_ENABLED=0",
		"GOARCH=amd64",
		"GOOS=linux",
	)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("build agent binary: %w", err)
	}

	// Prepare the server
	sess, err := ssh.NewSession()
	if err != nil {
		return fmt.Errorf("open SSH session: %w", err)
	}
	defer sess.Close()
	sess.Stdout = os.Stdout
	sess.Stderr = os.Stderr
	if root {
		cmds := []string{
			fmt.Sprintf("mkdir -p /home/deploy/.ship/%s", version),
			fmt.Sprintf("sudo mkdir -p /root/.ship/%s", version),
		}
		if err := sess.Run(strings.Join(cmds, " && ")); err != nil {
			return fmt.Errorf("prepare remote directories for root install: %w", err)
		}
	} else {
		cmds := []string{
			fmt.Sprintf("mkdir -p /home/deploy/.ship/%s", version),
		}
		if err := sess.Run(strings.Join(cmds, " && ")); err != nil {
			return fmt.Errorf("prepare remote directories for deploy install: %w", err)
		}
	}

	// Copy the binary to the server using SCP
	binaryPath := fmt.Sprintf("%s/agent", tempDir)
	bin, err := os.Open(binaryPath)
	if err != nil {
		return fmt.Errorf("open built agent binary %q: %w", binaryPath, err)
	}
	defer bin.Close()
	client, err := scp.NewClientBySSH(ssh)
	if err != nil {
		return fmt.Errorf("create SCP client: %w", err)
	}
	defer client.Close()
	fmt.Printf("Copying agent binary to the server...\n")
	remotePath := fmt.Sprintf("/home/deploy/.ship/%s/agent", version)
	if err := client.CopyFromFile(ctx, *bin, remotePath, "0744"); err != nil {
		return fmt.Errorf("copy agent binary to %q: %w", remotePath, err)
	}

	// Move the binary to root if needed
	if root {
		sess, err := ssh.NewSession()
		if err != nil {
			return fmt.Errorf("open SSH session for root promotion: %w", err)
		}
		defer sess.Close()
		sess.Stdout = os.Stdout
		sess.Stderr = os.Stderr
		cmds := []string{
			fmt.Sprintf("sudo mv /home/deploy/.ship/%s/agent /root/.ship/%s/agent", version, version),
			fmt.Sprintf("sudo rm -rf /home/deploy/.ship/%s", version),
			fmt.Sprintf("sudo chown root:root /root/.ship/%s/agent", version),
		}
		if err := sess.Run(strings.Join(cmds, " && ")); err != nil {
			return fmt.Errorf("promote agent binary to root: %w", err)
		}
	}

	fmt.Printf("Agent binary built and copied successfully\n")
	return nil
}

// copyVersionedAgentBinaryToServer copies a pre-built `agent` binary to the server.
func copyVersionedAgentBinaryToServer(
	_ context.Context,
	ssh *ssh.Client,
	root bool,
	version string,
) error {
	var agentBinaryDownloadURL = fmt.Sprintf(
		"https://github.com/markusylisiurunen/ship/releases/download/v%s/ship_agent_linux_amd64.tar.gz",
		version,
	)

	// Prepare the server and download the `agent` binary with locking
	sess, err := ssh.NewSession()
	if err != nil {
		return fmt.Errorf("open SSH session: %w", err)
	}
	defer sess.Close()
	sess.Stdout = os.Stdout
	sess.Stderr = os.Stderr

	// Use a lock file to prevent concurrent installations
	var lockFile string
	if root {
		lockFile = "/tmp/ship-agent-install-root.lock"
	} else {
		lockFile = "/tmp/ship-agent-install-deploy.lock"
	}

	if root {
		shellOneLiner := "if [ ! -f /root/.ship/%s/agent ]; then"
		shellOneLiner += fmt.Sprintf(" mkdir -p /root/.ship/%s;", version)
		shellOneLiner += fmt.Sprintf(" curl -fsSL -o /root/.ship/%s/ship.tar.gz %s;", version, agentBinaryDownloadURL)
		shellOneLiner += fmt.Sprintf(" tar -xzf /root/.ship/%s/ship.tar.gz -C /root/.ship/%s;", version, version)
		shellOneLiner += fmt.Sprintf(" rm -f /root/.ship/%s/ship.tar.gz;", version)
		shellOneLiner += fmt.Sprintf(" mv /root/.ship/%s/ship_agent_linux_amd64 /root/.ship/%s/agent;", version, version)
		shellOneLiner += fmt.Sprintf(" chmod +x /root/.ship/%s/agent;", version)
		shellOneLiner += fmt.Sprintf(" chown root:root /root/.ship/%s/agent;", version)
		shellOneLiner += " fi"
		if err := sess.Run(fmt.Sprintf("sudo timeout 5 flock %s sh -c '%s'", lockFile, shellOneLiner)); err != nil {
			return fmt.Errorf("install agent binary from %s: %w", agentBinaryDownloadURL, err)
		}
	} else {
		shellOneLiner := "if [ ! -f /home/deploy/.ship/%s/agent ]; then"
		shellOneLiner += fmt.Sprintf(" mkdir -p /home/deploy/.ship/%s;", version)
		shellOneLiner += fmt.Sprintf(" curl -fsSL -o /home/deploy/.ship/%s/ship.tar.gz %s;", version, agentBinaryDownloadURL)
		shellOneLiner += fmt.Sprintf(" tar -xzf /home/deploy/.ship/%s/ship.tar.gz -C /home/deploy/.ship/%s;", version, version)
		shellOneLiner += fmt.Sprintf(" rm -f /home/deploy/.ship/%s/ship.tar.gz;", version)
		shellOneLiner += fmt.Sprintf(" mv /home/deploy/.ship/%s/ship_agent_linux_amd64 /home/deploy/.ship/%s/agent;", version, version)
		shellOneLiner += fmt.Sprintf(" chmod +x /home/deploy/.ship/%s/agent;", version)
		shellOneLiner += fmt.Sprintf(" chown deploy:deploy /home/deploy/.ship/%s/agent;", version)
		shellOneLiner += " fi"
		if err := sess.Run(fmt.Sprintf("timeout 5 flock %s sh -c '%s'", lockFile, shellOneLiner)); err != nil {
			return fmt.Errorf("install agent binary from %s: %w", agentBinaryDownloadURL, err)
		}
	}

	fmt.Printf("Agent binary downloaded and installed successfully\n")
	return nil
}
