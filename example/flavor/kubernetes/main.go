package main

import (
	"os"

	log "github.com/Sirupsen/logrus"
	"github.com/docker/infrakit/cli"
	"github.com/docker/infrakit/plugin/flavor/kubernetes"
	flavor_plugin "github.com/docker/infrakit/spi/http/flavor"
	"github.com/spf13/cobra"
)

func main() {

	logLevel := cli.DefaultLogLevel
	var name string

	cmd := &cobra.Command{
		Use:   os.Args[0],
		Short: "Kubernetes flavor plugin",
		Run: func(c *cobra.Command, args []string) {
			cli.SetLogLevel(logLevel)
			cli.RunPlugin(name, flavor_plugin.PluginServer(kubernetes.NewPlugin()))
		},
	}

	cmd.AddCommand(cli.VersionCommand())

	cmd.Flags().IntVar(&logLevel, "log", logLevel, "Logging level. 0 is least verbose. Max is 5")
	cmd.Flags().StringVar(&name, "name", "flavor-kubernetes", "Plugin name to advertise for discovery")
	err := cmd.Execute()
	if err != nil {
		log.Error(err)
		os.Exit(1)
	}
}
