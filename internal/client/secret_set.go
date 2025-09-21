package client

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/urfave/cli/v3"
	"golang.org/x/crypto/ssh"
)

type SecretSetAction struct {
	version string
	hetzner *hcloud.Client
	ssh     *ssh.Client
}

func NewSecretSetAction(version string) *SecretSetAction {
	return &SecretSetAction{version: version}
}

func (a *SecretSetAction) init(ctx context.Context, cmd *cli.Command) (cleanup func(), initErr error) {
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

	serverName := cmd.String("server-name")
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

func (a *SecretSetAction) Action(ctx context.Context, cmd *cli.Command) error {
	// Initialize the Hetzner client and SSH connection
	cleanup, err := a.init(ctx, cmd)
	if err != nil {
		return err
	}
	defer cleanup()

	// Write the secret to the appropriate file
	var (
		appName     = cmd.String("app-name")
		secretName  = cmd.String("secret-name")
		secretValue = cmd.String("secret-value")
	)
	if appName == "" || secretName == "" || secretValue == "" {
		return fmt.Errorf("App name, secret name and secret value are required")
	}
	cmds := []string{
		fmt.Sprintf(`mkdir -p /home/deploy/apps/%s/secrets`, appName),
		fmt.Sprintf(`echo -n %q > /home/deploy/apps/%s/secrets/%s`, secretValue, appName, secretName),
		fmt.Sprintf(`chmod 644 /home/deploy/apps/%s/secrets/%s`, appName, secretName),
	}
	sess, err := a.ssh.NewSession()
	if err != nil {
		return err
	}
	defer sess.Close()
	sess.Stdout = os.Stdout
	sess.Stderr = os.Stderr
	if err := sess.Run(strings.Join(cmds, " && ")); err != nil {
		return err
	}

	return nil
}
