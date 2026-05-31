package cli

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/kobus-v-schoor/dcx/internal/config"
	"github.com/kobus-v-schoor/dcx/internal/docker"
	"github.com/kobus-v-schoor/dcx/internal/env"
	"github.com/kobus-v-schoor/dcx/internal/flags"
	"github.com/kobus-v-schoor/dcx/internal/git"
	"github.com/kobus-v-schoor/dcx/internal/mounts"
	"github.com/kobus-v-schoor/dcx/internal/override"
	"github.com/kobus-v-schoor/dcx/internal/runner"
	"github.com/kobus-v-schoor/dcx/internal/ssh"
	"github.com/moby/moby/api/types/container"
	"github.com/spf13/cobra"
)

// newUpCmd creates the "up" subcommand. It reads the already-loaded config,
// creates the override directory, assembles devcontainer CLI flags, and
// delegates execution. The --rebuild flag maps to the devcontainer CLI's
// --remove-existing-container flag, forcing container recreation so that
// config changes (env vars, mounts, features) take effect. Added to the
// root command tree in Execute().
func newUpCmd() *cobra.Command {
	var rebuild bool

	cmd := &cobra.Command{
		Use:   "up",
		Short: "Start a devcontainer using dcx configuration",
		Long:  "Start a devcontainer by delegating to the devcontainer CLI with dcx-assembled flags.\nAny flags after -- are passed through to devcontainer up unchanged.",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUp(cmd.Context(), rebuild, args)
		},
	}

	cmd.Flags().BoolVar(&rebuild, "rebuild", false, "recreate the container if it already exists, so config changes take effect")

	return cmd
}

// runUp implements the dcx up workflow. Called by Cobra when the user
// runs "dcx up". The rebuild parameter controls whether the devcontainer
// CLI's --remove-existing-container flag is emitted, forcing container
// recreation so config changes take effect. Config and log level are
// already initialised by the root command's PersistentPreRunE.
func runUp(ctx context.Context, rebuild bool, args []string) error {
	slog.Info("workspace-folder", "path", workspaceFolder)

	devcontainerPath, err := runner.Find()
	if err != nil {
		return err
	}
	slog.Info("found devcontainer CLI", "path", devcontainerPath)

	slog.Info("config loaded")
	slog.Debug("ssh.forward_agent", "enabled", activeCfg.SSH.ForwardAgent)
	slog.Debug("git.inject_configs", "enabled", activeCfg.Git.InjectConfigs)

	overrideDir, err := override.Create(workspaceFolder, activeCfg.DefaultImage)
	if err != nil {
		return fmt.Errorf("creating override config: %w", err)
	}
	defer overrideDir.Close()

	slog.Info("override dir", "path", overrideDir.Dir)

	// Detect the SSH agent once so both env and mount collection use the
	// same result. ResolveAgent handles VM-based runtimes (e.g. Colima) by
	// resolving the socket path inside the VM rather than on the host.
	var agentResult ssh.AgentResult
	if activeCfg.SSH.ForwardAgent {
		agentResult = ssh.ResolveAgent(activeCfg.SSH)
	}

	// Prepare terminfo forwarding. This captures the local terminal's
	// terminfo source using infocmp, writes it to a stable host file, and
	// returns a mount and postCreateCommand to compile it inside the
	// container with tic. The compiled entry is placed in the container user's
	// ~/.terminfo directory, which ncurses searches automatically.
	terminfoResult := env.PrepareTerminfo(overrideDir.ContainerHomeDir)

	// Collect all container env vars (user-configured, SSH agent, git config)
	// and inject them into the override config's containerEnv property. This
	// makes the env vars persistent Docker-level environment variables in the
	// running container (visible via env, docker exec, etc.), unlike remoteEnv
	// which only applies to VS Code server processes or --remote-env flags
	// which only apply during lifecycle commands.
	containerEnvVars := collectContainerEnv(activeCfg, agentResult, overrideDir.ContainerWorkspaceFolder)
	overrideDir.InjectContainerEnv(containerEnvVars)

	// Collect all mount strings (user-configured, SSH agent, git config) and
	// inject them into the override config's mounts property. Mounts are
	// injected via the config rather than --mount CLI flags because the
	// devcontainer CLI's --mount flag has a strict format that only accepts
	// type, source, target and external — it does not support readonly or
	// other Docker mount options. The devcontainer.json mounts property
	// accepts the full Docker --mount format.
	mountStrings := collectMountStrings(activeCfg, agentResult, terminfoResult, overrideDir.ContainerHomeDir)
	overrideDir.InjectMounts(mountStrings)

	// Inject the terminfo compilation postCreateCommand if terminfo forwarding
	// is active. This runs alongside any project-defined postCreateCommand.
	if terminfoResult.PostCreateCommand != "" {
		overrideDir.InjectPostCreateCommand(terminfoResult.PostCreateCommand)
	}

	// Persist all injected modifications to disk before delegating to the
	// devcontainer CLI.
	if err := overrideDir.Save(); err != nil {
		return fmt.Errorf("saving override config: %w", err)
	}

	// Check for stale bind mounts on a stopped devcontainer. If a bind mount
	// source no longer exists, Docker will refuse to start the container.
	// Return a helpful error so the user can decide how to proceed instead of
	// silently removing the container.
	if !rebuild {
		cli, err := docker.NewClient(ctx)
		if err != nil {
			return err
		}
		defer func() { _ = cli.Close() }()

		containers, err := docker.FindDevcontainers(ctx, cli, workspaceFolder)
		if err != nil {
			return fmt.Errorf("checking for existing devcontainer: %w", err)
		}

		if len(containers.Items) > 0 && containers.Items[0].State != container.StateRunning {
			if err := docker.CheckStaleMounts(ctx, cli, containers.Items[0].ID); err != nil {
				return err
			}
		}
	}

	dcArgs := flags.Build(workspaceFolder, activeCfg, overrideDir.Dir, rebuild)

	dcArgs = append(dcArgs, args...)

	slog.Debug("invoking devcontainer", "args", dcArgs)

	return runner.Run(devcontainerPath, dcArgs)
}

// collectContainerEnv gathers all environment variables that should be set in the
// devcontainer from four sources: (1) auto-forwarded variables (e.g. TERM),
// (2) user-configured environment passthrough declarations, (3) SSH agent
// forwarding env vars, and (4) git config env vars. Each source is resolved
// independently; later sources overwrite earlier ones on name conflict
// (auto < user < SSH < git precedence for same env var name). Returns an empty
// map when no env vars are produced. The returned map is injected into the
// override config's containerEnv property so the vars become Docker-level
// environment variables visible in the running container.
func collectContainerEnv(cfg *config.Config, agentResult ssh.AgentResult, containerWorkspaceFolder string) map[string]string {
	result := make(map[string]string)

	// 1. Auto-forwarded environment variables (e.g. TERM for TUI support).
	// These have the lowest precedence — user config can override them.
	for _, resolved := range env.AutoForward() {
		result[resolved.Name] = resolved.Value
	}

	// 2. User-configured environment variable passthrough.
	for _, resolved := range env.ResolveAll(cfg.Environment) {
		result[resolved.Name] = resolved.Value
	}

	// 3. SSH agent forwarding env var.
	if cfg.SSH.ForwardAgent && agentResult.Mount != nil {
		result[agentResult.EnvName] = agentResult.EnvValue
	}

	// 4. Git config forwarding env var and safe.directory trust.
	if cfg.Git.InjectConfigs {
		gitResult := git.DetectConfigs(cfg.Git)
		if gitResult.EnvName != "" {
			result[gitResult.EnvName] = gitResult.EnvValue
		}
		for _, resolved := range env.BuildGitConfigEnv(git.SafeDirConfig(containerWorkspaceFolder)) {
			result[resolved.Name] = resolved.Value
		}
	}

	return result
}

// collectMountStrings gathers all mount strings that should be injected into
// the override config's mounts property from four sources: (1) user-configured
// bind mounts from the config, (2) TERMINFO source file mount, (3) SSH agent
// socket mount, and (4) git config file mounts. Each source is resolved
// independently; mounts with missing
// source paths on the host are silently skipped. The containerHomeDir is used
// to expand ~/ in user-configured mount targets to the container user's home
// directory. Returns nil when no mounts are produced.
func collectMountStrings(cfg *config.Config, agentResult ssh.AgentResult, terminfoResult env.TerminfoResult, containerHomeDir string) []string {
	var result []string

	// 1. User-configured bind mounts.
	result = append(result, mounts.BuildStrings(cfg.Mounts, containerHomeDir)...)

	// 1a. TERMINFO source file bind mount. Captured from the host using
	// infocmp and compiled inside the container with tic during postCreateCommand.
	if terminfoResult.Mount != nil {
		result = append(result, mounts.Format(*terminfoResult.Mount))
	}

	// 2. SSH agent forwarding mount.
	if cfg.SSH.ForwardAgent && agentResult.Mount != nil {
		result = append(result, mounts.Format(*agentResult.Mount))
	}

	// 3. Git config forwarding mounts.
	if cfg.Git.InjectConfigs {
		gitResult := git.DetectConfigs(cfg.Git)
		for _, m := range gitResult.Mounts {
			result = append(result, mounts.Format(*m))
		}
	}

	return result
}
