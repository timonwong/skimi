package main

import (
	"os"

	"github.com/timonwong/skimi/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
