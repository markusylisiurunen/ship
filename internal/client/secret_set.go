package client

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/markusylisiurunen/ship/internal/constant"
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
		initErr = fmt.Errorf("hetzner API token is required")
		return
	}
	a.hetzner = hcloud.NewClient(hcloud.WithToken(token))

	serverName := cmd.String("server-name")
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
		return fmt.Errorf("app name, secret name and secret value are required")
	}
	cmds := []string{
		fmt.Sprintf(`mkdir -p /home/deploy/apps/%s/secrets`, appName),
		fmt.Sprintf(`echo -n %q > /home/deploy/apps/%s/secrets/%s`, secretValue, appName, secretName),
		fmt.Sprintf(`chmod 640 /home/deploy/apps/%s/secrets/%s`, appName, secretName),
	}
	sess, err := a.ssh.NewSession()
	if err != nil {
		return fmt.Errorf("create SSH session for secret set: %w", err)
	}
	defer sess.Close()
	sess.Stdout = os.Stdout
	sess.Stderr = os.Stderr
	command := strings.Join(cmds, " && ")
	if err := sess.Run(command); err != nil {
		return fmt.Errorf("run remote command %q: %w", command, err)
	}

	return nil
}
