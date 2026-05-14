package main

import (
	"fmt"
	"os"

	"github.com/kobus-v-schoor/dcx/internal/cli"
)

var version = "dev"

func main() {
	if err := cli.Execute(version); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
