package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/version"
)

// versionCmd represents the "version" subcommand.
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	RunE: func(_ *cobra.Command, _ []string) error {
		fmt.Println("neoviolet", version.Version)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
