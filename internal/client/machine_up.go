package client

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/markusylisiurunen/ship/internal/constant"
	"github.com/urfave/cli/v3"
	"golang.org/x/crypto/ssh"
)

type MachineUpAction struct {
	version string
	hetzner *hcloud.Client
	ssh     *ssh.Client
}

func NewMachineUpAction(version string) *MachineUpAction {
	return &MachineUpAction{version: version}
}

func (a *MachineUpAction) init(ctx context.Context, cmd *cli.Command) (cleanup func(), initErr error) {
	cleanup = func() {
		if a.ssh != nil {
			a.ssh.Close()
		}
	}

	token := cmd.String("token")
	if token == "" {
		initErr = fmt.Errorf("hetzner API token is required")
		return
	}
	a.hetzner = hcloud.NewClient(hcloud.WithToken(token))

	serverName := cmd.String("name")
	if serverName == "" {
		initErr = fmt.Errorf("server name is required")
		return
	}
	server, _, err := a.hetzner.Server.GetByName(ctx, serverName)
	if err != nil {
		initErr = fmt.Errorf("fetch server %q: %w", serverName, err)
		return
	}
	if server == nil {
		initErr = fmt.Errorf("server %q not found", serverName)
		return
	}

	sshPrivateKey := cmd.String("ssh-private-key")
	if sshPrivateKey == "" {
		initErr = fmt.Errorf("ssh private key is required")
		return
	}
	privateKey, err := os.ReadFile(sshPrivateKey)
	if err != nil {
		initErr = fmt.Errorf("read ssh private key %q: %w", sshPrivateKey, err)
		return
	}
	signer, err := ssh.ParsePrivateKey(privateKey)
	if err != nil {
		initErr = fmt.Errorf("parse ssh private key: %w", err)
		return
	}
	if client, err := ssh.Dial(
		"tcp",
		fmt.Sprintf("%s:%d", server.PublicNet.IPv4.IP.String(), constant.SSH.Port),
		&ssh.ClientConfig{
			Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			Timeout:         10 * time.Second,
			User:            "deploy",
		},
	); err != nil {
		initErr = fmt.Errorf("connect to server %q over ssh: %w", serverName, err)
		return
	} else {
		a.ssh = client
	}

	return
}

func (a *MachineUpAction) Action(ctx context.Context, cmd *cli.Command) error {
	// Initialize the Hetzner client and SSH connection
	cleanup, err := a.init(ctx, cmd)
	if err != nil {
		return err
	}
	defer cleanup()

	// Ensure the `agent` binary is on the machine
	var copyErr error
	if a.version == "dev" {
		copyErr = copyDevAgentBinaryToServer(ctx, a.ssh, true)
	} else {
		copyErr = copyVersionedAgentBinaryToServer(ctx, a.ssh, true, a.version)
	}
	if copyErr != nil {
		return fmt.Errorf("ensure agent binary on server: %w", copyErr)
	}

	// Execute the appropriate `agent` command on the machine
	sess, err := a.ssh.NewSession()
	if err != nil {
		return fmt.Errorf("create SSH session for up: %w", err)
	}
	defer sess.Close()
	sess.Stdout = os.Stdout
	sess.Stderr = os.Stderr
	upCmd := fmt.Sprintf("sudo /root/.ship/%s/agent up", a.version)
	if err := sess.Run(upCmd); err != nil {
		return fmt.Errorf("run agent up command %q: %w", upCmd, err)
	}

	return nil
}
