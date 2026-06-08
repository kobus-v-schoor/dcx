package cli

import (
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/kobus-v-schoor/dcx/internal/compose"
	"github.com/kobus-v-schoor/dcx/internal/docker"
	"github.com/moby/moby/api/types/container"
	"github.com/spf13/cobra"
)

// newPsCmd creates the "ps" subcommand. It lists all containers associated
// with the current project's devcontainer, including both the devcontainer
// itself and any Docker Compose services in the same compose project. The
// output format mirrors docker compose ps. Added to the root command tree in
// Execute().
func newPsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ps",
		Short: "List containers for the current project's devcontainer",
		Long:  "Lists all devcontainer and Docker Compose containers for the current project.\nShows the container name, status, and image for each container.",
		RunE:  runPs,
	}
}

// runPs implements the dcx ps workflow. Called by Cobra when the user
// runs "dcx ps". Config, log level, and Docker daemon reachability are
// already verified by the root command's PersistentPreRunE.
func runPs(cmd *cobra.Command, args []string) error {
	slog.Info("workspace-folder", "path", workspaceFolder)

	cli, err := docker.NewClient(cmd.Context())
	if err != nil {
		return err
	}
	defer func() { _ = cli.Close() }()

	containers, err := compose.FindProjectContainers(cmd.Context(), cli, workspaceFolder)
	if err != nil {
		return fmt.Errorf("dcx ps: %w", err)
	}

	// Sort by name for stable output.
	sort.Slice(containers, func(i, j int) bool {
		return formatName(containers[i]) < formatName(containers[j])
	})

	if len(containers) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No containers found for this project.")
		return nil
	}

	return printContainers(cmd.OutOrStdout(), containers)
}

// printContainers writes a tabular listing of containers to w.
func printContainers(w io.Writer, containers []container.Summary) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tIMAGE\tCOMMAND\tSERVICE\tCREATED\tSTATUS\tPORTS")
	for _, ctr := range containers {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			formatName(ctr),
			ctr.Image,
			formatCommand(ctr.Command),
			formatService(ctr),
			formatCreated(ctr.Created),
			ctr.Status,
			formatPorts(ctr.Ports),
		)
	}
	return tw.Flush()
}

// formatName returns a human-readable name for the container.
// It uses the first Docker name with the leading slash stripped,
// or the short container ID if no name is present.
func formatName(ctr container.Summary) string {
	if len(ctr.Names) > 0 {
		return strings.TrimPrefix(ctr.Names[0], "/")
	}
	return docker.ShortID(ctr.ID)
}

// formatService returns the Docker Compose service name if the container
// is managed by Compose; otherwise it returns "-".
func formatService(ctr container.Summary) string {
	if svc := ctr.Labels["com.docker.compose.service"]; svc != "" {
		return svc
	}
	return "-"
}

// formatCommand returns the container's command string quoted and truncated
// to 20 runes, matching the docker compose ps output.
func formatCommand(command string) string {
	return strconv.Quote(ellipsis(command, 20))
}

// ellipsis truncates s to maxLen runes, appending "…" (three dots) when
// truncation occurs. For strings that fit within the limit, the original
// string is returned unchanged. Matches the docker CLI convention.
func ellipsis(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return "..."
	}
	return string(runes[:maxLen-3]) + "..."
}

// formatCreated returns a human-readable "time ago" string from the
// container's creation Unix timestamp, e.g. "2 minutes ago".
func formatCreated(created int64) string {
	return humanDuration(time.Since(time.Unix(created, 0))) + " ago"
}

// humanDuration converts a duration into a concise human-readable string,
// e.g. "2 minutes", "3 hours", "1 day". Units are rounded down to the most
// significant non-zero unit.
func humanDuration(d time.Duration) string {
	seconds := int(d.Seconds())
	minutes := seconds / 60
	hours := minutes / 60
	days := hours / 24

	switch {
	case days > 0:
		if days == 1 {
			return "1 day"
		}
		return fmt.Sprintf("%d days", days)
	case hours > 0:
		if hours == 1 {
			return "1 hour"
		}
		return fmt.Sprintf("%d hours", hours)
	case minutes > 0:
		if minutes == 1 {
			return "1 minute"
		}
		return fmt.Sprintf("%d minutes", minutes)
	default:
		if seconds <= 1 {
			return "1 second"
		}
		return fmt.Sprintf("%d seconds", seconds)
	}
}

// formatPorts formats the container's port bindings into a single string
// matching the docker ps / docker compose ps conventions, e.g.
// "0.0.0.0:80->8080/tcp, 5432/tcp".
func formatPorts(ports []container.PortSummary) string {
	if len(ports) == 0 {
		return ""
	}

	// Sort for deterministic output matching Docker's ordering.
	sort.Slice(ports, func(i, j int) bool {
		if ports[i].PrivatePort != ports[j].PrivatePort {
			return ports[i].PrivatePort < ports[j].PrivatePort
		}
		if ports[i].IP != ports[j].IP {
			return ports[i].IP.String() < ports[j].IP.String()
		}
		if ports[i].PublicPort != ports[j].PublicPort {
			return ports[i].PublicPort < ports[j].PublicPort
		}
		return ports[i].Type < ports[j].Type
	})

	var parts []string
	for _, p := range ports {
		if p.PublicPort > 0 {
			ip := "0.0.0.0"
			if p.IP.IsValid() {
				ip = p.IP.String()
			}
			parts = append(parts, fmt.Sprintf("%s:%d->%d/%s", ip, p.PublicPort, p.PrivatePort, p.Type))
		} else {
			parts = append(parts, fmt.Sprintf("%d/%s", p.PrivatePort, p.Type))
		}
	}
	return strings.Join(parts, ", ")
}
