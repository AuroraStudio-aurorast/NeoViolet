package cmd

import (
	"os"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/logger"
	neoviolet "github.com/AuroraStudio-aurorast/neoviolet/internal/ui"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/ui/wizard"
)

// rootCmd is the root CLI command for NeoViolet.
var rootCmd = &cobra.Command{
	Use:   "neoviolet [flags] [filepath]",
	Short: "NeoViolet - a terminal music player",
	Long:  `NeoViolet - a terminal music player`,
	Args:  cobra.MaximumNArgs(1),
	// SilenceErrors lets us control error output in Execute().
	SilenceErrors: true,
	// Version enables the --version flag.
	Version: Version,
	RunE:    runRoot,
}

var (
	flagVolume    int  // --vol: initial volume percentage (0–100)
	flagSeek      int  // --seek: seek to position in seconds
	flagXDGConfig bool // --xdg-config: use XDG standard config path
)

func init() {
	rootCmd.CompletionOptions.DisableDefaultCmd = true
	rootCmd.SetVersionTemplate("NeoViolet version {{.Version}}\n")
	rootCmd.Flags().IntVar(&flagVolume, "vol", 0, "initial volume (0-100, 0=default)")
	rootCmd.Flags().IntVar(&flagSeek, "seek", 0, "seek to position in seconds at start")
	rootCmd.Flags().BoolVar(&flagXDGConfig, "xdg-config", false, "store config at XDG standard path (~/.config/neoviolet/config.json)")
}

func runRoot(cmd *cobra.Command, args []string) error {
	if err := logger.Init(); err != nil {
		return err
	}
	defer logger.Close()

	// Apply --xdg-config before any config access
	config.SetXDGConfig(flagXDGConfig)

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

	// Apply --vol flag: override config default if explicitly set
	if flagVolume > 0 {
		vol := float64(flagVolume) / 100.0
		if vol > 1.0 {
			vol = 1.0
		}
		cfg.DefaultVolume = vol
		logger.Info("Volume set via flag", "volume", cfg.DefaultVolume)
	}

	var filePath string
	if len(args) > 0 {
		filePath = args[0]
	}

	var seekDuration time.Duration
	if flagSeek > 0 {
		seekDuration = time.Duration(flagSeek) * time.Second
	}

	model := neoviolet.NewModel(filePath, cfg, seekDuration)
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
		return err
	}
	if model, ok := m.(*neoviolet.Model); ok {
		if model.ExitCode == 0 {
			// Normal quit: persist runtime volume to config
			model.Config.DefaultVolume = model.Audio.Volume
			if saveErr := model.Config.Save(); saveErr != nil {
				logger.Warn("Failed to save config on quit", "err", saveErr)
			}
		} else {
			logger.Info("Program exited with code", "code", model.ExitCode)
			os.Exit(model.ExitCode)
		}
	}
	logger.Info("Program exited")
	return nil
}