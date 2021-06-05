package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

const version = "1.0.0"

func init() {
	
	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number of ahead",
	Long:  `Print the version number of ahead`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Version: v" + version)
	},
}
