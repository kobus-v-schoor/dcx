package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/kobus-v-schoor/dcx/internal/docker"
	"github.com/kobus-v-schoor/dcx/internal/ghproxy"
	"github.com/kobus-v-schoor/dcx/internal/runner"
	"github.com/spf13/cobra"
)

// newExecCmd creates the "exec" subcommand. It opens an interactive shell or
// executes a command inside the running devcontainer, with the GitHub API
// proxy active if enabled in the config. The proxy enforces repository-level
// scoping on the user's GitHub token for the duration of the shell session.
// Added to the root command tree in Execute().
func newExecCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "exec [flags] [-- command [args...]]",
		Short: "Execute a shell or command inside the devcontainer with optional GitHub API proxy",
		Long: `Open an interactive shell or execute a command inside the running devcontainer.
When github_cli is enabled in the config, starts a GitHub API proxy that enforces
repository-level scoping on the user's GitHub token for the duration of the session.
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

	// Set up the GitHub API proxy if enabled in the config. The proxy
	// intercepts all gh CLI requests from the container and enforces
	// repository-level scoping. If no GitHub token is available on the
	// host, a warning is logged and the container runs without proxy.
	var proxy *ghproxy.Proxy
	var caCertPath string
	var remoteEnv []string

	if activeCfg.GitHubCLI.Enabled {
		var err error
		proxy, caCertPath, remoteEnv, err = setupProxy(cmd, containerID)
		if err != nil {
			// If proxy setup fails, log a warning and proceed without it —
			// the user gets a shell but without repo-scoped GitHub access.
			slog.Warn("GitHub API proxy setup failed, proceeding without proxy", "error", err)
			proxy = nil
		} else {
			defer func() {
				proxy.Shutdown()
				// Clean up the temporary CA cert file.
				if caCertPath != "" {
					_ = os.Remove(caCertPath)
				}
			}()
		}
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

// setupProxy initializes and starts the GitHub API reverse proxy. It detects
// the host's GitHub token and the repository from the git remote, creates the
// proxy, writes the CA cert to a temp file, copies it into the container via
// the Docker API, and returns the proxy instance, CA cert file path, and the
// remote env vars to inject into the container. Returns an error if no token
// is available or the proxy fails to start. The caller is responsible for
// calling proxy.Shutdown() and cleaning up the CA cert file.
func setupProxy(cmd *cobra.Command, containerID string) (*ghproxy.Proxy, string, []string, error) {
	// Detect the host's GitHub token.
	token, ok := ghproxy.DetectToken()
	if !ok {
		return nil, "", nil, fmt.Errorf("no GitHub token available on host")
	}

	// Determine the allowed repository. Use the config value if set, otherwise
	// auto-detect from the git remote.
	repository := activeCfg.GitHubCLI.Repository
	if repository == "" {
		detected, ok := ghproxy.DetectRepository(workspaceFolder)
		if !ok {
			return nil, "", nil, fmt.Errorf("cannot detect repository from git remote and no repository configured")
		}
		repository = detected
	}

	// Get the Docker client early — we need it to detect the gateway IP
	// before starting the proxy, since the gateway IP is included in the
	// TLS certificate's IP SANs.
	dockerCLI, err := docker.NewClient(cmd.Context())
	if err != nil {
		return nil, "", nil, fmt.Errorf("creating Docker client: %w", err)
	}
	defer dockerCLI.Close()

	// Determine the host IP that the container can reach. The proxy listens
	// on 0.0.0.0 so it is reachable from the container, but the container
	// needs the correct IP to connect. We inspect the container's network
	// to find the gateway IP, which is the host's IP on the Docker bridge
	// network. This is more reliable than host.docker.internal, which may
	// not be routable in all environments (e.g. codespaces).
	gatewayIP, err := docker.GatewayIP(cmd.Context(), dockerCLI, containerID)
	if err != nil {
		return nil, "", nil, fmt.Errorf("detecting host gateway IP: %w", err)
	}

	// Create and start the proxy. The gateway IP is included in the TLS
	// certificate's IP SANs so the container can verify the connection.
	proxy := ghproxy.New(token, repository, gatewayIP)
	port, err := proxy.Start()
	if err != nil {
		return nil, "", nil, fmt.Errorf("starting GitHub API proxy: %w", err)
	}

	// Write the CA certificate to a temp file on the host, then copy it into
	// the container via the Docker API. The devcontainer exec command does not
	// support --mount flags, so we use the Docker API's CopyToContainer to
	// place the CA cert file directly into the container filesystem.
	caCertPath, err := writeCACert(proxy.CACertPEM())
	if err != nil {
		proxy.Shutdown()
		return nil, "", nil, fmt.Errorf("writing CA certificate: %w", err)
	}

	// Create the target directory inside the container and copy the CA cert.
	// The directory must exist before CopyToContainer can place files into it.
	if err := docker.MkdirInContainer(cmd.Context(), dockerCLI, containerID, "/opt/dcx/gh-proxy"); err != nil {
		proxy.Shutdown()
		_ = os.Remove(caCertPath)
		return nil, "", nil, fmt.Errorf("creating CA cert directory in container: %w", err)
	}

	if err := docker.CopyBytesToContainer(
		cmd.Context(),
		dockerCLI,
		containerID,
		"ca.crt",
		proxy.CACertPEM(),
		"/opt/dcx/gh-proxy",
	); err != nil {
		// Log the error and proceed — the gh CLI may still work without
		// the CA cert if the proxy is not used for HTTPS verification.
		slog.Warn("could not copy CA cert into container, gh CLI may not trust the proxy", "error", err)
	}

	// Build the remote env vars that configure the gh CLI and git inside
	// the container to route through the proxy.
	remoteEnv := buildProxyRemoteEnv(port, gatewayIP)

	return proxy, caCertPath, remoteEnv, nil
}

// writeCACert writes the PEM-encoded CA certificate to a temporary file on
// the host. The file is used as an intermediate step before copying the cert
// into the container so the gh CLI trusts the proxy's self-signed TLS
// certificate via SSL_CERT_FILE and NODE_EXTRA_CA_CERTS.
// Returns the path to the temp file. The caller should clean up the file when
// the proxy is shut down.
func writeCACert(caCertPEM []byte) (string, error) {
	tmp, err := os.CreateTemp("", "dcx-gh-proxy-ca-*.crt")
	if err != nil {
		return "", fmt.Errorf("creating temp file for CA cert: %w", err)
	}
	defer tmp.Close()

	if _, err := tmp.Write(caCertPEM); err != nil {
		_ = os.Remove(tmp.Name())
		return "", fmt.Errorf("writing CA cert: %w", err)
	}

	return tmp.Name(), nil
}

// buildProxyRemoteEnv constructs the environment variable flags that
// configure the gh CLI and git inside the container to route all GitHub API
// requests through the proxy. These are passed as --remote-env flags to
// devcontainer exec. Returns a slice of "--remote-env=NAME=VALUE" strings.
//
// The gatewayIP is the host's IP address on the Docker bridge network,
// which the container uses to reach the proxy. The proxyPort is the port
// the proxy is listening on.
func buildProxyRemoteEnv(proxyPort int, gatewayIP string) []string {
	var envVars []string

	// GH_HOST tells the gh CLI which GitHub host to target. The gh CLI
	// constructs the API URL as https://GH_HOST/api/v3/... so including
	// the port directs it to the proxy. Using the gateway IP (not
	// host.docker.internal) ensures connectivity in all environments.
	ghHost := fmt.Sprintf("%s:%d", gatewayIP, proxyPort)
	envVars = append(envVars, fmt.Sprintf("--remote-env=GH_HOST=%s", ghHost))

	// SSL_CERT_FILE tells Go-based programs (like the gh CLI binary) to trust
	// the proxy's self-signed CA certificate. The CA cert is copied into the
	// container at the fixed path CACertMountPath via the Docker API.
	// NODE_EXTRA_CA_CERTS is also set for Node.js-based tools.
	envVars = append(envVars, fmt.Sprintf("--remote-env=SSL_CERT_FILE=%s", ghproxy.CACertMountPath))
	envVars = append(envVars, fmt.Sprintf("--remote-env=NODE_EXTRA_CA_CERTS=%s", ghproxy.CACertMountPath))

	// GIT_CONFIG_COUNT, GIT_CONFIG_KEY_0, and GIT_CONFIG_VALUE_0 configure
	// git to rewrite GitHub URLs so that git operations (clone, push, pull)
	// also route through the proxy. The insteadOf directive maps
	// https://GH_HOST/ URLs to https://github.com/ URLs so git can reach
	// the proxy when users clone/push to GitHub remotes.
	envVars = append(envVars, "--remote-env=GIT_CONFIG_COUNT=1")
	envVars = append(envVars, fmt.Sprintf("--remote-env=GIT_CONFIG_KEY_0=url.https://%s/.insteadOf", ghHost))
	envVars = append(envVars, "--remote-env=GIT_CONFIG_VALUE_0=https://github.com/")

	return envVars
}

// buildExecArgs assembles the arguments for devcontainer exec. It includes
// the container ID, workspace folder, and remote env vars for the proxy.
// If no command is specified, it defaults to bash for an interactive shell.
func buildExecArgs(containerID string, remoteEnv []string, userArgs []string) []string {
	args := []string{"exec"}

	args = append(args, "--workspace-folder", workspaceFolder)

	// Add the container ID so devcontainer exec knows which container to
	// target. The devcontainer CLI uses this to find the running container.
	args = append(args, "--container-id", containerID)

	// Add remote env vars for the GitHub API proxy (GH_HOST,
	// NODE_EXTRA_CA_CERTS, GIT_CONFIG_*). These are only present when the
	// proxy is enabled.
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

// setupSignalHandler installs a signal handler for SIGINT and SIGTERM that
// cancels the given context. This allows the proxy to shut down cleanly when
// the user presses Ctrl+C or the process receives a termination signal.
func setupSignalHandler() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		cancel()
	}()

	return ctx
}
