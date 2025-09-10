package cmd

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/bramvdbogaerde/go-scp"
	"github.com/markusylisiurunen/ship/internal/log"
	"github.com/markusylisiurunen/ship/internal/util"
	"github.com/urfave/cli/v3"
	"golang.org/x/crypto/ssh"
)

type deployAction struct {
	client  *ssh.Client
	homeDir string
}

func newDeployAction() *deployAction {
	return &deployAction{}
}

func (a *deployAction) action(ctx context.Context, c *cli.Command) error {
	if err := a.dial(c); err != nil {
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
		a.stepHomeDir,
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

// steps -------------------------------------------------------------------------------------------

func (a *deployAction) stepHomeDir(ctx context.Context, c *cli.Command) (func(), error) {
	var cleanupFn func()
	out, err := a.runCaptureCommand("echo $HOME")
	if err != nil {
		return cleanupFn, fmt.Errorf("failed to get home directory: %w", err)
	}
	a.homeDir = strings.TrimSpace(out)
	return cleanupFn, nil
}

func (a *deployAction) stepArchive(ctx context.Context, c *cli.Command) (func(), error) {
	var cleanupFn func()
	archive, err := os.Create("archive.zip")
	if err != nil {
		return cleanupFn, fmt.Errorf("failed to create archive: %w", err)
	}
	defer archive.Close()
	cleanupFn = func() {
		if err := os.Remove("archive.zip"); err != nil {
			log.Errorf("failed to remove archive: %v", err)
		}
	}
	zipWriter := zip.NewWriter(archive)
	defer zipWriter.Close()
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
		if relPath == "archive.zip" {
			return nil
		}
		if info.IsDir() && info.Name() == ".git" {
			return filepath.SkipDir
		}
		// TODO: should allow ignoring other files too
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
			cmd := fmt.Sprintf("mkdir -p $HOME/projects/%s/%s",
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
				fmt.Sprintf("%s/projects/%s/%s/archive.zip",
					a.homeDir, c.String("name"), c.String("version")),
				"0655",
			); err != nil {
				return fmt.Errorf("failed to copy archive to remote: %w", err)
			}
			return nil
		}).
		Do(func() error {
			cmd := fmt.Sprintf("unzip -o $HOME/projects/%s/%s/archive.zip -d $HOME/projects/%s/%s",
				c.String("name"), c.String("version"), c.String("name"), c.String("version"))
			return a.runCommand(cmd)
		}).
		Do(func() error {
			cmd := fmt.Sprintf("rm $HOME/projects/%s/%s/archive.zip",
				c.String("name"), c.String("version"))
			return a.runSilentCommand(cmd)
		})
	return cleanupFn, doer.Err()
}

func (a *deployAction) stepVolumes(ctx context.Context, c *cli.Command) (func(), error) {
	var cleanupFn func()
	var doer util.Doer
	doer.Do(func() error {
		cmd := fmt.Sprintf("mkdir -p $HOME/projects/%s/.data",
			c.String("name"))
		return a.runCommand(cmd)
	})
	for _, volume := range c.StringSlice("volumes") {
		doer.Do(func() error {
			cmd := fmt.Sprintf("mkdir -p $HOME/projects/%s/.data/%s",
				c.String("name"), volume)
			return a.runCommand(cmd)
		})
	}
	doer.Do(func() error {
		cmd := fmt.Sprintf("ln -sfn $HOME/projects/%s/.data $HOME/projects/%s/%s/.data",
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
			cmd := fmt.Sprintf("mkdir -p $HOME/projects/%s/.secrets",
				c.String("name"))
			return a.runCommand(cmd)
		}).
		Do(func() error {
			cmd := fmt.Sprintf("ln -sfn $HOME/projects/%s/.secrets $HOME/projects/%s/%s/.secrets",
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
			cmd := fmt.Sprintf("ln -sfn $HOME/projects/%s/%s $HOME/projects/%s/.current",
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
			cmd := fmt.Sprintf("cd $HOME/projects/%s/.current && docker compose pull",
				c.String("name"))
			return a.runCommand(cmd)
		}).
		Do(func() error {
			cmd := fmt.Sprintf("cd $HOME/projects/%s/.current && VERSION=%s docker compose build",
				c.String("name"), c.String("version"))
			return a.runCommand(cmd)
		}).
		Do(func() error {
			cmd := fmt.Sprintf("cd $HOME/projects/%s/.current && VERSION=%s docker compose up -d --remove-orphans",
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
			cmd := "mkdir -p $HOME/_caddy/sites-enabled"
			return a.runCommand(cmd)
		}).
		Do(func() error {
			cmd := fmt.Sprintf("cp $HOME/projects/%s/.current/Caddyfile $HOME/_caddy/sites-enabled/%s",
				c.String("name"), c.String("name"))
			return a.runCommand(cmd)
		}).
		Do(func() error {
			cmd := "cd $HOME/_caddy && docker compose exec caddy caddy reload --config /etc/caddy/Caddyfile"
			return a.runCommand(cmd)
		})
	return cleanupFn, doer.Err()
}

// helpers -----------------------------------------------------------------------------------------

func (a *deployAction) dial(c *cli.Command) error {
	if a.client != nil {
		return nil
	}
	var (
		host    = c.String("host")
		user    = c.String("user")
		keyFile = c.String("key-file")
	)
	if host == "" || user == "" || keyFile == "" {
		return errors.New("--host, --user and --key-file are required")
	}
	key, err := os.ReadFile(keyFile)
	if err != nil {
		return err
	}
	signer, err := ssh.ParsePrivateKey(key)
	if err != nil {
		return err
	}
	client, err := ssh.Dial("tcp", host+":22", &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
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
	requiredFlags := []string{"name", "version", "host", "user", "key-file"}
	for _, flag := range requiredFlags {
		if c.String(flag) == "" {
			return fmt.Errorf("%s is required", flag)
		}
	}
	if !regexp.MustCompile(`^[a-zA-Z0-9_-]+$`).MatchString(c.String("version")) {
		return errors.New("version must be alphanumeric, with dashes and underscores allowed")
	}
	for _, volume := range c.StringSlice("volumes") {
		if !regexp.MustCompile(`^[a-zA-Z0-9_-]+$`).MatchString(volume) {
			return errors.New("volumes must be alphanumeric, with dashes and underscores allowed")
		}
	}
	if err := a.runCheckExitCodeCommand(
		fmt.Sprintf("test ! -d $HOME/projects/%s/%s",
			c.String("name"), c.String("version")),
		0,
	); err != nil {
		return errors.New("version already deployed")
	}
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

func (a *deployAction) runCaptureCommand(cmd string) (string, error) {
	var output bytes.Buffer
	sess, err := a.client.NewSession()
	if err != nil {
		return "", fmt.Errorf("failed to create session: %w", err)
	}
	defer sess.Close()
	sess.Stdout = &output
	sess.Stderr = os.Stderr
	log.Debugf("running command: %q", cmd)
	if err := sess.Run(cmd); err != nil {
		return "", fmt.Errorf("failed to run command: %w", err)
	}
	return output.String(), nil
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
	if err := a.runCommand(cmd); err != nil {
		var exitErr *ssh.ExitError
		if errors.As(err, &exitErr) {
			return nil
		}
		return fmt.Errorf("failed to run command: %w", err)
	}
	return nil
}
