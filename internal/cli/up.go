package cli

import (
	"fmt"
	"log/slog"

	"github.com/kobus-v-schoor/dcx/internal/config"
	"github.com/kobus-v-schoor/dcx/internal/env"
	"github.com/kobus-v-schoor/dcx/internal/flags"
	"github.com/kobus-v-schoor/dcx/internal/git"
	"github.com/kobus-v-schoor/dcx/internal/override"
	"github.com/kobus-v-schoor/dcx/internal/runner"
	"github.com/kobus-v-schoor/dcx/internal/ssh"
	"github.com/spf13/cobra"
)

// newUpCmd creates the "up" subcommand. It reads the already-loaded config,
// creates the override directory, assembles devcontainer CLI flags, and
// delegates execution. Added to the root command tree in Execute().
func newUpCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "up",
		Short: "Start a devcontainer using dcx configuration",
		Long:  "Start a devcontainer by delegating to the devcontainer CLI with dcx-assembled flags.\nAny flags after -- are passed through to devcontainer up unchanged.",
		Args:  cobra.ArbitraryArgs,
		RunE:  runUp,
	}
}

// runUp implements the dcx up workflow. Called by Cobra when the user
// runs "dcx up". Config and log level are already initialised by the
// root command's PersistentPreRunE.
func runUp(cmd *cobra.Command, args []string) error {
	slog.Info("workspace-folder", "path", workspaceFolder)

	devcontainerPath, err := runner.Find()
	if err != nil {
		return err
	}
	slog.Info("found devcontainer CLI", "path", devcontainerPath)

	slog.Info("config loaded")
	slog.Debug("ssh.forward_agent", "enabled", activeCfg.SSH.ForwardAgent)
	slog.Debug("git.inject_configs", "enabled", activeCfg.Git.InjectConfigs)

	overrideDir, cleanup, err := override.Create(workspaceFolder)
	if err != nil {
		return fmt.Errorf("creating override config: %w", err)
	}
	defer cleanup()

	slog.Info("override dir", "path", overrideDir)

	// Collect all container env vars (user-configured, SSH agent, git config)
	// and inject them into the override config's containerEnv property. This
	// makes the env vars persistent Docker-level environment variables in the
	// running container (visible via env, docker exec, etc.), unlike remoteEnv
	// which only applies to VS Code server processes or --remote-env flags
	// which only apply during lifecycle commands.
	containerEnvVars := collectContainerEnv(activeCfg)
	if err := override.InjectContainerEnv(overrideDir, containerEnvVars); err != nil {
		return fmt.Errorf("injecting container env: %w", err)
	}

	dcArgs := flags.Build(workspaceFolder, activeCfg, overrideDir)

	dcArgs = append(dcArgs, args...)

	slog.Debug("invoking devcontainer", "args", dcArgs)

	return runner.Run(devcontainerPath, dcArgs)
}

// collectContainerEnv gathers all environment variables that should be set in the
// devcontainer from three sources: (1) user-configured environment passthrough
// declarations, (2) SSH agent forwarding env vars, and (3) git config env vars.
// Each source is resolved independently; later sources overwrite earlier ones
// on name conflict (user < SSH < git precedence for same env var name). Returns
// an empty map when no env vars are produced. The returned map is injected into
// the override config's containerEnv property so the vars become Docker-level
// environment variables visible in the running container.
func collectContainerEnv(cfg *config.Config) map[string]string {
	result := make(map[string]string)

	// 1. User-configured environment variable passthrough.
	for _, resolved := range env.ResolveAll(cfg.Environment) {
		result[resolved.Name] = resolved.Value
	}

	// 2. SSH agent forwarding env var.
	if cfg.SSH.ForwardAgent {
		agentResult := ssh.DetectAgent(cfg.SSH)
		if agentResult.Mount != nil {
			result[agentResult.EnvName] = agentResult.EnvValue
		}
	}

	// 3. Git config forwarding env var.
	if cfg.Git.InjectConfigs {
		gitResult := git.DetectConfigs(cfg.Git)
		if gitResult.EnvName != "" {
			result[gitResult.EnvName] = gitResult.EnvValue
		}
	}

	return result
}
