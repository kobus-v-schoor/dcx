package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/kobus-v-schoor/dcx/internal/docker"
	"github.com/kobus-v-schoor/dcx/internal/ghproxy"
	"github.com/kobus-v-schoor/dcx/internal/runner"
	"github.com/spf13/cobra"
)

// newExecCmd creates the "exec" subcommand. It opens an interactive shell or
// executes a command inside the running devcontainer, with the GitHub API
// proxy active if enabled in the config. The proxy injects the host's GitHub
// token into gh CLI requests without exposing the token inside the container.
// Added to the root command tree in Execute().
func newExecCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "exec [flags] [-- command [args...]]",
		Short: "Execute a shell or command inside the devcontainer with optional GitHub API proxy",
		Long: `Open an interactive shell or execute a command inside the running devcontainer.
When github_cli is enabled in the config, starts a GitHub API proxy that injects
the host's GitHub token into all gh CLI requests for the duration of the session.
The token is never exposed inside the container.
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
	// intercepts all gh CLI requests from the container and injects
	// the host's GitHub token. If no token is available on the host,
	// a warning is logged and the container runs without proxy.
	var proxy *ghproxy.Proxy
	var caCertPath string
	var remoteEnv []string

	if activeCfg.GitHubCLI.Enabled {
		var err error
		proxy, caCertPath, remoteEnv, err = setupProxy(cmd, containerID)
		if err != nil {
			// If proxy setup fails, log a warning and proceed without it —
			// the user gets a shell but without GitHub API access via proxy.
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
// the host's GitHub token, creates the proxy with options from the config,
// writes the CA cert to a temp file, copies it into the container via the
// Docker API, creates a combined CA bundle, and returns the proxy instance,
// CA cert file path, and the remote env vars to inject into the container.
// Returns an error if no token is available or the proxy fails to start.
// The caller is responsible for calling proxy.Shutdown() and cleaning up
// the CA cert file.
func setupProxy(cmd *cobra.Command, containerID string) (*ghproxy.Proxy, string, []string, error) {
	// Detect the host's GitHub token.
	token, ok := ghproxy.DetectToken()
	if !ok {
		return nil, "", nil, fmt.Errorf("no GitHub token available on host")
	}

	// Get the Docker client early — we need it to detect the gateway IP
	// before starting the proxy, since the gateway IP is included in the
	// TLS certificate's IP SANs.
	dockerCLI, err := docker.NewClient(cmd.Context())
	if err != nil {
		return nil, "", nil, fmt.Errorf("creating Docker client: %w", err)
	}
	defer func() { _ = dockerCLI.Close() }()

	// Determine the host IP that the container can reach. The proxy listens
	// on the gateway IP by default (more secure) so it is reachable from
	// the container. We inspect the container's network to find the gateway
	// IP, which is the host's IP on the Docker bridge network. This is more
	// reliable than host.docker.internal, which may not be routable in all
	// environments (e.g. codespaces).
	gatewayIP, err := docker.GatewayIP(cmd.Context(), dockerCLI, containerID)
	if err != nil {
		return nil, "", nil, fmt.Errorf("detecting host gateway IP: %w", err)
	}

	// Build proxy options from the config. The gateway IP is included in the
	// TLS certificate's IP SANs so the container can verify the connection.
	// By default the proxy binds to the gateway IP only (not 0.0.0.0), which
	// is more secure as it limits the attack surface to the Docker bridge
	// network.
	opts := ghproxy.Options{
		Token:      token,
		GatewayIP:  gatewayIP,
		BindAddr:   activeCfg.GitHubCLI.BindAddr,
		APIURL:     activeCfg.GitHubCLI.APIURL,
		CACertPath: activeCfg.GitHubCLI.CACertPath,
		CertExpiry: activeCfg.GitHubCLI.CertExpiry,
	}

	proxy := ghproxy.New(opts)
	port, err := proxy.Start()
	if err != nil {
		return nil, "", nil, fmt.Errorf("starting GitHub API proxy: %w", err)
	}

	// Write the CA certificate to a temp file on the host, then copy it into
	// the container via the Docker API. The devcontainer exec command does not
	// support --mount flags, so we use the Docker API's CopyToContainer to
	// place the CA cert file directly into the container filesystem.
	caCertPath, err := ghproxy.WriteCACertToFile(proxy.CACertPEM())
	if err != nil {
		proxy.Shutdown()
		return nil, "", nil, fmt.Errorf("writing CA certificate: %w", err)
	}

	// Create the target directory inside the container and copy the CA cert.
	// The directory must exist before CopyToContainer can place files into it.
	resolvedOpts := proxy.Opts()
	caDir := filepath.Dir(resolvedOpts.CACertPathResolved())
	if err := docker.MkdirInContainer(cmd.Context(), dockerCLI, containerID, caDir); err != nil {
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
		caDir,
	); err != nil {
		// Log the error and proceed — the gh CLI may still work without
		// the CA cert if the proxy is not used for HTTPS verification.
		slog.Warn("could not copy CA cert into container, gh CLI may not trust the proxy", "error", err)
	}

	// Create a combined CA bundle inside the container that merges the
	// system CA certificates with the proxy's self-signed CA certificate.
	// This is required because Go's SSL_CERT_FILE replaces the system CA
	// pool entirely (rather than appending to it), so a bundle containing
	// only the proxy CA would break HTTPS for all other Go programs in the
	// container. NODE_EXTRA_CA_CERTS does not need this — Node.js appends
	// to the system trust store.
	if err := createCABundleInContainer(cmd.Context(), dockerCLI, containerID, resolvedOpts); err != nil {
		slog.Warn("could not create combined CA bundle in container, Go programs may have HTTPS issues", "error", err)
	}

	// Build the remote env vars that configure the gh CLI and git inside
	// the container to route through the proxy.
	remoteEnv := proxy.BuildRemoteEnv(port)

	return proxy, caCertPath, remoteEnv, nil
}

// createCABundleInContainer creates a combined CA bundle file inside the
// container that includes both the system CA certificates and the proxy's
// self-signed CA certificate. This is necessary because Go's SSL_CERT_FILE
// environment variable replaces the system CA pool entirely (it does not
// append), so setting it to only the proxy CA would break all HTTPS
// connectivity for Go programs in the container. The combined bundle is
// referenced by SSL_CERT_FILE, while NODE_EXTRA_CA_CERTS points to the
// proxy CA alone (Node.js appends rather than replaces).
func createCABundleInContainer(ctx context.Context, dockerCLI docker.DockerClient, containerID string, opts ghproxy.Options) error {
	// Build a multi-line script that concatenates the system CA bundle with the
	// proxy's CA cert. The system CA bundle location varies by distro; we
	// check the most common paths in order.
	//
	// Debian/Ubuntu: /etc/ssl/certs/ca-certificates.crt
	// Alpine:        /etc/ssl/certs/ca-certificates.crt (same)
	// RHEL/Fedora:   /etc/pki/tls/certs/ca-bundle.crt
	// OpenSUSE:      /etc/ssl/ca-bundle.pem
	script := fmt.Sprintf(`
		sys_ca=""
		for f in /etc/ssl/certs/ca-certificates.crt /etc/pki/tls/certs/ca-bundle.crt /etc/ssl/ca-bundle.pem; do
			if [ -f "$f" ]; then
				sys_ca="$f"
				break
			fi
		done
		if [ -n "$sys_ca" ]; then
			cat "$sys_ca" %s > %s
		else
			cp %s %s
		fi`,
		opts.CACertPathResolved(),
		opts.CABundlePathResolved(),
		opts.CACertPathResolved(),
		opts.CABundlePathResolved(),
	)

	if err := docker.ExecInContainer(ctx, dockerCLI, containerID, "sh", "-c", script); err != nil {
		return fmt.Errorf("creating combined CA bundle in container: %w", err)
	}

	return nil
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
