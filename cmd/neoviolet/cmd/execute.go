package cmd

import "os"

// Execute is the entry point for the NeoViolet CLI.
// It runs the root command and exits with an appropriate code on failure.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}