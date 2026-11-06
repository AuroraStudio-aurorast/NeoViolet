package cmd

import (
	"os"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/logger"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/lyrics/fetch"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/mediactl"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/startup"
	neoviolet "github.com/AuroraStudio-aurorast/neoviolet/internal/ui"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/version"
)

// rootCmd is the root CLI command for NeoViolet.
var rootCmd = &cobra.Command{
	Use:   "neoviolet [flags] [filepath|-]",
	Short: "NeoViolet - a terminal music player",
	Long: `NeoViolet - a terminal music player

Use "-" as the file path to read audio data from stdin, e.g.:
  cat song.mp3 | neoviolet -
  curl -sL https://example.com/song.flac | neoviolet -`,
	Args: cobra.MaximumNArgs(1),
	// SilenceErrors lets us control error output in Execute().
	SilenceErrors: true,
	// Version enables the --version flag.
	Version: version.Version,
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

func runRoot(_ *cobra.Command, args []string) error {
	if err := logger.Init(); err != nil {
		return err
	}
	defer func() { _ = logger.Close() }()

	// Apply --xdg-config before any config access
	config.SetXDGConfig(flagXDGConfig)

	var filePath string
	if len(args) > 0 {
		filePath = args[0]
	}

	var seekDuration time.Duration
	if flagSeek > 0 {
		seekDuration = time.Duration(flagSeek) * time.Second
	}

	// Run everything inside runWithOSMedia so that on macOS the first-run wizard
	// and main program share the same NSApplication context. Running the wizard
	// before MacOSRun may leave the terminal in a state that interferes with the
	// AppKit event loop, preventing the main UI from rendering.
	return runWithOSMedia(func() error {
		needsWizard := !config.Exists()
		defaults := config.DefaultConfig()
		cfg := &defaults
		if !needsWizard {
			var err error
			cfg, err = config.Load()
			if err != nil {
				logger.Warn("Failed to load config", "err", err)
			}
		}

		// Sweep expired online-lyrics cache files at startup (best effort).
		go func() {
			if dir, err := config.CacheDir(); err == nil {
				if n, err := fetch.CleanupExpired(dir); err == nil && n > 0 {
					logger.Info("lyrics cache cleanup", "removed", n)
				}
			}
		}()

		model := startup.New(filePath, cfg, needsWizard, seekDuration, flagVolume)
		defer model.Close()
		p := tea.NewProgram(model)

		// Bridge OS media control commands (MPRIS on Linux, NowPlaying on macOS) into BubbleTea messages.
		// MediaCtl is initialized lazily after the program starts to avoid AppKit interaction issues
		// when the first-run wizard runs before the main program on macOS.
		go func() {
			mc, err := mediactl.New()
			if err != nil {
				logger.Warn("mediactl: deferred new failed", "err", err)
				return
			}
			cmdChan, err := mc.Start()
			if err != nil {
				logger.Error("mediactl: start failed", "err", err)
				return
			}
			// Hand the controller to the event loop (thread-safe via p.Send)
			p.Send(neoviolet.MediaCtlReadyMsg{Controller: mc})
			for cmd := range cmdChan {
				p.Send(neoviolet.MediaCtlMsg{Command: cmd})
			}
		}()

		// Start stdin listener for runtime track loading via pipe input
		go neoviolet.StdinListener(p)

		_, err := p.Run()
		if err != nil {
			logger.Error("Program error", "err", err)
			return err
		}
		if player := model.Player(); player != nil {
			if player.ExitCode == 0 {
				// Normal quit: persist runtime volume to config
				player.Config.DefaultVolume = player.Audio.Volume
				if saveErr := player.Config.Save(); saveErr != nil {
					logger.Warn("Failed to save config on quit", "err", saveErr)
				}
			} else {
				logger.Info("Program exited with code", "code", player.ExitCode)
				os.Exit(player.ExitCode)
			}
		}
		logger.Info("Program exited")
		return nil
	})
}
