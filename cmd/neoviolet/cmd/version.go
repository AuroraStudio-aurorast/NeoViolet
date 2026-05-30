package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Version is the application version, set via ldflags at build time.
// Default value "dev" is used when built without ldflags.
var Version = "dev"

// versionCmd represents the "version" subcommand.
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("neoviolet", Version)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
