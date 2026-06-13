package cli

import (
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/kobus-v-schoor/dcx/internal/devcontainer/spec"
	"github.com/kobus-v-schoor/dcx/internal/docker"
	"github.com/kobus-v-schoor/dcx/internal/proxy"
	"github.com/moby/moby/api/types/container"
	"github.com/spf13/cobra"
)

// newExecCmd creates the "exec" subcommand. It opens an interactive shell or
// executes a command inside the running devcontainer, with proxy services
// active if enabled in the config. The proxies inject the host's credentials
// into API requests without exposing them inside the container.
// Added to the root command tree in Execute().
func newExecCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "exec [flags] [-- command [args...]]",
		Short: "Execute a shell or command inside the devcontainer with optional API proxies",
		Long: `Open an interactive shell or execute a command inside the running devcontainer.
When proxy services are enabled in the config, starts local reverse proxies that inject
the host's credentials into API requests for the duration of the session.
Credentials are never exposed inside the container.
If the devcontainer is not running, it is started first.`,
		RunE: runExec,
	}

	return cmd
}

// runExec implements the dcx exec workflow using direct docker exec. Called
// by Cobra when the user runs "dcx exec". Config, log level, and Docker
// daemon reachability are already verified by the root command's
// PersistentPreRunE.
func runExec(cmd *cobra.Command, args []string) error {
	slog.Info("workspace-folder", "path", workspaceFolder)

	// Ensure the devcontainer is running. If it's not, start it via dcx up.
	// This allows "dcx exec" to work as a single command that both starts
	// and connects to the devcontainer.
	if err := ensureDevcontainerRunning(cmd); err != nil {
		return fmt.Errorf("ensuring devcontainer is running: %w", err)
	}

	// Find the devcontainer ID so we can exec into it.
	containerID, err := findContainerID(cmd)
	if err != nil {
		return fmt.Errorf("finding devcontainer: %w", err)
	}

	slog.Info("found devcontainer", "id", shortContainerID(containerID))

	// Set up all enabled proxy services. This starts each proxy, handles TLS
	// certificate injection into the container, and returns the combined remote
	// env vars and a cleanup function.
	var proxyRemoteEnv []string
	proxyResult, err := proxy.SetupAllProxies(cmd.Context(), activeCfg, containerID)
	if err != nil {
		// If proxy setup fails entirely, log a warning and proceed without
		// any proxies — the user gets a shell but without API proxy access.
		slog.Warn("proxy setup failed, proceeding without proxies", "error", err)
	} else {
		proxyRemoteEnv = proxyResult.RemoteEnv
		defer proxyResult.Cleanup()
	}

	// Open an interactive exec session directly via the Docker API.
	cli, err := docker.NewClient(cmd.Context())
	if err != nil {
		return err
	}
	defer func() { _ = cli.Close() }()

	// Load the merged devcontainer spec to resolve remoteUser, workspaceFolder,
	// and remoteEnv. If the workspace has no devcontainer.json and no default
	// image is configured, fall back to empty user and host workspace folder.
	var (
		user     string
		workdir  string
		envVars  []string
		execArgs []string
	)

	specCfg, err := spec.Load(workspaceFolder, activeCfg.DefaultImage)
	if err != nil {
		slog.Warn("failed to load devcontainer spec for exec, using fallbacks", "error", err)
		user = ""
		workdir = workspaceFolder
		envVars = mergeExecEnv(nil, parseProxyEnv(proxyRemoteEnv))
	} else {
		user = specCfg.RemoteUser
		// Features may have injected a remoteUser that is not present in the
		// original devcontainer.json. Inspect the container labels to pick it up.
		if user == "" {
			containerUser, _ := docker.ResolveRemoteUserFromContainer(cmd.Context(), cli, containerID)
			if containerUser != "" {
				user = containerUser
			}
		}
		workdir = specCfg.WorkspaceFolder
		envVars = mergeExecEnv(specCfg.RemoteEnv, parseProxyEnv(proxyRemoteEnv))
	}

	// If the user provided a command after --, append it. Otherwise default
	// to the configured shell for an interactive shell.
	if len(args) > 0 {
		execArgs = args
	} else {
		shell := "bash"
		if activeCfg != nil && activeCfg.DefaultShell != "" {
			shell = activeCfg.DefaultShell
		}
		execArgs = []string{shell}
	}

	return docker.ExecInteractive(cmd.Context(), cli, containerID, user, workdir, envVars, execArgs)
}

// ensureDevcontainerRunning checks whether a devcontainer exists for the
// current workspace. If one is found and is already running, it is a no-op.
// If one is found but is stopped, it is started (or recreated if a stale
// bind mount is detected). If no devcontainer is found, the function runs
// dcx up to create one. This allows "dcx exec" to work as a single command
// that both starts and connects to the devcontainer.
func ensureDevcontainerRunning(cmd *cobra.Command) error {
	cli, err := docker.NewClient(cmd.Context())
	if err != nil {
		return err
	}
	defer func() { _ = cli.Close() }()

	containers, err := docker.FindDevcontainers(cmd.Context(), cli, workspaceFolder)
	if err != nil {
		return fmt.Errorf("checking for devcontainer: %w", err)
	}

	if len(containers.Items) > 0 {
		if containers.Items[0].State == container.StateRunning {
			slog.Info("devcontainer already running")
			return nil
		}

		slog.Info("devcontainer exists but is not running, starting")
		if err := runUp(cmd.Context(), false, nil); err != nil {
			return fmt.Errorf("running dcx up: %w", err)
		}
		return nil
	}

	// No devcontainer found — start one.
	slog.Info("no devcontainer found, starting one with dcx up")

	if err := runUp(cmd.Context(), false, nil); err != nil {
		return fmt.Errorf("running dcx up: %w", err)
	}

	return nil
}

// findContainerID locates the running devcontainer for the current workspace
// and returns its container ID. Returns an error if no devcontainer is found.
// Called by dcx exec to determine which container to exec into.
func findContainerID(cmd *cobra.Command) (string, error) {
	cli, err := docker.NewClient(cmd.Context())
	if err != nil {
		return "", err
	}
	defer func() { _ = cli.Close() }()

	containers, err := docker.FindDevcontainers(cmd.Context(), cli, workspaceFolder)
	if err != nil {
		return "", fmt.Errorf("finding devcontainers: %w", err)
	}

	if len(containers.Items) == 0 {
		return "", fmt.Errorf("no devcontainer found for %s", workspaceFolder)
	}

	return containers.Items[0].ID, nil
}

// parseProxyEnv converts the proxy's RemoteEnv slice from devcontainer CLI
// flag format ("--remote-env=KEY=VALUE") into a map of KEY → VALUE.
func parseProxyEnv(remoteEnv []string) map[string]string {
	result := make(map[string]string, len(remoteEnv))
	const prefix = "--remote-env="
	for _, e := range remoteEnv {
		e = strings.TrimPrefix(e, prefix)
		if idx := strings.Index(e, "="); idx >= 0 {
			result[e[:idx]] = e[idx+1:]
		}
	}
	return result
}

// mergeExecEnv merges config remoteEnv and proxy env vars into a single
// sorted KEY=VALUE slice. Proxy values take precedence on key conflict.
func mergeExecEnv(remoteEnv, proxyEnv map[string]string) []string {
	merged := make(map[string]string, len(remoteEnv)+len(proxyEnv))
	for k, v := range remoteEnv {
		merged[k] = v
	}
	for k, v := range proxyEnv {
		merged[k] = v
	}

	keys := make([]string, 0, len(merged))
	for k := range merged {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	result := make([]string, len(keys))
	for i, k := range keys {
		result[i] = k + "=" + merged[k]
	}
	return result
}

// shortContainerID returns the first 12 characters of a container ID for
// human-readable log output, matching the Docker CLI convention.
func shortContainerID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}
