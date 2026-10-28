package platform

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"
)

// Only this trusted development worker has the Docker socket. Generated code
// receives neither the socket, platform environment, nor provider credentials.
type Docker struct{ NetworkPrefix, GatewayContainer, PostgresContainer string }

type boundedBuffer struct{ bytes.Buffer }

func (b *boundedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	const limit = 128 << 10
	if len(p) > limit {
		p = p[len(p)-limit:]
	}
	if excess := b.Len() + len(p) - limit; excess > 0 {
		b.Next(excess)
	}
	_, _ = b.Buffer.Write(p)
	return n, nil
}

func (d *Docker) command(ctx context.Context, input io.Reader, env []string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Env = append([]string{"PATH=" + os.Getenv("PATH"), "HOME=/tmp"}, env...)
	if host := os.Getenv("DOCKER_HOST"); host != "" {
		cmd.Env = append(cmd.Env, "DOCKER_HOST="+host)
	}
	cmd.Stdin = input
	var output boundedBuffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	err := cmd.Run()
	return output.String(), err
}

func ValidateFiles(files map[string]string) error {
	if len(files) == 0 || len(files) > 120 {
		return errors.New("application must have 1–120 source files")
	}
	total := 0
	for name, content := range files {
		total += len(content)
		if name == "" || len(name) > 200 || path.Clean(name) != name || strings.HasPrefix(name, "/") || strings.Contains(name, "\\") || strings.ContainsAny(name, "\x00\r\n") {
			return errors.New("invalid application file path")
		}
		for _, part := range strings.Split(name, "/") {
			if part == ".." || part == ".git" || strings.HasPrefix(part, ".env") {
				return errors.New("forbidden application file path")
			}
		}
	}
	if total > 2<<20 {
		return errors.New("application source exceeds 2 MiB")
	}
	if files["Dockerfile"] == "" {
		return errors.New("application needs a Dockerfile")
	}
	return nil
}

func (d *Docker) Build(ctx context.Context, image string, files map[string]string) (string, error) {
	if err := ValidateFiles(files); err != nil {
		return "", err
	}
	var archive bytes.Buffer
	tw := tar.NewWriter(&archive)
	for name, content := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0644, Size: int64(len(content))}); err != nil {
			return "", err
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			return "", err
		}
	}
	if err := tw.Close(); err != nil {
		return "", err
	}
	return d.command(ctx, &archive, nil, "build", "--pull=false", "--memory=1g", "--label", "sg1.managed=true", "-t", image, "-")
}

func (d *Docker) PrepareNetwork(ctx context.Context, projectID string) (string, error) {
	network := d.NetworkPrefix + strings.ReplaceAll(projectID, "-", "")
	if _, err := d.command(ctx, nil, nil, "network", "inspect", network); err != nil {
		if _, err := d.command(ctx, nil, nil, "network", "create", "--internal", "--label", "sg1.managed=true", network); err != nil {
			return "", errors.New("could not create project network")
		}
	}
	for _, container := range []string{d.GatewayContainer, d.PostgresContainer} {
		// Already-connected networks are harmless. Inspect after connecting so
		// actual failures cannot be silently ignored.
		_, _ = d.command(ctx, nil, nil, "network", "connect", network, container)
		out, err := d.command(ctx, nil, nil, "inspect", "--format", "{{json .NetworkSettings.Networks}}", container)
		if err != nil || !strings.Contains(out, `"`+network+`"`) {
			return "", errors.New("could not attach trusted project services")
		}
	}
	return network, nil
}

func (d *Docker) Start(ctx context.Context, image, name, network, databaseURL string) error {
	if _, err := d.command(ctx, nil, nil, "container", "inspect", name); err == nil {
		return errors.New("candidate container already exists")
	}
	_, err := d.command(ctx, nil, []string{"DATABASE_URL=" + databaseURL}, "run", "-d", "--name", name, "--network", network, "--restart", "unless-stopped", "--label", "sg1.managed=true", "--memory=512m", "--cpus=1", "--pids-limit=256", "--cap-drop=ALL", "--security-opt=no-new-privileges", "--read-only", "--tmpfs", "/tmp:rw,noexec,nosuid,size=128m", "--env", "DATABASE_URL", "--env", "PORT=8080", image)
	if err != nil {
		return errors.New("candidate container could not start")
	}
	return nil
}

func (d *Docker) Healthy(ctx context.Context, name string) error {
	for range 40 {
		out, err := d.command(ctx, nil, nil, "inspect", "--format", "{{json .State}}", name)
		var state struct {
			Running bool
			Health  struct{ Status string }
		}
		if err == nil && json.Unmarshal([]byte(out), &state) == nil {
			if !state.Running {
				return errors.New("application exited during startup")
			}
			if state.Health.Status == "healthy" {
				return nil
			}
			if state.Health.Status == "" {
				return errors.New("Dockerfile must define HEALTHCHECK")
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return errors.New("application did not become healthy")
}

func (d *Docker) Stop(ctx context.Context, name string) {
	if !strings.HasPrefix(name, "sg1-app-") {
		return
	}
	_, _ = d.command(ctx, nil, nil, "stop", "--time=5", name)
}

func (d *Docker) Remove(ctx context.Context, name string) {
	if !strings.HasPrefix(name, "sg1-app-") {
		return
	}
	_, _ = d.command(ctx, nil, nil, "rm", "-f", name)
}

func containerName(projectID, deploymentID string) string {
	return fmt.Sprintf("sg1-app-%s-%s", projectID, deploymentID[:8])
}
