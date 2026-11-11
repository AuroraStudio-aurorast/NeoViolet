package config

// MediaCtlConfig controls OS media integration. Changes take effect on restart.
type MediaCtlConfig struct {
	Enabled bool `json:"enabled"`
}
