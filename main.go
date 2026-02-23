package main

import (
	"os"

	"github.com/nvandessel/tier/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
