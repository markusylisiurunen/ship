package client

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
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
		initErr = fmt.Errorf("Hetzner API token is required")
		return
	}
	a.hetzner = hcloud.NewClient(hcloud.WithToken(token))

	serverName := cmd.String("name")
	if serverName == "" {
		initErr = fmt.Errorf("Server name is required")
		return
	}
	server, _, err := a.hetzner.Server.GetByName(ctx, serverName)
	if err != nil {
		initErr = err
		return
	}
	if server == nil {
		initErr = fmt.Errorf("Server %q not found", serverName)
		return
	}

	sshPrivateKey := cmd.String("ssh-private-key")
	if sshPrivateKey == "" {
		initErr = fmt.Errorf("SSH private key is required")
		return
	}
	privateKey, err := os.ReadFile(sshPrivateKey)
	if err != nil {
		initErr = err
		return
	}
	signer, err := ssh.ParsePrivateKey(privateKey)
	if err != nil {
		initErr = err
		return
	}
	if client, err := ssh.Dial(
		"tcp",
		fmt.Sprintf("%s:67", server.PublicNet.IPv4.IP.String()),
		&ssh.ClientConfig{
			Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			Timeout:         10 * time.Second,
			User:            "deploy",
		},
	); err != nil {
		initErr = err
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
		return copyErr
	}

	// Execute the appropriate `agent` command on the machine
	sess, err := a.ssh.NewSession()
	if err != nil {
		return err
	}
	defer sess.Close()
	sess.Stdout = os.Stdout
	sess.Stderr = os.Stderr
	if err := sess.Run(
		fmt.Sprintf("sudo /root/.ship/%s/agent up", a.version),
	); err != nil {
		return err
	}

	return nil
}
