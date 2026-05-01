package main

import (
	"os"

	tea "charm.land/bubbletea/v2"

	"neoviolet/internal/logger"
	neoviolet "neoviolet/internal/ui"
)

func main() {
	if err := logger.Init(); err != nil {
		println("Logger init failed:", err.Error())
		os.Exit(1)
	}
	defer logger.Close()

	var filePath string
	if len(os.Args) > 1 {
		filePath = os.Args[1]
	}

	m, err := tea.NewProgram(neoviolet.NewModel(filePath)).Run()
	if err != nil {
		logger.Error("Program error", "err", err)
		os.Exit(1)
	}
	if m != nil && m.(*neoviolet.Model).ExitCode != 0 {
		os.Exit(m.(*neoviolet.Model).ExitCode)
	}
	logger.Info("Program exited")
}