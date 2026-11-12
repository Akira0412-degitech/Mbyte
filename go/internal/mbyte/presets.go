package mbyte

import (
	"fmt"
	"strings"
)

// This is a faithful port of mbyte/presets.py: named insert presets and their
// default output extensions. Step 3 will unify these into the detection
// signature table; until then they live here to keep the cli.py port 1:1.

// preset is one named magic-byte preset (name -> hex string).
type preset struct {
	Name string
	Hex  string
}

// presetsOrdered preserves the Python dict's insertion order so that
// ListPresets and the "valid presets: ..." error list match byte-for-byte.
var presetsOrdered = []preset{
	{"png", "89504e470d0a1a0a"},
	{"gif89a", "474946383961"},
	{"gif87a", "474946383761"},
	{"jpeg", "ffd8ffe0"},
	{"pdf", "25504446"},
	{"zip", "504b0304"},
}

// presetExtensions maps a preset name to the default output suffix (no dot)
// used when --preset is given and -o/--output is omitted.
var presetExtensions = map[string]string{
	"png":    "png",
	"gif89a": "gif",
	"gif87a": "gif",
	"jpeg":   "jpg",
	"pdf":    "pdf",
	"zip":    "zip",
}

// PresetHex returns the hex string for a preset name, and ok=false if unknown.
func PresetHex(name string) (hex string, ok bool) {
	for _, p := range presetsOrdered {
		if p.Name == name {
			return p.Hex, true
		}
	}
	return "", false
}

// PresetExtension returns the default output extension for a preset name, and
// ok=false if the name has no mapping.
func PresetExtension(name string) (ext string, ok bool) {
	ext, ok = presetExtensions[name]
	return ext, ok
}

// presetNames returns the preset names in declaration order (used for the
// unknown-preset error message, matching Python's ", ".join(PRESETS)).
func presetNames() []string {
	names := make([]string, len(presetsOrdered))
	for i, p := range presetsOrdered {
		names[i] = p.Name
	}
	return names
}

// ListPresets returns a human-readable table of available presets, byte-for-byte
// identical to Python's presets.list_presets().
func ListPresets() string {
	width := 0
	for _, p := range presetsOrdered {
		if len(p.Name) > width {
			width = len(p.Name)
		}
	}
	lines := []string{
		"Available presets:",
		"",
		fmt.Sprintf("  %s  HEX", pad("NAME", width)),
		fmt.Sprintf("  %s  %s", strings.Repeat("-", width), strings.Repeat("-", 16)),
	}
	for _, p := range presetsOrdered {
		lines = append(lines, fmt.Sprintf("  %s  %s", pad(p.Name, width), p.Hex))
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
