package mbyte

import (
	"encoding/hex"
	"fmt"
	"strings"
)

// Step 3 unified the insert presets into the single `signatures` table in
// core.go (replacing presets.py's separate PRESETS / PRESET_EXTENSIONS dicts).
// The helpers below are now a thin projection over that table: an entry is an
// insert preset iff PresetName != "". This keeps detection and preset data in
// one source of truth so they can't drift apart.

// PresetHex returns the hex string an insert writes for a preset name, and
// ok=false if the name is unknown. It is the entry's PresetHex, or the hex of
// its detection prefix when PresetHex is unset (they match for every preset
// except jpeg).
func PresetHex(name string) (hexStr string, ok bool) {
	for _, s := range signatures {
		if s.PresetName == name {
			if s.PresetHex != "" {
				return s.PresetHex, true
			}
			return hex.EncodeToString(s.Hex), true
		}
	}
	return "", false
}

// PresetExtension returns the default output extension for a preset name, and
// ok=false if the name is unknown (or the entry has no extension set).
func PresetExtension(name string) (ext string, ok bool) {
	for _, s := range signatures {
		if s.PresetName == name {
			return s.Ext, s.Ext != ""
		}
	}
	return "", false
}

// presetNames returns the preset names in table order (used for the
// unknown-preset error message, mirroring Python's ", ".join(PRESETS)).
func presetNames() []string {
	var names []string
	for _, s := range signatures {
		if s.PresetName != "" {
			names = append(names, s.PresetName)
		}
	}
	return names
}

// ListPresets returns a human-readable table of available presets. Only entries
// with a PresetName (insert presets) are listed; detection-only signatures such
// as BMP are excluded. Format matches Python's presets.list_presets(): the NAME
// column is width-aligned to the longest name and the HEX column shows the hex
// an insert actually writes.
func ListPresets() string {
	type row struct{ name, hex string }
	var rows []row
	width := 0
	for _, s := range signatures {
		if s.PresetName == "" {
			continue
		}
		h := s.PresetHex
		if h == "" {
			h = hex.EncodeToString(s.Hex)
		}
		rows = append(rows, row{s.PresetName, h})
		if len(s.PresetName) > width {
			width = len(s.PresetName)
		}
	}

	lines := []string{
		"Available presets:",
		"",
		fmt.Sprintf("  %s  HEX", pad("NAME", width)),
		fmt.Sprintf("  %s  %s", strings.Repeat("-", width), strings.Repeat("-", 16)),
	}
	for _, r := range rows {
		lines = append(lines, fmt.Sprintf("  %s  %s", pad(r.name, width), r.hex))
	}
	return strings.Join(lines, "\n")
}

// pad left-justifies s to width (Python str.ljust).
func pad(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}
