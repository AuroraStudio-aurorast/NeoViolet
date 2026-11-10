package midi

import (
	"errors"
	"math"
	"math/bits"
	"time"
)

func timingRatio(division uint16, rate int32) (uint64, uint64, error) {
	if rate <= 0 || rate > 384000 {
		return 0, 0, errors.New("invalid sample rate")
	}
	if division&0x8000 == 0 {
		if division == 0 {
			return 0, 0, errors.New("zero PPQN")
		}
		return uint64(rate) * 500000, uint64(division) * 1000000, nil
	}
	// #nosec G115 -- sample rate is validated positive and bounded by timingRatio; SMPTE high byte intentionally encodes a signed frame rate.
	fps := int8(division >> 8)
	sub := uint64(division & 255)
	if sub == 0 {
		return 0, 0, errors.New("zero SMPTE subframe")
	}
	switch fps {
	case -24, -25, -30:
		return uint64(rate), uint64(-int(fps)) * sub, nil
	case -29:
		return uint64(rate) * 1001, 30000 * sub, nil
	}
	return 0, 0, errors.New("invalid SMPTE frame rate")
}

// Compile integrates tempo using integer quotient/remainder rather than truncating
// every tick. The DSP observes events at the next 64-frame boundary.
func (s *Song) Compile(rate int32) (*Timeline, error) {
	numerator, denominator, err := timingRatio(s.division, rate)
	if err != nil {
		return nil, err
	}
	t := &Timeline{SampleRate: rate, Events: make([]TimedEvent, 0, len(s.Events))}
	var tick, frame, remainder uint64
	// #nosec G115 -- sample rate is validated positive and bounded by timingRatio; SMPTE high byte intentionally encodes a signed frame rate.
	maxFrame := uint64(math.MaxInt64/int64(time.Second)) * uint64(rate)
	advance := func(next uint64) error {
		if next < tick {
			return errors.New("unordered timeline")
		}
		hi, lo := bits.Mul64(next-tick, numerator)
		lo, carry := bits.Add64(lo, remainder, 0)
		hi += carry
		if hi >= denominator {
			return errors.New("timeline overflow")
		}
		q, r := bits.Div64(hi, lo, denominator)
		if q > maxFrame-frame {
			return errors.New("timeline duration overflow")
		}
		frame += q
		remainder = r
		tick = next
		return nil
	}
	ei, ti := 0, 0
	for ei < len(s.Events) || ti < len(s.tempos) {
		next := s.end
		if ei < len(s.Events) {
			next = s.Events[ei].Tick
		}
		if ti < len(s.tempos) {
			next = min(next, s.tempos[ti].tick)
		}
		if err = advance(next); err != nil {
			return nil, err
		}
		for ti < len(s.tempos) && s.tempos[ti].tick == next {
			if s.division&0x8000 == 0 {
				// #nosec G115 -- sample rate is validated positive and bounded by timingRatio; SMPTE high byte intentionally encodes a signed frame rate.
				numerator = uint64(s.tempos[ti].tempo) * uint64(rate)
			}
			ti++
		}
		for ei < len(s.Events) && s.Events[ei].Tick == next {
			t.Events = append(t.Events, TimedEvent{int64(frame), int64((frame + 63) / 64 * 64), s.Events[ei]})
			ei++
		}
	}
	if err = advance(s.end); err != nil {
		return nil, err
	}
	t.ContentFrames = int64(frame)
	if frame != 0 || len(s.Events) > 0 {
		// #nosec G115 -- sample rate is validated positive and bounded by timingRatio; SMPTE high byte intentionally encodes a signed frame rate.
		if frame+uint64(2*rate) > maxFrame {
			return nil, errors.New("tail duration overflow")
		}
		t.TotalFrames = int64(frame) + int64(2*rate)
	}
	return t, nil
}

// FramesDuration avoids multiplication overflow for API positions.
func FramesDuration(frame int64, rate int32) time.Duration {
	if frame <= 0 || rate <= 0 {
		return 0
	}
	q, r := frame/int64(rate), frame%int64(rate)
	if q > math.MaxInt64/int64(time.Second) {
		return time.Duration(math.MaxInt64)
	}
	ns := q * int64(time.Second)
	tail := r * int64(time.Second) / int64(rate)
	if ns > math.MaxInt64-tail {
		return time.Duration(math.MaxInt64)
	}
	return time.Duration(ns + tail)
}

// DurationFrames clamps before multiplication, including arbitrary media requests.
func DurationFrames(pos time.Duration, rate int32, total int64) int64 {
	if pos <= 0 || rate <= 0 {
		return 0
	}
	if pos >= FramesDuration(total, rate) {
		return total
	}
	seconds, ns := int64(pos/time.Second), int64(pos%time.Second)
	if seconds > total/int64(rate) {
		return total
	}
	return min(total, seconds*int64(rate)+ns*int64(rate)/int64(time.Second))
}
