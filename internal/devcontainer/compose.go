package devcontainer

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kobus-v-schoor/dcx/internal/devcontainer/spec"
	"github.com/kobus-v-schoor/dcx/internal/docker"
)

// composeKeepAliveScript is the shell script used to keep a compose service
// container alive. It is identical to keepAliveScript but with $$ in place
// of $ for $@ and $! so that Docker Compose variable interpolation does not
// treat them as unset environment variables.
const composeKeepAliveScript = `echo Container started
trap "exit 0" 15

exec "$$@"
while sleep 1 & wait $$!; do :; done`

// composeUpRunner is the function used by UpCompose to invoke
// docker compose up. In production it is runComposeUpCLI; tests may
// override it to capture constructed arguments without invoking Docker.
var composeUpRunner = runComposeUpCLI

// UpCompose creates or reuses a devcontainer managed by Docker Compose.
// It substitutes variables, generates a temporary compose override file,
// brings the target service up, and runs postCreateCommand for newly
// created containers. When rebuild is false and a running container already
// exists, it returns the existing container ID. When a stopped container
// exists and rebuild is false, it is started with --no-recreate. When
// rebuild is true or no container exists, the container is created or
// recreated.
func UpCompose(ctx context.Context, cli docker.DockerClient, cfg *spec.Config, hostWorkspaceFolder string, rebuild bool) (string, error) {
	absHostFolder, err := filepath.Abs(hostWorkspaceFolder)
	if err != nil {
		return "", fmt.Errorf("resolving workspace folder: %w", err)
	}

	// Substitute variables in the config so that compose paths, env vars,
	// and mounts use resolved values before generating the override file.
	if err := SubstituteAll(cfg, absHostFolder); err != nil {
		return "", fmt.Errorf("substituting variables: %w", err)
	}

	// Resolve compose file absolute paths and derive the project name.
	composeFiles := resolveComposeFilePaths(cfg, absHostFolder)
	if len(composeFiles) == 0 {
		return "", fmt.Errorf("no docker compose files configured")
	}
	projectName := resolveProjectName(absHostFolder)

	// Phase 1: discover existing devcontainers by workspace label.
	existing, err := docker.FindDevcontainers(ctx, cli, absHostFolder)
	if err != nil {
		return "", fmt.Errorf("finding existing containers: %w", err)
	}

	if len(existing.Items) > 0 && !rebuild {
		ctr := existing.Items[0]
		if docker.IsContainerRunning(ctr) {
			slog.Info("reusing running devcontainer", "id", docker.ShortID(ctr.ID))
			return ctr.ID, nil
		}

		// Stopped container: start without recreation so that config
		// changes do not force an unwanted recreate.
		slog.Info("starting stopped devcontainer", "id", docker.ShortID(ctr.ID))
		args := buildComposeUpArgs(projectName, composeFiles, "", "no", cfg.Service, cfg.RunServices)
		if err := composeUpRunner(ctx, args); err != nil {
			return "", fmt.Errorf("starting compose project: %w", err)
		}

		updated, err := docker.FindDevcontainers(ctx, cli, absHostFolder)
		if err != nil {
			return "", fmt.Errorf("finding container after compose up: %w", err)
		}
		if len(updated.Items) == 0 {
			return "", fmt.Errorf("devcontainer container not found after compose up")
		}
		slog.Info("started container", "id", docker.ShortID(updated.Items[0].ID))
		return updated.Items[0].ID, nil
	}

	// Phase 2: create or recreate the container with a temporary override.
	tempDir, err := os.MkdirTemp("", "dcx-compose-")
	if err != nil {
		return "", fmt.Errorf("creating temp directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	overridePath := filepath.Join(tempDir, "dcx.compose.override.yml")
	if err := writeComposeOverride(cfg, overridePath, absHostFolder); err != nil {
		return "", fmt.Errorf("writing compose override: %w", err)
	}

	recreatePolicy := ""
	if rebuild {
		recreatePolicy = "force"
	}
	args := buildComposeUpArgs(projectName, composeFiles, overridePath, recreatePolicy, cfg.Service, cfg.RunServices)
	if err := composeUpRunner(ctx, args); err != nil {
		return "", fmt.Errorf("running compose up: %w", err)
	}

	// Resolve the container ID from labels after compose up.
	updated, err := docker.FindDevcontainers(ctx, cli, absHostFolder)
	if err != nil {
		return "", fmt.Errorf("finding container after compose up: %w", err)
	}
	if len(updated.Items) == 0 {
		return "", fmt.Errorf("devcontainer container not found after compose up")
	}
	containerID := updated.Items[0].ID

	if rebuild {
		slog.Info("rebuilt devcontainer", "id", docker.ShortID(containerID))
	} else {
		slog.Info("created devcontainer", "id", docker.ShortID(containerID))
	}

	// Run post-create command for newly created or recreated containers.
	postCreateRunner(ctx, containerID, cfg)

	return containerID, nil
}

// buildComposeUpArgs constructs the full argument slice for
// docker compose up. composeFiles must contain at least one path.
// overrideFile may be empty. recreatePolicy is one of:
//
//	""     → no recreate flag
//	"no"   → --no-recreate
//	"force"→ --force-recreate
//
// When runServices is empty, no service names are appended so that all
// services in the compose file are started. When runServices is non-empty,
// the target service and all runServices are appended explicitly.
func buildComposeUpArgs(projectName string, composeFiles []string, overrideFile string, recreatePolicy string, service string, runServices []string) []string {
	var args []string
	args = append(args, "compose")
	args = append(args, "-p", projectName)
	for _, f := range composeFiles {
		args = append(args, "-f", f)
	}
	if overrideFile != "" {
		args = append(args, "-f", overrideFile)
	}
	args = append(args, "up", "-d")
	switch recreatePolicy {
	case "no":
		args = append(args, "--no-recreate")
	case "force":
		args = append(args, "--force-recreate")
	}
	if len(runServices) > 0 {
		args = append(args, service)
		args = append(args, runServices...)
	}
	return args
}

// resolveComposeFilePaths resolves dockerComposeFile entries into absolute
// paths. Absolute paths are returned as-is. Relative paths are resolved
// against the .devcontainer directory inside the workspace folder.
func resolveComposeFilePaths(cfg *spec.Config, absHostFolder string) []string {
	files := cfg.EffectiveDockerComposeFiles()
	if len(files) == 0 {
		return nil
	}
	devcontainerDir := filepath.Join(absHostFolder, ".devcontainer")
	resolved := make([]string, len(files))
	for i, f := range files {
		if filepath.IsAbs(f) {
			resolved[i] = f
		} else {
			resolved[i] = filepath.Join(devcontainerDir, f)
		}
	}
	return resolved
}

// resolveProjectName derives the compose project name from the workspace
// folder basename. This matches the devcontainer CLI behavior, which
// ignores spec.Name for the compose project name.
func resolveProjectName(absHostFolder string) string {
	return filepath.Base(absHostFolder)
}

// writeComposeOverride generates a temporary Docker Compose override file
// that injects devcontainer labels, environment variables, additional
// mounts, and the keep-alive entrypoint into the target service.
func writeComposeOverride(cfg *spec.Config, path, absHostFolder string) error {
	var b strings.Builder

	b.WriteString("services:\n")
	b.WriteString(fmt.Sprintf("  %s:\n", cfg.Service))

	// Labels
	labels := buildComposeLabels(cfg, absHostFolder)
	if len(labels) > 0 {
		b.WriteString("    labels:\n")
		for _, l := range labels {
			b.WriteString(fmt.Sprintf("      - %q\n", l))
		}
	}

	// Environment variables sorted for determinism.
	if len(cfg.ContainerEnv) > 0 {
		b.WriteString("    environment:\n")
		keys := make([]string, 0, len(cfg.ContainerEnv))
		for k := range cfg.ContainerEnv {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			b.WriteString(fmt.Sprintf("      - %q\n", k+"="+cfg.ContainerEnv[k]))
		}
	}

	// Volumes (mounts) converted to Compose long-form syntax.
	if len(cfg.Mounts) > 0 {
		b.WriteString("    volumes:\n")
		for _, m := range cfg.Mounts {
			if m.IsEmpty() {
				continue
			}
			if s, ok := m.AsString(); ok {
				vol, err := mountEntryToComposeVolume(s)
				if err != nil {
					slog.Warn("skipping invalid mount entry", "error", err, "mount", s)
					continue
				}
				lines := strings.Split(vol, "\n")
				if len(lines) > 0 {
					b.WriteString("      - ")
					b.WriteString(lines[0])
					b.WriteString("\n")
					for _, line := range lines[1:] {
						b.WriteString("        ")
						b.WriteString(line)
						b.WriteString("\n")
					}
				}
			}
		}
	}

	// Entrypoint: always injected for compose projects so the container
	// stays alive for dcx exec regardless of overrideCommand setting.
	b.WriteString("    entrypoint:\n")
	b.WriteString("      - \"/bin/sh\"\n")
	b.WriteString("      - \"-c\"\n")
	b.WriteString(fmt.Sprintf("      - %q\n", composeKeepAliveScript))
	b.WriteString("      - \"-\"\n")

	content := b.String()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("writing override file: %w", err)
	}
	return nil
}

// buildComposeLabels returns the devcontainer labels that should be applied
// to the compose service, including the workspace folder, config file path,
// metadata, and dcx.managed marker. The labels are sorted deterministically.
func buildComposeLabels(cfg *spec.Config, absHostFolder string) []string {
	labels := []string{
		docker.DevcontainerLabel + "=" + absHostFolder,
		"devcontainer.config_file=" + filepath.Join(absHostFolder, ".devcontainer", "devcontainer.json"),
		"dcx.managed=true",
	}
	metadataJSON, err := buildComposeMetadataJSON(cfg)
	if err == nil && metadataJSON != "" {
		labels = append(labels, "devcontainer.metadata="+metadataJSON)
	}
	sort.Strings(labels)
	return labels
}

// buildComposeMetadataJSON constructs the devcontainer.metadata label value
// as a JSON array containing a single object with the config properties that
// the devcontainer CLI stores. For compose projects overrideCommand is
// always true because the keep-alive entrypoint is always injected.
func buildComposeMetadataJSON(cfg *spec.Config) (string, error) {
	configMeta := make(map[string]any)
	if cfg.RemoteUser != "" {
		configMeta["remoteUser"] = cfg.RemoteUser
	}
	if cfg.ContainerUser != "" {
		configMeta["containerUser"] = cfg.ContainerUser
	}
	if len(cfg.ContainerEnv) > 0 {
		configMeta["containerEnv"] = cfg.ContainerEnv
	}
	if cfg.WorkspaceFolder != "" {
		configMeta["workspaceFolder"] = cfg.WorkspaceFolder
	}
	if len(cfg.Mounts) > 0 {
		configMeta["mounts"] = cfg.Mounts
	}
	configMeta["overrideCommand"] = true
	if len(cfg.ForwardPorts) > 0 {
		configMeta["forwardPorts"] = cfg.ForwardPorts
	}
	if cfg.ShutdownAction != "" {
		configMeta["shutdownAction"] = cfg.ShutdownAction
	}
	if cfg.UpdateRemoteUserUID != nil {
		configMeta["updateRemoteUserUID"] = *cfg.UpdateRemoteUserUID
	}

	meta := []map[string]any{configMeta}
	b, err := json.Marshal(meta)
	if err != nil {
		return "", fmt.Errorf("marshalling metadata: %w", err)
	}
	return string(b), nil
}

// mountEntryToComposeVolume converts a Docker --mount format string into a
// Compose long-form volume YAML entry (without the leading list marker).
// Supported options are mapped to their Compose equivalents; unsupported
// options are skipped with a warning.
func mountEntryToComposeVolume(mount string) (string, error) {
	wm, err := spec.ParseMountString(mount)
	if err != nil {
		return "", err
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("type: %s", wm.Type))
	lines = append(lines, fmt.Sprintf("source: %s", wm.Source))
	lines = append(lines, fmt.Sprintf("target: %s", wm.Target))

	for _, opt := range wm.Options {
		switch {
		case opt == "readonly":
			lines = append(lines, "read_only: true")
		case strings.HasPrefix(opt, "consistency="):
			lines = append(lines, fmt.Sprintf("consistency: %s", strings.TrimPrefix(opt, "consistency=")))
		case strings.HasPrefix(opt, "bind-propagation="):
			lines = append(lines, "bind:")
			lines = append(lines, fmt.Sprintf("  propagation: %s", strings.TrimPrefix(opt, "bind-propagation=")))
		default:
			slog.Warn("unsupported mount option for compose, skipping", "option", opt)
		}
	}

	return strings.Join(lines, "\n"), nil
}

// runComposeUpCLI invokes docker compose with the given arguments via
// exec.CommandContext, streaming stdout and stderr to the user. Returns an
// error if the command exits with a non-zero code.
func runComposeUpCLI(ctx context.Context, args []string) error {
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker compose up failed: %w", err)
	}
	return nil
}
