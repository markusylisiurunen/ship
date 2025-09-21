package agent

import (
	"context"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"
)

func Execute(ctx context.Context, version string) {
	cmd := &cli.Command{
		Name:    "ship",
		Usage:   "deploy an app to a VPS",
		Version: version,
		Commands: []*cli.Command{
			{
				Name:   "up",
				Usage:  "reconcile the machine to an up-to-date state",
				Action: NewUpAction(version).Action,
			},
			{
				Name:  "deploy",
				Usage: "deploy an app",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "app-name", Usage: "application name", Required: true},
					&cli.StringFlag{Name: "app-version", Usage: "application version", Required: true},
					&cli.StringSliceFlag{Name: "volume-name", Usage: "volume name (can be specified multiple times)"},
				},
				Action: NewDeployAction().Action,
			},
		},
	}
	if err := cmd.Run(ctx, os.Args); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}

// checkFileExists checks if a file exists at the given path.
func checkFileExists(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("file does not exist: %s", path)
	} else if err != nil {
		return fmt.Errorf("error checking file %s: %w", path, err)
	}
	return nil
}

// ensureDirExists checks if a directory exists at the given path, and creates it with the specified mode if it does not.
func ensureDirExists(path string, perm os.FileMode) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.MkdirAll(path, perm); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", path, err)
		}
	}
	return nil
}

// listDirEntries lists the entries in the specified directory.
func listDirEntries(path string) ([]os.DirEntry, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return []os.DirEntry{}, nil
	} else if err != nil {
		return nil, fmt.Errorf("error checking directory %s: %w", path, err)
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory %s: %w", path, err)
	}
	return entries, nil
}

// removeFile removes the file at the specified path if it exists.
func removeFile(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove file %s: %w", path, err)
	}
	return nil
}

// symlink creates or updates a symbolic link from oldname to newname.
func symlink(src, dst string) error {
	info, err := os.Lstat(dst)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("stat %s: %w", dst, err)
	}
	if err == nil && (info.Mode()&os.ModeSymlink) == 0 {
		return fmt.Errorf("destination %s exists and is not a symlink", dst)
	}
	if err := os.Remove(dst); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove %s: %w", dst, err)
	}
	if err := os.Symlink(src, dst); err != nil {
		return fmt.Errorf("symlink %s -> %s: %w", dst, src, err)
	}
	return nil
}
