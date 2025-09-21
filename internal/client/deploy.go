package client

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/bramvdbogaerde/go-scp"
	"github.com/hetznercloud/hcloud-go/v2/hcloud"
	"github.com/markusylisiurunen/ship/internal/constant"
	"github.com/urfave/cli/v3"
	"golang.org/x/crypto/ssh"
)

type DeployAction struct {
	version string
	hetzner *hcloud.Client
	ssh     *ssh.Client
}

func NewDeployAction(version string) *DeployAction {
	return &DeployAction{version: version}
}

func (a *DeployAction) init(ctx context.Context, cmd *cli.Command) (cleanup func(), initErr error) {
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

func (a *DeployAction) Action(ctx context.Context, cmd *cli.Command) error {
	// Initialize the Hetzner client and SSH connection
	cleanupInit, err := a.init(ctx, cmd)
	if err != nil {
		return err
	}
	defer cleanupInit()

	// Ensure the `agent` binary is on the machine
	var copyErr error
	if a.version == "dev" {
		copyErr = copyDevAgentBinaryToServer(ctx, a.ssh, false)
	} else {
		copyErr = copyVersionedAgentBinaryToServer(ctx, a.ssh, false, a.version)
	}
	if copyErr != nil {
		return fmt.Errorf("ensure agent binary on server: %w", copyErr)
	}

	// Create the archive of the current directory
	archivePath, cleanupArchive, err := a.createArchive()
	if err != nil {
		return fmt.Errorf("create app archive: %w", err)
	}
	defer cleanupArchive()

	// Upload the archive to the server
	var (
		appName    = cmd.String("app-name")
		appVersion = cmd.String("app-version")
	)
	if appName == "" || appVersion == "" {
		return fmt.Errorf("app name and version are required")
	}
	alphaNumericRegexp := regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	if !alphaNumericRegexp.MatchString(appName) {
		return fmt.Errorf("app name %q can only contain letters, numbers, dashes, and underscores", appName)
	}
	if !alphaNumericRegexp.MatchString(appVersion) {
		return fmt.Errorf("app version %q can only contain letters, numbers, dashes, and underscores", appVersion)
	}
	if err := a.uploadArchive(ctx, archivePath, appName, appVersion); err != nil {
		return fmt.Errorf("upload archive: %w", err)
	}

	// Execute the appropriate `agent` command on the machine
	sess, err := a.ssh.NewSession()
	if err != nil {
		return fmt.Errorf("create SSH session for deploy: %w", err)
	}
	defer sess.Close()
	sess.Stdout = os.Stdout
	sess.Stderr = os.Stderr
	deployCmd := fmt.Sprintf("/home/deploy/.ship/%s/agent deploy --app-name %s --app-version %s",
		a.version, appName, appVersion)
	if volumeNames := cmd.StringSlice("volume"); len(volumeNames) > 0 {
		for _, volumeName := range volumeNames {
			if !alphaNumericRegexp.MatchString(volumeName) {
				return fmt.Errorf("volume name %q can only contain letters, numbers, dashes, and underscores", volumeName)
			}
			deployCmd += fmt.Sprintf(" --volume-name %s", volumeName)
		}
	}
	fmt.Printf("Running deploy command: %s\n", deployCmd)
	if err := sess.Run(deployCmd); err != nil {
		return fmt.Errorf("run deploy command %q: %w", deployCmd, err)
	}

	return nil
}

func (a *DeployAction) createArchive() (string, func(), error) {
	tempFile, err := os.CreateTemp("", "ship*.zip")
	if err != nil {
		return "", nil, fmt.Errorf("create temp file for archive: %w", err)
	}
	defer tempFile.Close()

	cleanup := func() {
		if err := os.Remove(tempFile.Name()); err != nil {
			fmt.Printf("Failed to remove temp archive file %q: %v\n", tempFile.Name(), err)
		}
	}

	zipWriter := zip.NewWriter(tempFile)
	defer zipWriter.Close()

	cwd, err := os.Getwd()
	if err != nil {
		return "", cleanup, fmt.Errorf("determine working directory: %w", err)
	}

	walkErr := filepath.Walk(cwd, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("walk into %q: %w", path, walkErr)
		}

		relPath, err := filepath.Rel(cwd, path)
		if err != nil {
			return fmt.Errorf("relativize path %q: %w", path, err)
		}
		if relPath == "." {
			return nil
		}

		// Skip .git directories
		if info.IsDir() && info.Name() == ".git" {
			return filepath.SkipDir
		}

		// Write the ZIP header
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return fmt.Errorf("create zip header for %q: %w", relPath, err)
		}
		header.Name = relPath
		if info.IsDir() {
			header.Name += "/"
		}
		header.Method = zip.Deflate
		writer, err := zipWriter.CreateHeader(header)
		if err != nil {
			return fmt.Errorf("create zip entry for %q: %w", relPath, err)
		}

		// If it's a directory, nothing more to do
		if info.IsDir() {
			return nil
		}

		// Write the file content
		file, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("open %q: %w", path, err)
		}
		if _, err := io.Copy(writer, file); err != nil {
			file.Close()
			return fmt.Errorf("copy %q into archive: %w", path, err)
		}
		if err := file.Close(); err != nil {
			return fmt.Errorf("close %q after copying: %w", path, err)
		}

		return nil
	})
	if walkErr != nil {
		return "", cleanup, fmt.Errorf("walk project directory: %w", walkErr)
	}

	return tempFile.Name(), cleanup, nil
}

func (a *DeployAction) uploadArchive(
	ctx context.Context, localArchive, appName, appVersion string,
) error {
	// Open the local archive file
	archiveFile, err := os.Open(localArchive)
	if err != nil {
		return fmt.Errorf("open archive %q: %w", localArchive, err)
	}
	defer archiveFile.Close()

	// Make sure the remote directory exists
	remoteAppDir := fmt.Sprintf("/home/deploy/apps/%s/%s", appName, appVersion)
	sess, err := a.ssh.NewSession()
	if err != nil {
		return fmt.Errorf("create SSH session for archive upload: %w", err)
	}
	defer sess.Close()
	sess.Stdout = os.Stdout
	sess.Stderr = os.Stderr
	mkdirCmd := fmt.Sprintf("mkdir -p %s", remoteAppDir)
	if err := sess.Run(mkdirCmd); err != nil {
		return fmt.Errorf("run remote command %q: %w", mkdirCmd, err)
	}

	// Upload the archive to the server
	remoteArchive := fmt.Sprintf("%s/archive.zip", remoteAppDir)
	client, err := scp.NewClientBySSH(a.ssh)
	if err != nil {
		return fmt.Errorf("create SCP client: %w", err)
	}
	defer client.Close()
	if err := client.CopyFromFile(ctx, *archiveFile, remoteArchive, "0755"); err != nil {
		return fmt.Errorf("copy archive to %q: %w", remoteArchive, err)
	}

	return nil
}
