package cmd

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/bramvdbogaerde/go-scp"
	"github.com/markusylisiurunen/ship/internal/log"
	"github.com/markusylisiurunen/ship/internal/util"
	"github.com/urfave/cli/v3"
	"golang.org/x/crypto/ssh"
)

type deployAction struct {
	client *ssh.Client
}

func newDeployAction() *deployAction {
	return &deployAction{}
}

func (a *deployAction) action(ctx context.Context, c *cli.Command) error {
	if err := a.connectClient(c); err != nil {
		return err
	}
	defer a.client.Close()
	if err := a.assertDeployable(c); err != nil {
		return err
	}
	type CleanupFn = func()
	type StepFn = func(context.Context, *cli.Command) (CleanupFn, error)
	cleanupFns := make([]CleanupFn, 0)
	defer func() {
		for _, cleanupFn := range cleanupFns {
			if cleanupFn != nil {
				cleanupFn()
			}
		}
	}()
	stepFns := []StepFn{
		a.stepArchive,
		a.stepCopy,
		a.stepVolumes,
		a.stepSecrets,
		a.stepLink,
		a.stepDocker,
		a.stepCaddy,
	}
	for _, stepFn := range stepFns {
		cleanupFn, err := stepFn(ctx, c)
		if err != nil {
			return err
		}
		cleanupFns = append(cleanupFns, cleanupFn)
	}
	return nil
}

// steps
// ---

func (a *deployAction) stepArchive(ctx context.Context, c *cli.Command) (func(), error) {
	var cleanupFn func()
	// create the archive.zip file
	archive, err := os.Create("archive.zip")
	if err != nil {
		return cleanupFn, fmt.Errorf("failed to create archive: %w", err)
	}
	defer archive.Close()
	// cleanup the archive file
	cleanupFn = func() {
		if err := os.Remove("archive.zip"); err != nil {
			log.Errorf("failed to remove archive: %v", err)
		}
	}
	// create the zip writer
	zipWriter := zip.NewWriter(archive)
	defer zipWriter.Close()
	// walk the current working directory and add files to the archive
	cwd, err := os.Getwd()
	if err != nil {
		return cleanupFn, fmt.Errorf("failed to get working directory: %w", err)
	}
	if err := filepath.Walk(cwd, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(cwd, path)
		if err != nil {
			return fmt.Errorf("failed to get relative path: %w", err)
		}
		// ignore the archive.zip file
		if relPath == "archive.zip" {
			return nil
		}
		// ignore the .git directory
		if info.IsDir() && info.Name() == ".git" {
			return filepath.SkipDir
		}
		// create the zip entry
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return fmt.Errorf("failed to create zip file header: %w", err)
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
		// write the file to the zip entry
		file, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("failed to open file %q: %w", path, err)
		}
		defer file.Close()
		if _, err := io.Copy(writer, file); err != nil {
			return fmt.Errorf("failed to write file %q to archive: %w", path, err)
		}
		return nil
	}); err != nil {
		return cleanupFn, fmt.Errorf("failed to walk directory: %w", err)
	}
	return cleanupFn, nil
}

func (a *deployAction) stepCopy(ctx context.Context, c *cli.Command) (func(), error) {
	var cleanupFn func()
	var doer util.Doer
	doer.
		Do(func() error {
			cmd := fmt.Sprintf("mkdir -p /root/projects/%s/%s",
				c.String("name"), c.String("version"))
			return a.runCommand(cmd)
		}).
		Do(func() error {
			client, err := scp.NewClientBySSH(a.client)
			if err != nil {
				return fmt.Errorf("failed to create scp client: %w", err)
			}
			defer client.Close()
			archive, err := os.Open("archive.zip")
			if err != nil {
				return fmt.Errorf("failed to open archive: %w", err)
			}
			defer archive.Close()
			if err := client.CopyFromFile(ctx, *archive,
				fmt.Sprintf("/root/projects/%s/%s/archive.zip",
					c.String("name"), c.String("version")),
				"0655",
			); err != nil {
				return fmt.Errorf("failed to copy archive to remote: %w", err)
			}
			return nil
		}).
		Do(func() error {
			cmd := fmt.Sprintf("unzip -o /root/projects/%s/%s/archive.zip -d /root/projects/%s/%s",
				c.String("name"), c.String("version"), c.String("name"), c.String("version"))
			return a.runCommand(cmd)
		}).
		Do(func() error {
			cmd := fmt.Sprintf("rm /root/projects/%s/%s/archive.zip",
				c.String("name"), c.String("version"))
			return a.runSilentCommand(cmd)
		})
	return cleanupFn, doer.Err()
}

func (a *deployAction) stepVolumes(ctx context.Context, c *cli.Command) (func(), error) {
	var cleanupFn func()
	var doer util.Doer
	doer.Do(func() error {
		cmd := fmt.Sprintf("mkdir -p /root/projects/%s/.data",
			c.String("name"))
		return a.runCommand(cmd)
	})
	for _, volume := range c.StringSlice("volumes") {
		doer.Do(func() error {
			cmd := fmt.Sprintf("mkdir -p /root/projects/%s/.data/%s",
				c.String("name"), volume)
			return a.runCommand(cmd)
		})
	}
	doer.Do(func() error {
		cmd := fmt.Sprintf("ln -sfn /root/projects/%s/.data /root/projects/%s/%s/.data",
			c.String("name"), c.String("name"), c.String("version"))
		return a.runCommand(cmd)
	})
	return cleanupFn, doer.Err()
}

func (a *deployAction) stepSecrets(ctx context.Context, c *cli.Command) (func(), error) {
	var cleanupFn func()
	var doer util.Doer
	doer.
		Do(func() error {
			cmd := fmt.Sprintf("mkdir -p /root/projects/%s/.secrets",
				c.String("name"))
			return a.runCommand(cmd)
		}).
		Do(func() error {
			cmd := fmt.Sprintf("ln -sfn /root/projects/%s/.secrets /root/projects/%s/%s/.secrets",
				c.String("name"), c.String("name"), c.String("version"))
			return a.runCommand(cmd)
		})
	return cleanupFn, doer.Err()
}

func (a *deployAction) stepLink(ctx context.Context, c *cli.Command) (func(), error) {
	var cleanupFn func()
	var doer util.Doer
	doer.
		Do(func() error {
			cmd := fmt.Sprintf("ln -sfn /root/projects/%s/%s /root/projects/%s/.current",
				c.String("name"), c.String("version"), c.String("name"))
			return a.runCommand(cmd)
		})
	return cleanupFn, doer.Err()
}

func (a *deployAction) stepDocker(ctx context.Context, c *cli.Command) (func(), error) {
	var cleanupFn func()
	var doer util.Doer
	doer.
		Do(func() error {
			cmd := fmt.Sprintf("cd /root/projects/%s/.current && docker compose pull",
				c.String("name"))
			return a.runCommand(cmd)
		}).
		Do(func() error {
			cmd := fmt.Sprintf("cd /root/projects/%s/.current && VERSION=%s docker compose build",
				c.String("name"), c.String("version"))
			return a.runCommand(cmd)
		}).
		Do(func() error {
			cmd := fmt.Sprintf("cd /root/projects/%s/.current && VERSION=%s docker compose up -d --remove-orphans",
				c.String("name"), c.String("version"))
			return a.runCommand(cmd)
		})
	return cleanupFn, doer.Err()
}

func (a *deployAction) stepCaddy(ctx context.Context, c *cli.Command) (func(), error) {
	var cleanupFn func()
	var doer util.Doer
	doer.
		Do(func() error {
			cmd := "mkdir -p /root/_caddy/sites-enabled"
			return a.runCommand(cmd)
		}).
		Do(func() error {
			cmd := fmt.Sprintf("cp /root/projects/%s/.current/Caddyfile /root/_caddy/sites-enabled/%s",
				c.String("name"), c.String("name"))
			return a.runCommand(cmd)
		}).
		Do(func() error {
			cmd := "cd /root/_caddy && docker compose exec caddy caddy reload --config /etc/caddy/Caddyfile"
			return a.runCommand(cmd)
		})
	return cleanupFn, doer.Err()
}

// helpers
// ---

func (a *deployAction) connectClient(c *cli.Command) error {
	if a.client != nil {
		return nil
	}
	var (
		name     = c.String("name")
		host     = c.String("host")
		password = c.String("password")
	)
	if name == "" || host == "" || password == "" {
		return errors.New("name, host and password are required")
	}
	client, err := ssh.Dial("tcp", host+":22", &ssh.ClientConfig{
		User:            "root",
		Auth:            []ssh.AuthMethod{ssh.Password(password)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	})
	if err != nil {
		return fmt.Errorf("failed to dial: %w", err)
	}
	a.client = client
	return nil
}

func (a *deployAction) assertDeployable(c *cli.Command) error {
	// make sure the required flags are set
	if c.String("name") == "" {
		return errors.New("name is required")
	}
	if c.String("version") == "" {
		return errors.New("version is required")
	}
	if c.String("host") == "" {
		return errors.New("host is required")
	}
	if c.String("password") == "" {
		return errors.New("password is required")
	}
	// make sure volumes are valid
	for _, volume := range c.StringSlice("volumes") {
		if !regexp.MustCompile(`^[a-zA-Z0-9_-]+$`).MatchString(volume) {
			return errors.New("volumes must be alphanumeric, with dashes and underscores allowed")
		}
	}
	// make sure the version has not already been deployed
	if err := a.runCheckExitCodeCommand(
		fmt.Sprintf("test ! -d /root/projects/%s/%s",
			c.String("name"), c.String("version")),
		0,
	); err != nil {
		return errors.New("version already deployed")
	}
	// all good
	return nil
}

func (a *deployAction) runCheckExitCodeCommand(cmd string, expectedCode int) error {
	sess, err := a.client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	defer sess.Close()
	err = sess.Run(cmd)
	if err == nil {
		if expectedCode == 0 {
			return nil
		}
		return fmt.Errorf("expected exit code %d, got 0", expectedCode)
	}
	var exitErr *ssh.ExitError
	if errors.As(err, &exitErr) {
		actualCode := exitErr.ExitStatus()
		if actualCode == expectedCode {
			return nil
		}
		return fmt.Errorf("expected exit code %d, got %d", expectedCode, actualCode)
	}
	return fmt.Errorf("failed to run command: %w", err)
}

func (a *deployAction) runCommand(cmd string) error {
	sess, err := a.client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	defer sess.Close()
	sess.Stdout = os.Stdout
	sess.Stderr = os.Stderr
	log.Debugf("running command: %q", cmd)
	if err := sess.Run(cmd); err != nil {
		return fmt.Errorf("failed to run command: %w", err)
	}
	return nil
}

func (a *deployAction) runSilentCommand(cmd string) error {
	sess, err := a.client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	defer sess.Close()
	sess.Stdout = os.Stdout
	sess.Stderr = os.Stderr
	log.Debugf("running command: %q", cmd)
	if err := sess.Run(cmd); err != nil {
		var exitErr *ssh.ExitError
		if errors.As(err, &exitErr) {
			return nil
		}
		return fmt.Errorf("failed to run command: %w", err)
	}
	return nil
}
