package replacement

import (
	"sync"
)

type Key struct {
	AssetID   int64
	SlotIndex int
}

type Entry struct {
	AssetID  int64
	FilePath string
	Remove   bool
}

func (e Entry) IsFile() bool { return e.FilePath != "" }
func (e Entry) IsID() bool {
	return !e.Remove && e.FilePath == "" && e.AssetID > 0
}
func (e Entry) IsRemove() bool { return e.Remove }

func IDEntry(id int64) Entry { return Entry{AssetID: id} }
func RemoveEntry() Entry     { return Entry{Remove: true} }

type Map struct {
	mu      sync.RWMutex
	entries map[Key]Entry
}

func NewMap() *Map {
	return &Map{
		entries: make(map[Key]Entry),
	}
}

func (m *Map) Swap(next map[Key]Entry) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make(map[Key]Entry, len(next))
	for k, v := range next {
		cp[k] = v
	}
	m.entries = cp
}

func (m *Map) Lookup(assetID int64, slotDecoded *int) (Entry, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.entries) == 0 {
		return Entry{}, false
	}
	if slotDecoded != nil && *slotDecoded >= 0 {
		slot := *slotDecoded
		if e, ok := m.entries[Key{AssetID: assetID, SlotIndex: slot}]; ok {
			return e, true
		}
		// roblox encode fidelity as slot+32; normalize so pack slot keys still match
		if slot >= 32 {
			if e, ok := m.entries[Key{AssetID: assetID, SlotIndex: slot - 32}]; ok {
				return e, true
			}
		}
	}
	if e, ok := m.entries[Key{AssetID: assetID, SlotIndex: -1}]; ok {
		return e, true
	}
	return Entry{}, false
}

func (m *Map) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.entries)
}

func (m *Map) Snapshot() map[Key]Entry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[Key]Entry, len(m.entries))
	for k, v := range m.entries {
		out[k] = v
	}
	return out
}
