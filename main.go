package main

import (
	"os"

	cmd "github.com/Stoakes/k8s-watcher-rds-server/cmd"
)

func main() {
	if err := cmd.RootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
