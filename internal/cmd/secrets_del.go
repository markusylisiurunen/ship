package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"time"

	"github.com/markusylisiurunen/ship/internal/log"
	"github.com/markusylisiurunen/ship/internal/util"
	"github.com/urfave/cli/v3"
	"golang.org/x/crypto/ssh"
)

type secretsDelAction struct {
	client *ssh.Client
}

func newSecretsDelAction() *secretsDelAction {
	return &secretsDelAction{}
}

func (a *secretsDelAction) action(ctx context.Context, c *cli.Command) error {
	if err := a.connectClient(c); err != nil {
		return err
	}
	defer a.client.Close()
	if err := a.assertOperational(c); err != nil {
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
		a.stepDelSecret,
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

func (a *secretsDelAction) stepDelSecret(ctx context.Context, c *cli.Command) (func(), error) {
	var cleanupFn func()
	var doer util.Doer
	doer.
		Do(func() error {
			key := c.Args().Get(0)
			cmd := fmt.Sprintf("test -f /root/projects/%s/.secrets/%s && rm /root/projects/%s/.secrets/%s || true",
				c.String("name"), key, c.String("name"), key)
			return a.runCommand(cmd)
		})
	return cleanupFn, doer.Err()
}

// helpers
// ---

func (a *secretsDelAction) connectClient(c *cli.Command) error {
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

func (a *secretsDelAction) assertOperational(c *cli.Command) error {
	// make sure the required flags are set
	if c.String("name") == "" {
		return errors.New("name is required")
	}
	if c.String("host") == "" {
		return errors.New("host is required")
	}
	if c.String("password") == "" {
		return errors.New("password is required")
	}
	// make sure there are exactly one arg
	if c.Args().Len() != 1 {
		return errors.New("exactly one arg is required")
	}
	// make sure the key is valid
	if !regexp.MustCompile(`^[a-zA-Z0-9_]+$`).MatchString(c.Args().Get(0)) {
		return errors.New("key must be alphanumeric")
	}
	// all good
	return nil
}

func (a *secretsDelAction) runCommand(cmd string) error {
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
