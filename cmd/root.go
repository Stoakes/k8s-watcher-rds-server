package cmd

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// RootCmd is the entrypoint for every command
var RootCmd = &cobra.Command{
	Use:               "watcher",
	Short:             "Kubernetes API to xDS server",
	SilenceUsage:      true,
	DisableAutoGenTag: true,
	Long:              `Watch Kubernetes API and serve xDS objects for Envoy dynamic configuration`,
}

func init() {
	flags := RootCmd.PersistentFlags()
	flags.StringP("kubeconfig", "c", "", "Kubernetes configuration file")
	viper.BindPFlag("kubeconfig", flags.Lookup("kubeconfig"))

	//flags.String("context", "", "The name of the kubeconfig context to use")
	//viper.BindPFlag("context", flags.Lookup("context"))

	//flags.StringP("namespace", "n", v1.NamespaceAll, "Config namespace")
	//viper.BindPFlag("namespace", flags.Lookup("namespace"))

	RootCmd.AddCommand(watchCmd)
}
