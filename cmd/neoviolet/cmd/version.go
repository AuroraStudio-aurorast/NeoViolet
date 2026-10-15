package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/audio/synth"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/version"
)

// versionCmd represents the "version" subcommand.
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	RunE: func(_ *cobra.Command, _ []string) error {
		fmt.Println("neoviolet", version.Version)

		// libopenmpt is optional, so name the library this run resolved: it is
		// what decides whether the tracker formats only it can decode will play.
		if synth.OpenmptAvailable() {
			fmt.Printf("libopenmpt %s (%s)\n", synth.OpenmptVersion(), synth.OpenmptLibraryPath())
		} else {
			fmt.Println("libopenmpt not available")
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
