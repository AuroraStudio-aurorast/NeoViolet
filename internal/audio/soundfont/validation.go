package soundfont

import "fmt"

func effectiveAddress(base int32, fine, coarse int16) int64 {
	return int64(base) + int64(fine) + 32768*int64(coarse)
}

func validateRegions(sf *SoundFont) {
	for _, inst := range sf.Instruments {
		for _, r := range inst.Regions {
			h := r.Sample
			start := effectiveAddress(h.Start, r.gs[genStartAddressOffset], r.gs[genStartAddressCoarseOffset])
			end := effectiveAddress(h.End, r.gs[genEndAddressOffset], r.gs[genEndAddressCoarseOffset])
			ls := effectiveAddress(h.StartLoop, r.gs[genStartLoopAddressOffset], r.gs[genStartLoopAddressCoarseOffset])
			le := effectiveAddress(h.EndLoop, r.gs[genEndLoopAddressOffset], r.gs[genEndLoopAddressCoarseOffset])
			r.valid = start >= 0 && start < end && end <= int64(len(sf.WaveData)) && h.SampleRate > 0 && h.SampleType&0x8000 == 0
			if !r.valid {
				sf.diagnose("invalid_sample", fmt.Sprintf("%s: %s", inst.Name, h.Name))
				continue
			}
			// #nosec G115 -- validated sample addresses fit the 1 GiB PCM bound; sample link identity is a 16-bit SF2 index.
			r.start, r.end = int32(start), int32(end)
			r.loopStart, r.loopEnd = r.start, r.end
			if start <= ls && ls < le && le <= end {
				// #nosec G115 -- validated sample addresses fit the 1 GiB PCM bound; sample link identity is a 16-bit SF2 index.
				r.loopStart, r.loopEnd = int32(ls), int32(le)
			} else {
				if r.getSampleModes() != loopNoLoop {
					sf.diagnose("invalid_loop", fmt.Sprintf("%s: %s; one-shot", inst.Name, h.Name))
				}
				r.gs[genSampleModes] = 0
			}
			if h.SampleType&6 != 0 {
				// #nosec G115 -- validated sample addresses fit the 1 GiB PCM bound; sample link identity is a 16-bit SF2 index.
				if int(h.Link) >= len(sf.SampleHeaders) || sf.SampleHeaders[h.Link].Link != uint16(indexOfSample(sf.SampleHeaders, h)) {
					sf.diagnose("invalid_stereo_link", h.Name)
				}
			}
		}
	}
}
func indexOfSample(samples []*SampleHeader, h *SampleHeader) int {
	for i, s := range samples {
		if s == h {
			return i
		}
	}
	return -1
}
