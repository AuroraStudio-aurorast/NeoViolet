package anim

import (
	"github.com/WhatDamon/go-nvaa-codec"
	"github.com/WhatDamon/go-nvaa-codec/photosensitivity"
	"github.com/WhatDamon/go-nvaa-codec/player"

	"github.com/AuroraStudio-aurorast/neoviolet/internal/config"
	"github.com/AuroraStudio-aurorast/neoviolet/internal/logger"
)

// Photosensitivity is the pair of readings a warning is drawn from: what the
// file says about its own flashing, and what an analysis of its frames
// measured.
//
// Both are kept because they can disagree. A file that declares nothing is not
// thereby safe, and a file that declares a pass ran its own approximation over
// a viewport that need not be the one being watched. Showing them side by side
// lets a viewer see which reading objected.
type Photosensitivity struct {
	Declared   player.Warning
	Assessment photosensitivity.Assessment
}

// Fails reports whether the reading the gate mode acts on found more flashing
// than the thresholds allow. A reading that was never made does not object.
//
// GateModeOff has no reading that counts, and GateModeDeclared stops at the
// file's own check, so the modes that skip the analysis cannot be moved by it.
func (p Photosensitivity) Fails(mode string) bool {
	if mode == config.GateModeOff {
		return false
	}
	if p.Declared.Verdict == player.VerdictFail {
		return true
	}
	return mode != config.GateModeDeclared &&
		p.Assessment.Verdict == photosensitivity.VerdictFail
}

// Analysed reports whether the analysis produced a verdict. It is false when
// the analysis could not run or the gate mode skipped it, so a warning can say
// what it is missing rather than implying the animation was measured and
// passed.
func (p Photosensitivity) Analysed() bool {
	return p.Assessment.Verdict != photosensitivity.VerdictUnknown
}

// assess reads the photosensitivity opinions about an animation: what the file
// says about itself, and, unless the gate mode skips it, what an analysis of its
// frames measures.
func assess(animation *nvaa.Animation, mode string) Photosensitivity {
	readings := Photosensitivity{Declared: player.WarningFrom(animation)}

	if mode == config.GateModeOff || mode == config.GateModeDeclared {
		// The analysis walks every frame. A mode that does not act on it does not
		// pay for it, which is the difference between loading an animation and
		// loading one and measuring all of it.
		return readings
	}

	// The window is left unset so the analysis uses the viewport the file's own
	// frames declare. Measuring there is what makes the two readings comparable:
	// a different viewport would compare a claim about one size against a result
	// for another, and flashing area is measured as a fraction of the window.
	assessment, err := photosensitivity.Analyze(animation, photosensitivity.Options{})
	if err != nil {
		logger.Warn("photosensitivity analysis failed", "error", err)
		return readings
	}
	readings.Assessment = assessment
	return readings
}
