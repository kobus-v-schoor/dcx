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

	"github.com/kobus-v-schoor/dcx/internal/devcontainer/features"
	"github.com/kobus-v-schoor/dcx/internal/devcontainer/spec"
	"github.com/kobus-v-schoor/dcx/internal/docker"
	"gopkg.in/yaml.v3"
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

// composeConfigRunner is used by resolveComposeBaseImage to run
// docker compose config. In production it is runComposeConfigCLI;
// tests may override it.
var composeConfigRunner = runComposeConfigCLI

// composeFeatureImageBuilder is used by UpCompose to build the
// feature-augmented image for compose projects. In production it is
// features.BuildFeatureImage; tests may override it.
var composeFeatureImageBuilder = features.BuildFeatureImage

// composeDockerfileBuilder is used by resolveComposeBaseImage to
// build a compose service that uses build: without an explicit
// image. In production it is buildFromDockerfile; tests may
// override it.
var composeDockerfileBuilder = buildFromDockerfile

// UpCompose creates or reuses a devcontainer managed by Docker Compose.
// It substitutes variables, generates a temporary compose override file,
// brings the target service up, and runs postCreateCommand for newly
// created containers. When rebuild is false and a running container already
// exists, it returns the existing container ID. Otherwise, docker compose up
// is allowed to start or recreate the container as needed.
func UpCompose(ctx context.Context, cli docker.DockerClient, cfg *spec.Config, hostWorkspaceFolder string, rebuild, upgradeLockfile bool) (string, error) {
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

	// Determine the base image and build a feature-augmented image
	// when features are configured.
	var featureImageRef string
	if cfg.HasFeatures() {
		baseImageRef, err := resolveComposeBaseImage(ctx, cli, projectName, composeFiles, cfg.Service, absHostFolder, rebuild)
		if err != nil {
			return "", fmt.Errorf("resolving compose base image: %w", err)
		}
		slog.Info("resolved compose base image", "ref", baseImageRef)

		featureImageRef, err = composeFeatureImageBuilder(ctx, cli, baseImageRef, cfg.Features, cfg.ContainerUser, cfg.RemoteUser, absHostFolder, rebuild, upgradeLockfile)
		if err != nil {
			return "", fmt.Errorf("building feature image: %w", err)
		}
		slog.Info("resolved feature image", "ref", featureImageRef)
	}

	// Phase 1: discover existing devcontainers by workspace label.
	existing, err := docker.FindDevcontainers(ctx, cli, absHostFolder)
	if err != nil {
		return "", fmt.Errorf("finding existing containers: %w", err)
	}

	if len(existing.Items) > 0 && !rebuild && docker.IsContainerRunning(existing.Items[0]) {
		slog.Info("reusing running devcontainer", "id", docker.ShortID(existing.Items[0].ID))
		return existing.Items[0].ID, nil
	}

	// Phase 2: create or recreate the container with a temporary override.
	tempDir, err := os.MkdirTemp("", "dcx-compose-")
	if err != nil {
		return "", fmt.Errorf("creating temp directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	overridePath := filepath.Join(tempDir, "dcx.compose.override.yml")
	if err := writeComposeOverride(cfg, overridePath, absHostFolder, featureImageRef); err != nil {
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
// overrideFile may be empty. recreatePolicy is either "force" (adds
// --force-recreate) or any other value (no recreate flag).
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
	if recreatePolicy == "force" {
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

// composeOverrideFile represents a minimal Docker Compose override file for a
// single service. It is serialised to YAML by writeComposeOverride.
type composeOverrideFile struct {
	Services map[string]composeServiceOverride `yaml:"services"`
}

// composeServiceOverride captures the dcx-injected labels, environment,
// volumes, and entrypoint for a compose service.
type composeServiceOverride struct {
	Image       string          `yaml:"image,omitempty"`
	Labels      []string        `yaml:"labels,omitempty"`
	Environment []string        `yaml:"environment,omitempty"`
	Volumes     []composeVolume `yaml:"volumes,omitempty"`
	Entrypoint  []string        `yaml:"entrypoint,omitempty"`
}

// composeVolume mirrors the Docker Compose long-form volume mount syntax.
type composeVolume struct {
	Type        string    `yaml:"type"`
	Source      string    `yaml:"source"`
	Target      string    `yaml:"target"`
	ReadOnly    bool      `yaml:"read_only,omitempty"`
	Consistency string    `yaml:"consistency,omitempty"`
	Bind        *bindOpts `yaml:"bind,omitempty"`
}

type bindOpts struct {
	Propagation string `yaml:"propagation,omitempty"`
}

// writeComposeOverride generates a temporary Docker Compose override file
// that injects devcontainer labels, environment variables, additional
// mounts, and the keep-alive entrypoint into the target service. When
// imageOverride is non-empty it also overrides the service image so that
// docker compose up uses the feature-augmented image instead of the
// original base image.
func writeComposeOverride(cfg *spec.Config, path, absHostFolder, imageOverride string) error {
	svc := composeServiceOverride{}
	if imageOverride != "" {
		svc.Image = imageOverride
	}

	// Labels
	labels := buildComposeLabels(cfg, absHostFolder)
	if len(labels) > 0 {
		svc.Labels = labels
	}

	// Environment variables sorted for determinism.
	if len(cfg.ContainerEnv) > 0 {
		keys := make([]string, 0, len(cfg.ContainerEnv))
		for k := range cfg.ContainerEnv {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			svc.Environment = append(svc.Environment, k+"="+cfg.ContainerEnv[k])
		}
	}

	// Volumes (mounts) converted to Compose long-form syntax.
	if len(cfg.Mounts) > 0 {
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
				svc.Volumes = append(svc.Volumes, vol)
			}
		}
	}

	// Entrypoint: always injected for compose projects so the container
	// stays alive for dcx exec regardless of overrideCommand setting.
	svc.Entrypoint = []string{"/bin/sh", "-c", composeKeepAliveScript, "-"}

	override := composeOverrideFile{
		Services: map[string]composeServiceOverride{
			cfg.Service: svc,
		},
	}

	data, err := yaml.Marshal(override)
	if err != nil {
		return fmt.Errorf("marshalling compose override: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
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
// composeVolume struct. Supported options are mapped to their Compose
// equivalents; unsupported options are skipped with a warning.
func mountEntryToComposeVolume(mount string) (composeVolume, error) {
	wm, err := spec.ParseMountString(mount)
	if err != nil {
		return composeVolume{}, err
	}

	vol := composeVolume{
		Type:   wm.Type,
		Source: wm.Source,
		Target: wm.Target,
	}

	for _, opt := range wm.Options {
		switch {
		case opt == "readonly":
			vol.ReadOnly = true
		case strings.HasPrefix(opt, "consistency="):
			vol.Consistency = strings.TrimPrefix(opt, "consistency=")
		case strings.HasPrefix(opt, "bind-propagation="):
			if vol.Bind == nil {
				vol.Bind = &bindOpts{}
			}
			vol.Bind.Propagation = strings.TrimPrefix(opt, "bind-propagation=")
		default:
			slog.Warn("unsupported mount option for compose, skipping", "option", opt)
		}
	}

	return vol, nil
}

// composeConfigJSON represents the output of docker compose config --format json.
type composeConfigJSON struct {
	Services map[string]composeServiceJSON `json:"services"`
}

// composeServiceJSON is the resolved service definition from docker compose config.
type composeServiceJSON struct {
	Image string            `json:"image"`
	Build *composeBuildJSON `json:"build"`
}

// composeBuildJSON is the resolved build block from docker compose config.
type composeBuildJSON struct {
	Context    string            `json:"context"`
	Dockerfile string            `json:"dockerfile"`
	Args       map[string]string `json:"args"`
	Target     string            `json:"target"`
}

// resolveComposeBaseImage determines the base image reference for the
// target compose service. It runs docker compose config to get the fully
// resolved configuration, then extracts the service's image or build
// properties. When the service declares build: without an explicit image,
// the Dockerfile is built using the existing buildFromDockerfile machinery
// and the resulting stable tag is returned.
func resolveComposeBaseImage(ctx context.Context, cli docker.DockerClient, projectName string, composeFiles []string, serviceName, workspaceFolder string, forceRebuild bool) (string, error) {
	out, err := composeConfigRunner(ctx, projectName, composeFiles)
	if err != nil {
		return "", err
	}

	var doc composeConfigJSON
	if err := json.Unmarshal(out, &doc); err != nil {
		return "", fmt.Errorf("parsing compose config JSON: %w", err)
	}

	svc, ok := doc.Services[serviceName]
	if !ok {
		return "", fmt.Errorf("service %q not found in compose configuration", serviceName)
	}

	// If the service explicitly specifies an image, use it directly.
	if svc.Image != "" {
		if err := docker.ImagePullIfMissing(ctx, cli, svc.Image, false); err != nil {
			return "", fmt.Errorf("pulling compose service image %s: %w", svc.Image, err)
		}
		return svc.Image, nil
	}

	// If the service specifies build without an explicit image, build the
	// Dockerfile to produce a base image.
	if svc.Build != nil {
		tempCfg := &spec.Config{
			Build: &spec.Build{
				Context:    svc.Build.Context,
				Dockerfile: svc.Build.Dockerfile,
				Args:       svc.Build.Args,
				Target:     svc.Build.Target,
			},
		}
		if tempCfg.Build.Dockerfile == "" {
			tempCfg.Build.Dockerfile = "Dockerfile"
		}
		tag, err := composeDockerfileBuilder(ctx, cli, tempCfg, workspaceFolder, forceRebuild)
		if err != nil {
			return "", fmt.Errorf("building compose service Dockerfile: %w", err)
		}
		return tag, nil
	}

	return "", fmt.Errorf("compose service %q does not specify image or build", serviceName)
}

// runComposeConfigCLI runs docker compose config --format json and returns
// the JSON output. It merges all compose files and resolves interpolation.
func runComposeConfigCLI(ctx context.Context, projectName string, composeFiles []string) ([]byte, error) {
	var args []string
	args = append(args, "compose", "-p", projectName)
	for _, f := range composeFiles {
		args = append(args, "-f", f)
	}
	args = append(args, "config", "--format", "json")
	cmd := exec.CommandContext(ctx, "docker", args...)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && len(exitErr.Stderr) > 0 {
			return nil, fmt.Errorf("docker compose config failed: %s", string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("docker compose config failed: %w", err)
	}
	return out, nil
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
