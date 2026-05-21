package cli

import (
	"fmt"
	"log/slog"

	"github.com/kobus-v-schoor/dcx/internal/docker"
	"github.com/kobus-v-schoor/dcx/internal/proxy"
	"github.com/kobus-v-schoor/dcx/internal/runner"
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

// runExec implements the dcx exec workflow. Called by Cobra when the user
// runs "dcx exec". Config, log level, and Docker daemon reachability are
// already verified by the root command's PersistentPreRunE.
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

	// Set up all enabled proxy services (GitHub, OpenAI, etc.). This starts
	// each proxy, handles TLS certificate injection into the container, and
	// returns the combined remote env vars and a cleanup function. The exec
	// command only needs the env vars to construct the devcontainer flags
	// and the cleanup function to shut down proxies when the session ends.
	var remoteEnv []string
	proxyResult, err := proxy.SetupAllProxies(cmd.Context(), activeCfg, containerID)
	if err != nil {
		// If proxy setup fails entirely, log a warning and proceed without
		// any proxies — the user gets a shell but without API proxy access.
		slog.Warn("proxy setup failed, proceeding without proxies", "error", err)
	} else {
		remoteEnv = proxyResult.RemoteEnv
		defer proxyResult.Cleanup()
	}

	// Build and execute the devcontainer exec command. This opens an
	// interactive shell (or runs the specified command) inside the container.
	devcontainerPath, err := runner.Find()
	if err != nil {
		return err
	}

	execArgs := buildExecArgs(containerID, remoteEnv, args)

	slog.Debug("invoking devcontainer exec", "args", execArgs)

	return runner.Run(devcontainerPath, execArgs)
}

// ensureDevcontainerRunning checks whether a devcontainer exists for the
// current workspace. If one is found (running or stopped), it is a no-op —
// the devcontainer exec command will handle starting a stopped container. If
// no devcontainer is found, the function runs dcx up to create one. This
// allows "dcx exec" to work as a single command that both starts and connects
// to the devcontainer.
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
		slog.Info("devcontainer already exists")
		return nil
	}

	// No devcontainer found — start one.
	slog.Info("no devcontainer found, starting one with dcx up")

	if err := runUp(false, nil); err != nil {
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

// buildExecArgs assembles the arguments for devcontainer exec. It includes
// the container ID, workspace folder, and remote env vars for the proxies.
// If no command is specified, it defaults to bash for an interactive shell.
func buildExecArgs(containerID string, remoteEnv []string, userArgs []string) []string {
	args := []string{"exec"}

	args = append(args, "--workspace-folder", workspaceFolder)

	// Add the container ID so devcontainer exec knows which container to
	// target. The devcontainer CLI uses this to find the running container.
	args = append(args, "--container-id", containerID)

	// Add remote env vars for the proxy services (GH_HOST,
	// NODE_EXTRA_CA_CERTS, GIT_CONFIG_*, etc.). These are only present when
	// at least one proxy is enabled and set up successfully.
	args = append(args, remoteEnv...)

	// If the user provided a command after --, append it. Otherwise default
	// to bash for an interactive shell.
	if len(userArgs) > 0 {
		args = append(args, userArgs...)
	} else {
		args = append(args, "bash")
	}

	return args
}

// shortContainerID returns the first 12 characters of a container ID for
// human-readable log output, matching the Docker CLI convention.
func shortContainerID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}
