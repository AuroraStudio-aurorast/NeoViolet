package soundfont

import (
	"errors"
)

// Instrument contains the regions of a named SF2 instrument.
type Instrument struct {
	Name    string
	Regions []*InstrumentRegion
}

func createInstrument(info *instrumentInfo, zones []*zone, samples []*SampleHeader) (*Instrument, error) {
	var err error

	result := new(Instrument)
	result.Name = info.name

	zoneCount := info.zoneEndIndex - info.zoneStartIndex + 1
	if zoneCount == 0 {
		return result, nil
	}
	if zoneCount < 0 || info.zoneStartIndex < 0 || int64(info.zoneStartIndex)+int64(zoneCount) > int64(len(zones)) {
		return nil, errors.New("invalid instrument zone range")
	}

	zoneSpan := zones[info.zoneStartIndex : info.zoneStartIndex+zoneCount]

	result.Regions, err = createInstrumentRegions(result, zoneSpan, samples)
	if err != nil {
		return nil, err
	}

	return result, nil
}

func createInstruments(infos []*instrumentInfo, zones []*zone, samples []*SampleHeader) ([]*Instrument, error) {

	var err error

	if len(infos) <= 1 {
		return nil, errors.New("no valid instrument was found")
	}

	// The last one is the terminator.
	count := len(infos) - 1

	instruments := make([]*Instrument, count)

	for i := 0; i < count; i++ {
		instruments[i], err = createInstrument(infos[i], zones, samples)
		if err != nil {
			return nil, err
		}
	}

	return instruments, nil
}
