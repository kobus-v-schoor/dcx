package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/kobus-v-schoor/dcx/internal/cli"
	"github.com/kobus-v-schoor/dcx/internal/runner"

	// Register proxy providers so that proxy.SetupAllProxies() can discover
	// them. Each provider registers itself via proxy.RegisterProvider in its
	// init() function. The blank import ensures the init() runs even though
	// no symbols are used directly from this package.
	_ "github.com/kobus-v-schoor/dcx/internal/proxy/github"
)

var version = "dev"

func main() {
	if err := cli.Execute(version); err != nil {
		var exitErr *runner.ExitCodeError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
