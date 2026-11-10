package ui

// listViewport retains scroll position separately from the selected item.
// The visible window moves only when its cursor leaves the window or its size changes.
type listViewport struct{ top int }

func (v listViewport) start(cursor, count, capacity int) int {
	capacity = max(1, capacity)
	if count <= 0 {
		return 0
	}
	cursor = max(0, min(cursor, count-1))
	top := max(0, min(v.top, max(0, count-capacity)))
	if cursor < top {
		return cursor
	}
	if cursor >= top+capacity {
		return cursor - capacity + 1
	}
	return top
}
func (v *listViewport) reveal(cursor, count, capacity int) { v.top = v.start(cursor, count, capacity) }

// Capacities use the same content-row budgets in navigation and rendering.
func listCapacity(height, header, footer, stride int) int {
	return max(1, (height-header-footer)/max(1, stride))
}
func playlistStride(height int) int {
	if height < 14 {
		return 1
	}
	return 3
}
func savedTrackTop(height int) int {
	if height < 14 {
		return 5
	}
	return 6
}

type pageViewports struct {
	home, scopes, queue, collections, tracks, settings, roots, formats listViewport
	collectionID                                                       string
}

// Synchronization belongs to Update; View reads the retained window without
// changing it, including after asynchronous data changes and terminal resize.
func (m *Model) syncListViewports() {
	if m.pages == nil || m.UI == nil || m.UI.Width < minWidth || m.UI.Height < minHeight {
		return
	}
	p := m.pages
	h := m.layoutPlan().ContentInnerH
	if b := p.browser; b != nil {
		b.viewport.reveal(b.cursor, len(b.entries), listCapacity(h, 4, 2, 1))
		return
	}
	if p.rootEditor {
		p.viewports.roots.reveal(p.rootCursor, len(p.draft.Library.Roots), listCapacity(h, 4, 1, 1))
		return
	}
	if p.picker != "" {
		p.viewports.formats.reveal(p.pickerIndex, len(p.pickerItems()), listCapacity(h, 4, 1, 1))
		return
	}
	switch m.UI.ActiveTab {
	case 0:
		p.viewports.home.reveal(p.cursors[0], len(p.visible), listCapacity(h, 4, 1, 2))
		p.viewports.scopes.reveal(p.scope, len(p.scopes), max(1, min(10, h-8)))
	case 1:
		if !p.saved {
			p.viewports.queue.reveal(p.cursors[1], len(m.session.entries), listCapacity(h, 4, 1, playlistStride(h)))
			return
		}
		p.viewports.collections.reveal(p.collection, len(m.playlists.lists), listCapacity(h, 4, 1, playlistStride(h)))
		if list := m.selectedPlaylist(); list != nil {
			if p.viewports.collectionID != list.ID {
				p.viewports.tracks.top = 0
				p.viewports.collectionID = list.ID
			}
			p.viewports.tracks.reveal(p.savedTrack, len(list.Tracks), listCapacity(h, savedTrackTop(h), 3, 1))
		} else {
			p.viewports.tracks.top = 0
			p.viewports.collectionID = ""
		}
	case 3:
		rows, cursor := p.settingRows()
		p.viewports.settings.top = p.settingsWindowStart(rows, cursor, listCapacity(h, 3, 1, 1))
	}
}
