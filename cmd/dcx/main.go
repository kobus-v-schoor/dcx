package main

import (
	"fmt"
	"os"

	"github.com/kobus-v-schoor/dcx/internal/cli"

	// Register proxy providers so that proxy.SetupAllProxies() can discover
	// them. Each provider registers itself via proxy.RegisterProvider in its
	// init() function. The blank import ensures the init() runs even though
	// no symbols are used directly from this package.
	_ "github.com/kobus-v-schoor/dcx/internal/proxy/github"
)

var version = "dev"

func main() {
	if err := cli.Execute(version); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
