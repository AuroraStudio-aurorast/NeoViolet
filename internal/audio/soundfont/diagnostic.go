package soundfont

// Diagnostic describes a bounded aggregate of recoverable input issues.
type Diagnostic struct {
	Code, Message string
	Count         uint64
}

func (sf *SoundFont) diagnose(code, message string) {
	for i := range sf.Diagnostics {
		if sf.Diagnostics[i].Code == code && sf.Diagnostics[i].Message == message {
			sf.Diagnostics[i].Count++
			return
		}
	}
	if len(sf.Diagnostics) < 128 {
		sf.Diagnostics = append(sf.Diagnostics, Diagnostic{code, message, 1})
		return
	}
	for i := range sf.Diagnostics {
		if sf.Diagnostics[i].Code == "suppressed" {
			sf.Diagnostics[i].Count++
			return
		}
	}
	sf.Diagnostics[127] = Diagnostic{"suppressed", "additional diagnostics suppressed", 1}
}
