//go:build windows

package tray

import (
	"strings"
	"testing"
)

// TestCopyToFixedAlwaysTerminates pins the Win32 fixed-buffer contract:
// copyToFixed must leave a NUL terminator in the destination for every
// input length. NOTIFYICONDATAW.szInfo is a [256]uint16 array; before the
// fix, a source longer than the buffer truncated to exactly len(dst)
// without any terminator, so Shell_NotifyIconW scanned past szInfo into
// szInfoTitle / the union while looking for the end of the string.
func TestCopyToFixedAlwaysTerminates(t *testing.T) {
	hasNUL := func(buf []uint16) bool {
		for _, c := range buf {
			if c == 0 {
				return true
			}
		}
		return false
	}

	cases := []struct {
		name  string
		input string
	}{
		{"empty", ""},
		{"short", "abc"},
		{"exact fit with own terminator", strings.Repeat("y", 255)}, // 255 chars + NUL = 256
		{"one past exact fit", strings.Repeat("x", 256)},            // truncation branch begins
		{"well past buffer", strings.Repeat("z", 400)},
	}
	for _, tc := range cases {
		var buf [256]uint16
		copyToFixed(buf[:], tc.input)
		if !hasNUL(buf[:]) {
			t.Errorf("%s: buffer left unterminated", tc.name)
		}
	}

	// Truncation keeps at most 255 visible characters before the
	// terminator (the last slot is reserved).
	var buf [256]uint16
	copyToFixed(buf[:], strings.Repeat("a", 400))
	visible := 0
	for _, c := range buf {
		if c == 0 {
			break
		}
		visible++
	}
	if visible != 255 {
		t.Errorf("truncated copy shows %d visible chars, want 255 (last slot reserved for the terminator)", visible)
	}

	// Non-truncated content is preserved verbatim.
	var small [8]uint16
	copyToFixed(small[:], "abc")
	if small[0] != 'a' || small[1] != 'b' || small[2] != 'c' || small[3] != 0 {
		t.Errorf("short content not preserved verbatim: %v", small[:4])
	}
}
