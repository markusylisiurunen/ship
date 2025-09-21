package agent

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"

	"github.com/urfave/cli/v3"
)

const (
	appVolumesDirPerm os.FileMode = 0o770
	appSecretsDirPerm os.FileMode = 0o750
	appShipDirPerm    os.FileMode = 0o750
)

type deployArgs struct {
	AppName     string
	AppVersion  string
	VolumeNames []string
}

func (a *deployArgs) parse(cmd *cli.Command) {
	a.AppName = cmd.String("app-name")
	a.AppVersion = cmd.String("app-version")
	a.VolumeNames = cmd.StringSlice("volume-name")
}

func (a deployArgs) validate() error {
	alphaNumRegex := regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	if a.AppName == "" {
		return fmt.Errorf("app name is required")
	}
	if !alphaNumRegex.MatchString(a.AppName) {
		return fmt.Errorf("app name can only contain letters, numbers, dashes, and underscores")
	}
	if a.AppVersion == "" {
		return fmt.Errorf("app version is required")
	}
	if !alphaNumRegex.MatchString(a.AppVersion) {
		return fmt.Errorf("app version can only contain letters, numbers, dashes, and underscores")
	}
	if len(a.VolumeNames) > 0 {
		for _, v := range a.VolumeNames {
			if v == "" {
				return fmt.Errorf("volume name cannot be empty")
			}
			if !alphaNumRegex.MatchString(v) {
				return fmt.Errorf("volume name %q can only contain letters, numbers, dashes, and underscores", v)
			}
		}
	}
	return nil
}

type DeployAction struct {
	args deployArgs
}

func NewDeployAction() *DeployAction {
	return &DeployAction{}
}

func (a *DeployAction) Action(ctx context.Context, cmd *cli.Command) error {
	a.args = deployArgs{}
	a.args.parse(cmd)
	if err := a.args.validate(); err != nil {
		return err
	}

	archivePath := filepath.Join("/home/deploy/apps", a.args.AppName, a.args.AppVersion, "archive.zip")
	if err := checkFileExists(archivePath); err != nil {
		return err
	}

	if entries, err := listDirEntries(filepath.Dir(archivePath)); err != nil {
		return err
	} else if len(entries) > 1 {
		return fmt.Errorf("archive directory %q is not empty", filepath.Dir(archivePath))
	}

	if err := a.execRun(ctx, "unzip", "-oq", archivePath, "-d", filepath.Dir(archivePath)); err != nil {
		return err
	}
	if err := removeFile(archivePath); err != nil {
		fmt.Printf("Failed to remove the archive.zip file: %v\n", err)
	}

	for _, dir := range []struct {
		path string
		perm os.FileMode
	}{
		{path: filepath.Join("/home/deploy/apps", a.args.AppName, "volumes"), perm: appVolumesDirPerm},
		{path: filepath.Join("/home/deploy/apps", a.args.AppName, "secrets"), perm: appSecretsDirPerm},
		{path: filepath.Join("/home/deploy/apps", a.args.AppName, a.args.AppVersion, ".ship"), perm: appShipDirPerm},
	} {
		if err := ensureDirExists(dir.path, dir.perm); err != nil {
			return err
		}
	}

	if len(a.args.VolumeNames) > 0 {
		for _, v := range a.args.VolumeNames {
			volumePath := filepath.Join("/home/deploy/apps", a.args.AppName, "volumes", v)
			if err := ensureDirExists(volumePath, appVolumesDirPerm); err != nil {
				return err
			}
		}
	}

	for _, l := range []struct {
		src string
		dst string
	}{
		{
			src: filepath.Join("/home/deploy/apps", a.args.AppName, "volumes"),
			dst: filepath.Join("/home/deploy/apps", a.args.AppName, a.args.AppVersion, ".ship", "volumes"),
		},
		{
			src: filepath.Join("/home/deploy/apps", a.args.AppName, "secrets"),
			dst: filepath.Join("/home/deploy/apps", a.args.AppName, a.args.AppVersion, ".ship", "secrets"),
		},
		{
			src: filepath.Join("/home/deploy/apps", a.args.AppName, a.args.AppVersion),
			dst: filepath.Join("/home/deploy/apps", a.args.AppName, "current"),
		},
	} {
		if err := symlink(l.src, l.dst); err != nil {
			return err
		}
	}

	if err := checkFileExists(
		filepath.Join("/home/deploy/apps", a.args.AppName, a.args.AppVersion, ".ship", "compose.yml"),
	); err == nil {
		for _, c := range [][]string{
			{"docker", "compose", "pull", "-f", "./.ship/compose.yml"},
			{"docker", "compose", "build", "-f", "./.ship/compose.yml", "--pull", "--build-arg", "VERSION=" + a.args.AppVersion},
			{"docker", "compose", "up", "-f", "./.ship/compose.yml", "-d", "--remove-orphans", "--no-build"},
		} {
			if err := a.execRunInDir(
				ctx,
				filepath.Join("/home/deploy/apps", a.args.AppName, a.args.AppVersion),
				c[0], c[1:]...,
			); err != nil {
				return err
			}
		}
	} else {
		fmt.Printf("No .ship/compose.yml found, skipping Docker Compose steps\n")
	}

	if err := checkFileExists(
		filepath.Join("/home/deploy/apps", a.args.AppName, a.args.AppVersion, ".ship", "Caddyfile"),
	); err == nil {
		for _, c := range [][]string{
			{"sudo", "cp", "./.ship/Caddyfile", "/root/.caddy/sites-enabled/" + a.args.AppName},
			{"sudo", "chown", "root:root", "/root/.caddy/sites-enabled/" + a.args.AppName},
			{"sudo", "chmod", "644", "/root/.caddy/sites-enabled/" + a.args.AppName},
			{"sudo", "bash", "-c", "cd /root/.caddy && docker compose exec caddy caddy reload --config /etc/caddy/Caddyfile"},
		} {
			if err := a.execRunInDir(
				ctx,
				filepath.Join("/home/deploy/apps", a.args.AppName, a.args.AppVersion),
				c[0], c[1:]...,
			); err != nil {
				return err
			}
		}
	} else {
		fmt.Printf("No .ship/Caddyfile found, skipping Caddy steps\n")
	}

	return nil
}

func (a *DeployAction) execRun(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (a *DeployAction) execRunInDir(ctx context.Context, dir string, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
