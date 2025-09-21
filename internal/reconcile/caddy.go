package reconcile

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

//go:embed caddy/compose.yml
var caddyComposeFile string

//go:embed caddy/Caddyfile
var caddyCaddyfileFile string

var _ Reconciler = (*Caddy)(nil)

type Caddy struct{}

func (r *Caddy) Reconcile(ctx context.Context) error {
	// Create the necessary directories
	for _, c := range []struct {
		name string
		args []string
	}{
		{name: "mkdir", args: []string{"-p", "/root/.caddy"}},
		{name: "mkdir", args: []string{"-p", "/root/.caddy/sites-enabled"}},
		{name: "mkdir", args: []string{"-p", "/root/.caddy/data"}},
		{name: "mkdir", args: []string{"-p", "/root/.caddy/config"}},
	} {
		if err := r.execRun(ctx, c.name, c.args...); err != nil {
			return err
		}
	}

	// Figure out the latest Caddy version
	const caddyMajorVersion = "2"
	caddyVersion, err := r.execCapture(ctx, "bash", "-lc",
		`curl -s 'https://hub.docker.com/v2/repositories/library/caddy/tags?page_size=100&ordering=last_updated'`+` | `+
			fmt.Sprintf(`jq -r '.results[].name | select(test("^%s\\.[0-9]+\\.[0-9]+$"))'`, caddyMajorVersion)+` | `+
			`head -n1`,
	)
	if err != nil {
		return err
	}
	caddyVersion = []byte(strings.TrimSpace(string(caddyVersion)))
	caddyVersionRegexp := regexp.MustCompile(`^` + caddyMajorVersion + `\.[0-9]+\.[0-9]+$`)
	if !caddyVersionRegexp.MatchString(string(caddyVersion)) {
		return fmt.Errorf("Failed to determine latest Caddy version, got: %q", string(caddyVersion))
	}
	fmt.Printf("Using Caddy version: %s\n", string(caddyVersion))

	// Create the Caddyfile and Docker Compose file
	if err := os.WriteFile("/root/.caddy/Caddyfile", []byte(caddyCaddyfileFile), 0644); err != nil {
		return err
	}
	composeContents := strings.ReplaceAll(caddyComposeFile, "{{VERSION}}", string(caddyVersion))
	if err := os.WriteFile("/root/.caddy/compose.yml", []byte(composeContents), 0644); err != nil {
		return err
	}

	// Start the Caddy container using Docker Compose
	for _, c := range []struct {
		name string
		args []string
	}{
		{name: "bash", args: []string{"-lc", "if ! docker network inspect caddy >/dev/null 2>&1; then docker network create caddy; fi"}},
		{name: "docker", args: []string{"compose", "pull"}},
		{name: "docker", args: []string{"compose", "up", "-d"}},
		{name: "docker", args: []string{"compose", "exec", "caddy", "caddy", "version"}},
	} {
		if err := r.execRunInDir(ctx, "/root/.caddy", c.name, c.args...); err != nil {
			return err
		}
	}

	return nil
}

func (r *Caddy) execCapture(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.Output()
}

func (r *Caddy) execRun(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (r *Caddy) execRunInDir(ctx context.Context, dir, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
