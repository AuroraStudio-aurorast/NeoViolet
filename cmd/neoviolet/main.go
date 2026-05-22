package main

import (
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/logger"
	neoviolet "github.com/AuroraStudio-aurorast/neoviolet/internal/ui"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/ui/wizard"
)

func main() {
	if err := logger.Init(); err != nil {
		println("Logger init failed:", err.Error())
		os.Exit(1)
	}
	defer logger.Close()

	if !config.ConfigExists() {
		logger.Info("First run detected, launching setup wizard")
		wizardCfg, err := wizard.Run()
		if err != nil {
			logger.Warn("Wizard error, using defaults", "err", err)
		}
		if wizardCfg != nil {
			if saveErr := wizardCfg.Save(); saveErr != nil {
				logger.Warn("Failed to save wizard config", "err", saveErr)
			}
		}
	}

	cfg, err := config.Load()
	if err != nil {
		logger.Warn("Failed to load config", "err", err)
	}

	var filePath string
	if len(os.Args) > 1 {
		filePath = os.Args[1]
	}

	model := neoviolet.NewModel(filePath, cfg)
	p := tea.NewProgram(model)

	// Bridge OS media control commands (MPRIS on Linux) into BubbleTea messages
	if model.MediaCtl != nil {
		go func() {
			cmdChan, err := model.MediaCtl.Start()
			if err != nil {
				logger.Error("mediactl: start failed", "err", err)
				return
			}
			for cmd := range cmdChan {
				p.Send(neoviolet.MediaCtlMsg{Command: cmd})
			}
		}()
	}

	m, err := p.Run()
	if err != nil {
		logger.Error("Program error", "err", err)
		os.Exit(1)
	}
	if m, ok := m.(*neoviolet.Model); ok && m.ExitCode != 0 {
		os.Exit(m.ExitCode)
	}
	logger.Info("Program exited")
}
