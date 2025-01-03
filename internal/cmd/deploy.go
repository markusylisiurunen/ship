package cmd

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/bramvdbogaerde/go-scp"
	"github.com/urfave/cli/v3"
	"golang.org/x/crypto/ssh"
)

type deployAction struct{}

func newDeployAction() *deployAction {
	return &deployAction{}
}

func (a *deployAction) action(ctx context.Context, c *cli.Command) error {
	steps := []func(context.Context, *cli.Command) error{
		a.archiveDirectory,
		a.copyArchiveToRemote,
		a.dockerCompose,
		a.caddyfile,
	}
	for _, step := range steps {
		err := step(ctx, c)
		if err != nil {
			return err
		}
	}
	return nil
}

func (a *deployAction) archiveDirectory(ctx context.Context, c *cli.Command) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}
	archive, err := os.Create("archive.zip")
	if err != nil {
		return fmt.Errorf("failed to create archive: %w", err)
	}
	defer archive.Close()
	zipWriter := zip.NewWriter(archive)
	defer zipWriter.Close()
	err = filepath.Walk(cwd, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		relPath, err := filepath.Rel(cwd, path)
		if err != nil {
			return fmt.Errorf("failed to get relative path: %w", err)
		}
		if relPath == "archive.zip" {
			return nil
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return fmt.Errorf("failed to create header: %w", err)
		}
		header.Name = relPath
		if info.IsDir() {
			header.Name += "/"
		}
		header.Method = zip.Deflate
		writer, err := zipWriter.CreateHeader(header)
		if err != nil {
			return fmt.Errorf("failed to create zip entry: %w", err)
		}
		if info.IsDir() {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("failed to open file %s: %w", path, err)
		}
		defer file.Close()
		_, err = io.Copy(writer, file)
		if err != nil {
			return fmt.Errorf("failed to write file %s to archive: %w", path, err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to walk directory: %w", err)
	}
	return nil
}

func (a *deployAction) copyArchiveToRemote(ctx context.Context, c *cli.Command) error {
	name, host, password := c.String("name"), c.String("host"), c.String("password")
	config := &ssh.ClientConfig{
		User:            "root",
		Auth:            []ssh.AuthMethod{ssh.Password(password)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}
	sshClient, err := ssh.Dial("tcp", host+":22", config)
	if err != nil {
		return fmt.Errorf("failed to dial: %w", err)
	}
	defer sshClient.Close()
	if err := a.executeOverSSH(ctx, sshClient, "mkdir -p /root/projects/"+name); err != nil {
		return fmt.Errorf("failed to create project directory: %w", err)
	}
	a.executeOverSSH(ctx, sshClient, fmt.Sprintf("rm /root/projects/%s/archive.zip", name))
	scpClient, err := scp.NewClientBySSH(sshClient)
	if err != nil {
		return fmt.Errorf("failed to create scp client: %w", err)
	}
	defer scpClient.Close()
	archive, _ := os.Open("archive.zip")
	defer archive.Close()
	err = scpClient.CopyFromFile(context.Background(), *archive, fmt.Sprintf("/root/projects/%s/archive.zip", name), "0655")
	if err != nil {
		return fmt.Errorf("failed to copy archive to remote: %w", err)
	}
	if err := a.executeOverSSH(ctx, sshClient, fmt.Sprintf("unzip -o /root/projects/%s/archive.zip -d /root/projects/%s", name, name)); err != nil {
		return fmt.Errorf("failed to unzip archive: %w", err)
	}
	a.executeOverSSH(ctx, sshClient, fmt.Sprintf("rm /root/projects/%s/archive.zip", name))
	return nil
}

func (a *deployAction) dockerCompose(ctx context.Context, c *cli.Command) error {
	name, version, host, password := c.String("name"), c.String("version"), c.String("host"), c.String("password")
	config := &ssh.ClientConfig{
		User:            "root",
		Auth:            []ssh.AuthMethod{ssh.Password(password)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}
	sshClient, err := ssh.Dial("tcp", host+":22", config)
	if err != nil {
		return fmt.Errorf("failed to dial: %w", err)
	}
	defer sshClient.Close()
	if err := a.executeOverSSH(ctx, sshClient, fmt.Sprintf("cd /root/projects/%s && docker compose pull", name)); err != nil {
		return fmt.Errorf("failed to pull images: %w", err)
	}
	if err := a.executeOverSSH(ctx, sshClient, fmt.Sprintf("cd /root/projects/%s && VERSION=%s docker compose build", name, version)); err != nil {
		return fmt.Errorf("failed to build containers: %w", err)
	}
	if err := a.executeOverSSH(ctx, sshClient, fmt.Sprintf("cd /root/projects/%s && VERSION=%s docker compose up -d --remove-orphans", name, version)); err != nil {
		return fmt.Errorf("failed to start containers: %w", err)
	}
	return nil
}

func (a *deployAction) caddyfile(ctx context.Context, c *cli.Command) error {
	name, host, password := c.String("name"), c.String("host"), c.String("password")
	config := &ssh.ClientConfig{
		User:            "root",
		Auth:            []ssh.AuthMethod{ssh.Password(password)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}
	sshClient, err := ssh.Dial("tcp", host+":22", config)
	if err != nil {
		return fmt.Errorf("failed to dial: %w", err)
	}
	defer sshClient.Close()
	if err := a.executeOverSSH(ctx, sshClient, fmt.Sprintf("mkdir -p /root/_caddy/sites-enabled")); err != nil {
		return fmt.Errorf("failed to create sites-enabled directory: %w", err)
	}
	if err := a.executeOverSSH(ctx, sshClient, fmt.Sprintf("cp /root/projects/%s/Caddyfile /root/_caddy/sites-enabled/%s", name, name)); err != nil {
		return fmt.Errorf("failed to copy Caddyfile: %w", err)
	}
	if err := a.executeOverSSH(ctx, sshClient, "cd /root/_caddy && docker compose exec caddy caddy reload --config /etc/caddy/Caddyfile"); err != nil {
		return fmt.Errorf("failed to reload Caddy: %w", err)
	}
	return nil
}

func (a *deployAction) executeOverSSH(ctx context.Context, client *ssh.Client, command string) error {
	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()
	session.Stdout = os.Stdout
	session.Stderr = os.Stderr
	fmt.Printf("Running command: %s\n", command)
	return session.Run(command)
}
