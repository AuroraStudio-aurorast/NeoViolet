//go:build darwin

package mediactl

import (
	"bytes"
	"image"
	"image/png"
	"unsafe"

	"github.com/ebitengine/purego/objc"
)

// buildArtwork converts cover → PNG → NSImage → MPMediaItemArtwork.
// Caches by image identity — re-encoding only happens when the cover
// image object actually changes, not on every Update() tick.
// Caller MUST be inside an autorelease pool.

func (c *darwinCtrl) buildArtwork(cover image.Image) objc.ID {
	if cover == nil {
		return 0
	}

	// Fast path: same image object as last time — reuse cached artwork.
	if cover == c.lastCoverImg && c.coverArtwork != 0 {
		return c.coverArtwork
	}

	// Encode to PNG (only when cover has changed).
	var buf bytes.Buffer
	if err := png.Encode(&buf, cover); err != nil {
		return 0
	}
	pngBytes := buf.Bytes()
	if len(pngBytes) == 0 {
		return 0
	}

	// Release previous artwork
	if c.coverArtwork != 0 {
		c.coverArtwork.Send(selRelease)
		c.coverArtwork = 0
	}

	// #nosec G103 -- ObjC interop requires passing a raw pointer to the PNG
	// bytes; the buffer is in-process and length-bounded by len(pngBytes).
	nsData := objc.ID(classNSData).Send(selDataWithBytes, uintptr(unsafe.Pointer(&pngBytes[0])), uint(len(pngBytes)))
	nsImage := objc.ID(classNSImage).Send(selAlloc).Send(selInitWithData, nsData)
	artwork := objc.ID(classMPMediaItemArtwork).Send(selAlloc).Send(selInitWithImage, nsImage)
	nsImage.Send(selRelease)

	c.lastCoverImg = cover
	c.coverArtwork = artwork
	return artwork
}

func (c *darwinCtrl) Update(state PlayState) {
	if state.Title == "" {
		return
	}

	c.mu.Lock()
	if c.closed || c.nowPlaying == 0 {
		c.mu.Unlock()
		return
	}
	np := c.nowPlaying
	c.mu.Unlock()

	autoPool(func() {
		dict := nsMutableDict()
		dur := state.Duration.Seconds()
		pos := state.Position.Seconds()
		prog := 0.0
		if dur > 0 {
			prog = pos / dur
		}

		dictSetKV(dict, nsString("MPNowPlayingInfoPropertyElapsedPlaybackTime"), nsDouble(pos))
		dictSetKV(dict, nsString("MPNowPlayingInfoPropertyPlaybackRate"), nsDouble(1.0))
		dictSetKV(dict, nsString("MPNowPlayingInfoPropertyDefaultPlaybackRate"), nsDouble(1.0))
		dictSetKV(dict, nsString("MPNowPlayingInfoPropertyPlaybackProgress"), nsDouble(prog))
		dictSetKV(dict, nsString("MPNowPlayingInfoPropertyMediaType"), nsInt(1))
		dictSetKV(dict, nsString("persistentID"), nsInt(1))
		dictSetKV(dict, nsString("title"), nsString(state.Title))
		dictSetKV(dict, nsString("artist"), nsString(state.Artist))
		dictSetKV(dict, nsString("albumTitle"), nsString(state.Album))
		dictSetKV(dict, nsString("albumArtist"), nsString(state.Artist))
		dictSetKV(dict, nsString("playbackDuration"), nsDouble(dur))
		dictSetKV(dict, nsString("mediaType"), nsInt(1))

		if art := c.buildArtwork(state.Cover); art != 0 {
			dictSetKV(dict, nsString("artwork"), art)
		}

		st := playbackStatePaused
		if state.Playing {
			st = playbackStatePlaying
		}
		np.Send(selSetPlaybackState, st)
		np.Send(selSetNowPlayingInfo, dict)

		// Re-register commands after SetNowPlayingInfo — macOS may
		// invalidate previous registrations (observed on macOS 26+).
		c.mu.Lock()
		if !c.closed {
			c.registerCommands()
		}
		c.mu.Unlock()
	})
}
