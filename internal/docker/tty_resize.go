package docker

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

// monitorExecResize listens for SIGWINCH and forwards the current terminal
// size to the Docker exec process. It returns a cancellation function that
// stops the monitoring goroutine. No-op if the terminal size cannot be read.
func monitorExecResize(ctx context.Context, cli DockerClient, execID string) func() {
	sigWinch := make(chan os.Signal, 1)
	signal.Notify(sigWinch, syscall.SIGWINCH)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-done:
				signal.Stop(sigWinch)
				return
			case <-ctx.Done():
				signal.Stop(sigWinch)
				return
			case <-sigWinch:
				if err := resizeExecTTY(ctx, cli, execID); err != nil {
					slog.Warn("failed to resize exec TTY", "exec_id", execID, "error", err)
				}
			}
		}
	}()
	return func() { close(done) }
}
