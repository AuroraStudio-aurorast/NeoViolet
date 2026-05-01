package main

import (
	"flag"
	"os"

	tea "charm.land/bubbletea/v2"

	"neoviolet/internal/logger"
	neoviolet "neoviolet/internal/ui"
)

func main() {
	fallbackIcon := flag.Bool("fallback-icon", false, "use non-NerdFont icons")
	emoji := flag.Bool("emoji", false, "use emoji icons instead of NerdFont")
	flag.Parse()

	// Support flags after positional args (flag package stops at first non-flag)
	for _, arg := range os.Args[1:] {
		switch arg {
		case "--fallback-icon", "-fallback-icon":
			*fallbackIcon = true
		case "--emoji", "-emoji":
			*emoji = true
		}
	}

	if err := logger.Init(); err != nil {
		println("Logger init failed:", err.Error())
		os.Exit(1)
	}
	defer logger.Close()

	var filePath string
	args := flag.Args()
	if len(args) > 0 {
		filePath = args[0]
	}

	m, err := tea.NewProgram(neoviolet.NewModel(filePath, *fallbackIcon, *emoji)).Run()
	if err != nil {
		logger.Error("Program error", "err", err)
		os.Exit(1)
	}
	if m != nil && m.(*neoviolet.Model).ExitCode != 0 {
		os.Exit(m.(*neoviolet.Model).ExitCode)
	}
	logger.Info("Program exited")
}