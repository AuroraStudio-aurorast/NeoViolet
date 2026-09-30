package config

// AnimConfig holds the NVAA animation (sidecar) settings.
type AnimConfig struct {
	// Auto shows a track's animation as soon as the track is loaded, without
	// waiting to be asked for it. It is off by default: taking over the content
	// area on its own is a surprise, and a viewer who wants it can say so once.
	Auto bool `json:"auto"`

	// Photosensitivity is the gate that stops an animation which flashes.
	Photosensitivity PhotosensitivityConfig `json:"photosensitivity"`

	// Power is the gate that holds an animation back on a low battery.
	Power PowerConfig `json:"power"`
}

// PowerConfig controls the gate that holds an animation back when the machine
// is running off a battery with little charge left in it.
type PowerConfig struct {
	// WarnBelow is the charge at or below which an animation waits for an
	// answer before it is read. Zero turns the warning off, which is a
	// documented choice rather than an absent key: Load starts from the
	// defaults, so a key that is not there keeps DefaultWarnBelow.
	WarnBelow int `json:"warn_below"`
}

// DefaultWarnBelow is the charge at which a battery counts as low. It is the
// figure the systems themselves put their own low battery warning at.
const DefaultWarnBelow = 20

// maxPercent is the ceiling on a charge, and so on a hand-written warning line.
const maxPercent = 100

// PhotosensitivityConfig controls the gate that stops an animation which
// flashes.
type PhotosensitivityConfig struct {
	Mode string `json:"mode"` // GateModeEither | GateModeDeclared | GateModeOff
}

// The photosensitivity gate modes. None of them is the zero value, so Normalize
// falls back to the union rather than to whatever an absent key decodes as: a
// hand-edited config cannot end up with a weaker gate than the default by
// accident.
const (
	// GateModeEither stops when the file's own check or our analysis objects.
	GateModeEither = "either"
	// GateModeDeclared stops on the file's own check only. It skips our
	// analysis, which is a walk over every frame of the animation.
	GateModeDeclared = "declared"
	// GateModeOff plays without stopping.
	GateModeOff = "off"

	DefaultGateMode = GateModeEither
)

// normalizeAnim repairs the animation settings. An unknown gate mode falls back
// to the union, never to off, so a typo cannot quietly remove the check that
// stands between a viewer and a flashing file: weakening it takes writing one
// of the documented words.
func normalizeAnim(a *AnimConfig) {
	switch a.Photosensitivity.Mode {
	case GateModeEither, GateModeDeclared, GateModeOff:
	default:
		a.Photosensitivity.Mode = DefaultGateMode
	}

	// Zero is the documented off, so it is left where it is. What is clamped is
	// a charge that cannot exist.
	a.Power.WarnBelow = clampInt(a.Power.WarnBelow, 0, maxPercent)
}
