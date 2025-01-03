package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"time"

	"github.com/markusylisiurunen/ship/internal/log"
	"github.com/markusylisiurunen/ship/internal/util"
	"github.com/urfave/cli/v3"
	"golang.org/x/crypto/ssh"
)

type secretsSetAction struct {
	client *ssh.Client
}

func newSecretsSetAction() *secretsSetAction {
	return &secretsSetAction{}
}

func (a *secretsSetAction) action(ctx context.Context, c *cli.Command) error {
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
		a.stepSetSecret,
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

func (a *secretsSetAction) stepSetSecret(ctx context.Context, c *cli.Command) (func(), error) {
	var cleanupFn func()
	var doer util.Doer
	doer.
		Do(func() error {
			cmd := fmt.Sprintf("mkdir -p /root/projects/%s/.secrets",
				c.String("name"))
			return a.runCommand(cmd)
		}).
		Do(func() error {
			key, value := c.Args().Get(0), c.Args().Get(1)
			cmd := fmt.Sprintf("echo -n %q > /root/projects/%s/.secrets/%s",
				value, c.String("name"), key)
			return a.runCommandDiscardOutput(cmd)
		})
	return cleanupFn, doer.Err()
}

// helpers
// ---

func (a *secretsSetAction) connectClient(c *cli.Command) error {
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

func (a *secretsSetAction) assertOperational(c *cli.Command) error {
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
	// make sure there are exactly two args
	if c.Args().Len() != 2 {
		return errors.New("exactly two args are required")
	}
	// make sure the key is valid
	if !regexp.MustCompile(`^[a-zA-Z0-9_]+$`).MatchString(c.Args().Get(0)) {
		return errors.New("key must be alphanumeric")
	}
	// all good
	return nil
}

func (a *secretsSetAction) runCommand(cmd string) error {
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

func (a *secretsSetAction) runCommandDiscardOutput(cmd string) error {
	sess, err := a.client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	defer sess.Close()
	sess.Stdout = io.Discard
	sess.Stderr = io.Discard
	log.Debugf("running a silent command")
	if err := sess.Run(cmd); err != nil {
		return fmt.Errorf("failed to run a command")
	}
	return nil
}
