package client

import (
	"context"
	"fmt"
	"time"

	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/urfave/cli/v3"
)

type MachineCreateAction struct {
	version string
	hetzner *hcloud.Client
}

func NewMachineCreateAction(version string) *MachineCreateAction {
	return &MachineCreateAction{version: version}
}

func (a *MachineCreateAction) init(_ context.Context, cmd *cli.Command) (cleanup func(), initErr error) {
	cleanup = func() {}

	token := cmd.String("token")
	if token == "" {
		initErr = fmt.Errorf("Hetzner API token is required")
		return
	}
	a.hetzner = hcloud.NewClient(hcloud.WithToken(token))

	return
}

func (a *MachineCreateAction) Action(ctx context.Context, cmd *cli.Command) error {
	// Initialize the Hetzner client
	cleanup, err := a.init(ctx, cmd)
	if err != nil {
		return err
	}
	defer cleanup()

	// Create the server on Hetzner
	var (
		sshKeyName = cmd.String("ssh-key-name")
		serverName = cmd.String("name")
		serverSize = cmd.String("size")
		location   = cmd.String("location")
	)
	if serverName == "" {
		serverName = fmt.Sprintf("ship-%s", time.Now().Format("2006-01-02-15-04"))
	}
	if sshKeyName == "" || serverName == "" || serverSize == "" || location == "" {
		return fmt.Errorf("SSH key name, server name, server size, and location are required")
	}
	if err := a.createServer(ctx, sshKeyName, serverName, serverSize, location); err != nil {
		return err
	}

	// Wait for the server to be running on Hetzner
	if err := a.waitForServer(ctx, serverName); err != nil {
		return err
	}

	return nil
}

func (a *MachineCreateAction) createServer(
	ctx context.Context, sshKeyName, serverName, serverSize, location string,
) error {
	// Find the SSH key ID from Hetzner
	sshKeys, err := a.hetzner.SSHKey.All(ctx)
	if err != nil {
		return err
	}
	var sshKeyID int64 = -1
	for _, k := range sshKeys {
		if k.Name == sshKeyName {
			sshKeyID = k.ID
			break
		}
	}
	if sshKeyID == -1 {
		return fmt.Errorf("SSH key %q not found on Hetzner", sshKeyName)
	}

	// Create the server on Hetzner
	fmt.Printf("Creating server %q on Hetzner...\n", serverName)
	server, _, err := a.hetzner.Server.Create(ctx, hcloud.ServerCreateOpts{
		Image:      &hcloud.Image{Name: "ubuntu-24.04"},
		Location:   &hcloud.Location{Name: location},
		Name:       serverName,
		SSHKeys:    []*hcloud.SSHKey{{ID: sshKeyID}},
		ServerType: &hcloud.ServerType{Name: serverSize},
		UserData:   userData,
	})
	if err != nil {
		return err
	}
	if server.Server == nil {
		return fmt.Errorf("Failed to create server %q on Hetzner", serverName)
	}

	return nil
}

func (a *MachineCreateAction) waitForServer(
	ctx context.Context, serverName string,
) error {
	var (
		server          *hcloud.Server
		maxWaitDuration = 5 * time.Minute
		waitStartTime   = time.Now()
	)
	for {
		time.Sleep(5 * time.Second)
		if time.Since(waitStartTime) > maxWaitDuration {
			return fmt.Errorf("Timed out waiting for server %q to be running", serverName)
		}

		s, _, err := a.hetzner.Server.GetByName(ctx, serverName)
		server = s
		if err != nil {
			return err
		}
		if server == nil {
			return fmt.Errorf("Server %q not found", serverName)
		}
		if s.Status == hcloud.ServerStatusRunning {
			break
		}

		fmt.Printf("Server %q not running yet, waiting...\n", serverName)
	}

	fmt.Printf("Server %q created successfully\n", serverName)
	fmt.Printf("  ID:   %d\n", server.ID)
	fmt.Printf("  Name: %s\n", server.Name)
	fmt.Printf("  IPv4: %s\n", server.PublicNet.IPv4.IP.String())
	fmt.Printf("  IPv6: %s\n", server.PublicNet.IPv6.IP.String())

	return nil
}

var userData = `#!/bin/bash

useradd -m -s /bin/bash deploy
usermod -aG sudo deploy

echo "deploy ALL=(ALL) NOPASSWD:ALL" > /etc/sudoers.d/deploy

mkdir -p /home/deploy/.ssh
cp /root/.ssh/authorized_keys /home/deploy/.ssh/
chown -R deploy:deploy /home/deploy/.ssh
chmod 700 /home/deploy/.ssh
chmod 600 /home/deploy/.ssh/authorized_keys

sed -i 's/^#Port 22/Port 67/' /etc/ssh/sshd_config
sed -i 's/^Port 22/Port 67/' /etc/ssh/sshd_config

systemctl daemon-reload
systemctl restart ssh.socket
`
