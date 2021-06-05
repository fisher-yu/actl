package cmd

import (
	"os"

	"github.com/fisher-yu/actl/log"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "actl",
	Short: "actl is a powerful scaffold",
	Long:  `actl is a powerful scaffold`,
}

func init() {
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(modelCmd)
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}
}
