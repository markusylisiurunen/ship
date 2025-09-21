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
		initErr = fmt.Errorf("Hetzner API token is required")
		return
	}
	a.hetzner = hcloud.NewClient(hcloud.WithToken(token))

	serverName := cmd.String("server-name")
	if serverName == "" {
		initErr = fmt.Errorf("Server name is required")
		return
	}
	server, _, err := a.hetzner.Server.GetByName(ctx, serverName)
	if err != nil {
		initErr = err
		return
	}
	if server == nil {
		initErr = fmt.Errorf("Server %q not found", serverName)
		return
	}

	sshPrivateKey := cmd.String("ssh-private-key")
	if sshPrivateKey == "" {
		initErr = fmt.Errorf("SSH private key is required")
		return
	}
	privateKey, err := os.ReadFile(sshPrivateKey)
	if err != nil {
		initErr = err
		return
	}
	signer, err := ssh.ParsePrivateKey(privateKey)
	if err != nil {
		initErr = err
		return
	}
	if client, err := ssh.Dial(
		"tcp",
		fmt.Sprintf("%s:67", server.PublicNet.IPv4.IP.String()),
		&ssh.ClientConfig{
			Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			Timeout:         10 * time.Second,
			User:            "deploy",
		},
	); err != nil {
		initErr = err
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
		return copyErr
	}

	// Create the archive of the current directory
	archivePath, cleanupArchive, err := a.createArchive()
	if err != nil {
		return err
	}
	defer cleanupArchive()

	// Upload the archive to the server
	var (
		appName    = cmd.String("app-name")
		appVersion = cmd.String("app-version")
	)
	if appName == "" || appVersion == "" {
		return fmt.Errorf("App name and version are required")
	}
	alphaNumericRegexp := regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	if !alphaNumericRegexp.MatchString(appName) {
		return fmt.Errorf("App name %q must be alphanumeric", appName)
	}
	if !alphaNumericRegexp.MatchString(appVersion) {
		return fmt.Errorf("App version %q must be alphanumeric", appVersion)
	}
	if err := a.uploadArchive(ctx, archivePath, appName, appVersion); err != nil {
		return err
	}

	// Execute the appropriate `agent` command on the machine
	sess, err := a.ssh.NewSession()
	if err != nil {
		return err
	}
	defer sess.Close()
	sess.Stdout = os.Stdout
	sess.Stderr = os.Stderr
	deployCmd := fmt.Sprintf("/home/deploy/.ship/%s/agent deploy --app-name %s --app-version %s",
		a.version, appName, appVersion)
	if volumeNames := cmd.StringSlice("volume"); len(volumeNames) > 0 {
		for _, volumeName := range volumeNames {
			if !alphaNumericRegexp.MatchString(volumeName) {
				return fmt.Errorf("Volume name %q must be alphanumeric", volumeName)
			}
			deployCmd += fmt.Sprintf(" --volume-name %s", volumeName)
		}
	}
	fmt.Printf("Running deploy command: %s\n", deployCmd)
	if err := sess.Run(deployCmd); err != nil {
		return err
	}

	return nil
}

func (a *DeployAction) createArchive() (string, func(), error) {
	tempFile, err := os.CreateTemp("", "ship*.zip")
	if err != nil {
		return "", nil, err
	}
	defer tempFile.Close()

	cleanup := func() {
		if err := os.Remove(tempFile.Name()); err != nil {
			fmt.Printf("Failed to remove temp archive file %q: %v", tempFile.Name(), err)
		}
	}

	zipWriter := zip.NewWriter(tempFile)
	defer zipWriter.Close()

	cwd, err := os.Getwd()
	if err != nil {
		return "", cleanup, err
	}

	walkErr := filepath.Walk(cwd, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		relPath, err := filepath.Rel(cwd, path)
		if err != nil {
			return err
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
			return err
		}
		header.Name = relPath
		if info.IsDir() {
			header.Name += "/"
		}
		header.Method = zip.Deflate
		writer, err := zipWriter.CreateHeader(header)
		if err != nil {
			return err
		}

		// If it's a directory, nothing more to do
		if info.IsDir() {
			return nil
		}

		// Write the file content
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		if _, err := io.Copy(writer, file); err != nil {
			return err
		}

		return nil
	})
	if walkErr != nil {
		return "", cleanup, walkErr
	}

	return tempFile.Name(), cleanup, nil
}

func (a *DeployAction) uploadArchive(
	ctx context.Context, localArchive, appName, appVersion string,
) error {
	// Open the local archive file
	archiveFile, err := os.Open(localArchive)
	if err != nil {
		return err
	}
	defer archiveFile.Close()

	// Make sure the remote directory exists
	remoteAppDir := fmt.Sprintf("/home/deploy/apps/%s/%s", appName, appVersion)
	sess, err := a.ssh.NewSession()
	if err != nil {
		return err
	}
	defer sess.Close()
	sess.Stdout = os.Stdout
	sess.Stderr = os.Stderr
	if err := sess.Run(fmt.Sprintf("mkdir -p %s", remoteAppDir)); err != nil {
		return err
	}

	// Upload the archive to the server
	remoteArchive := fmt.Sprintf("%s/archive.zip", remoteAppDir)
	client, err := scp.NewClientBySSH(a.ssh)
	if err != nil {
		return err
	}
	defer client.Close()
	if err := client.CopyFromFile(ctx, *archiveFile, remoteArchive, "0755"); err != nil {
		return err
	}

	return nil
}
