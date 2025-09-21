package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/urfave/cli/v3"
)

type DeployAction struct{}

func NewDeployAction() *DeployAction {
	return &DeployAction{}
}

func (a *DeployAction) Action(ctx context.Context, cmd *cli.Command) error {
	var (
		appName    = cmd.String("app-name")
		appVersion = cmd.String("app-version")
	)
	if appName == "" || appVersion == "" {
		return fmt.Errorf("app name and version are required")
	}

	// Check that the `archive.zip` file exists where it is expected to be
	archivePath := filepath.Join("/home/deploy/apps", appName, appVersion, "archive.zip")
	if _, err := os.Stat(archivePath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("Archive %q not found", archivePath)
		}
		return err
	}

	// Make sure there are no other files in the archive directory
	if err := a.cleanArchiveDir(filepath.Dir(archivePath)); err != nil {
		return err
	}

	// Extract the archive.zip file
	if err := a.extractArchive(ctx, archivePath); err != nil {
		return err
	}
	if err := os.Remove(archivePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		fmt.Printf("Warning: failed to remove archive %q: %v\n", archivePath, err)
	}

	// Make sure the `data` and `secrets` directories exist
	dirsToCreate := []string{
		"data",
		"secrets",
		fmt.Sprintf("%s/.ship", appVersion),
	}
	for _, dir := range dirsToCreate {
		path := filepath.Join("/home/deploy/apps", appName, dir)
		if err := os.MkdirAll(path, 0o777); err != nil {
			return err
		}
	}

	// Make sure the `data` and `secrets` directories are symlinked into the app directory
	symlinksToCreate := []struct {
		src string
		dst string
	}{
		{
			src: filepath.Join("/home/deploy/apps", appName, "data"),
			dst: filepath.Join("/home/deploy/apps", appName, appVersion, ".ship", "data"),
		},
		{
			src: filepath.Join("/home/deploy/apps", appName, "secrets"),
			dst: filepath.Join("/home/deploy/apps", appName, appVersion, ".ship", "secrets"),
		},
	}
	for _, link := range symlinksToCreate {
		if err := os.RemoveAll(link.dst); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := os.Symlink(link.src, link.dst); err != nil {
			return err
		}
	}

	// Make sure `current` symlink points to the new version
	if err := a.updateCurrentSymlink(appName, appVersion); err != nil {
		return err
	}

	// Restart the application using Docker Compose
	if err := a.restartApp(ctx, appName, appVersion); err != nil {
		return err
	}

	// Reconcile Caddy configuration if a Caddyfile is present
	if err := a.reconcileCaddy(ctx, appName, appVersion); err != nil {
		return err
	}

	return nil
}

func (a *DeployAction) cleanArchiveDir(archiveDir string) error {
	entries, err := os.ReadDir(archiveDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.Name() == "archive.zip" {
			continue
		}
		path := filepath.Join(archiveDir, entry.Name())
		if err := os.RemoveAll(path); err != nil {
			return err
		}
	}

	return nil
}

func (a *DeployAction) extractArchive(ctx context.Context, archivePath string) error {
	cmd := exec.CommandContext(ctx, "unzip", "-oq", archivePath, "-d", filepath.Dir(archivePath))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (a *DeployAction) updateCurrentSymlink(appName, appVersion string) error {
	currentLink := filepath.Join("/home/deploy/apps", appName, "current")

	if err := os.RemoveAll(currentLink); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	if err := os.Symlink(
		filepath.Join("/home/deploy/apps", appName, appVersion),
		currentLink,
	); err != nil {
		return err
	}

	return nil
}

func (a *DeployAction) restartApp(ctx context.Context, appName, appVersion string) error {
	composeFilePath := filepath.Join("/home/deploy/apps", appName, appVersion, "compose.yml")
	if _, err := os.Stat(composeFilePath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Printf("No compose.yml found for app %q, skipping Docker Compose steps\n", appName)
			return nil
		}
		return err
	}

	for _, cmd := range []struct {
		name string
		args []string
		env  []string
	}{
		{"docker", []string{"compose", "pull"}, nil},
		{"docker", []string{"compose", "build"}, []string{"VERSION=" + appVersion}},
		{"docker", []string{"compose", "up", "-d", "--remove-orphans"}, []string{"VERSION=" + appVersion}},
	} {
		c := exec.CommandContext(ctx, cmd.name, cmd.args...)
		c.Dir = filepath.Dir(composeFilePath)
		c.Env = append(os.Environ(), cmd.env...)
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		if err := c.Run(); err != nil {
			return err
		}
	}

	return nil
}

func (a *DeployAction) reconcileCaddy(ctx context.Context, appName, appVersion string) error {
	caddyfilePath := filepath.Join("/home/deploy/apps", appName, appVersion, "Caddyfile")
	if _, err := os.Stat(caddyfilePath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Printf("No Caddyfile found for app %q, skipping Caddy reconciliation\n", appName)
			return nil
		}
		return err
	}

	caddyRoot := "/root/.caddy"
	caddySitesEnabled := filepath.Join(caddyRoot, "sites-enabled")

	if err := copyFile(
		caddyfilePath,
		filepath.Join(caddySitesEnabled, appName),
	); err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, "docker", "compose", "exec", "caddy",
		"caddy", "reload", "--config", "/etc/caddy/Caddyfile")
	cmd.Dir = caddyRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}

	return nil
}
