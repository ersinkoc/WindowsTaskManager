//go:build windows

package winapi

import (
	"encoding/binary"
	"testing"
	"unicode/utf16"
	"unsafe"
)

// The offsets exercised below are the Windows SDK netioapi.h MIB_IF_ROW2
// layout, verified with offsetof against the real headers (x64). Decoy
// markers sit in the neighbouring slots (AdminStatus next to OperStatus,
// InUcastPkts next to InOctets, InNUcastPkts next to InUcastPkts,
// InMulticastOctets where a previous bug read InErrors, OutMulticastOctets
// where it read OutErrors) so a single-slot drift fails loudly.

func ifRow2PutU32(b []byte, off int, v uint32) { binary.LittleEndian.PutUint32(b[off:], v) }
func ifRow2PutU64(b []byte, off int, v uint64) { binary.LittleEndian.PutUint64(b[off:], v) }

func ifRow2PutUTF16(b []byte, off int, s string) {
	for i, r := range utf16.Encode([]rune(s)) {
		binary.LittleEndian.PutUint16(b[off+2*i:], r)
	}
	// Terminating NUL is already zero in the fresh buffer.
}

func buildIfRow2Buffer() []byte {
	raw := make([]byte, ifRow2Size)

	ifRow2PutU32(raw, 8, 0x11223344)             // InterfaceIndex
	ifRow2PutUTF16(raw, 28, "Marker-A")          // Alias
	ifRow2PutUTF16(raw, 542, "Marker Desc NIC")  // Description
	ifRow2PutU32(raw, 1124, 1500)                // Mtu
	ifRow2PutU32(raw, 1128, 6)                   // Type (ethernetCsmacd)
	ifRow2PutU32(raw, 1156, 5)                   // OperStatus (dormant)
	ifRow2PutU32(raw, 1160, 2)                   // AdminStatus (down) — decoy for OperStatus
	ifRow2PutU64(raw, 1192, 111_111_111)         // TransmitLinkSpeed
	ifRow2PutU64(raw, 1200, 222_222_222)         // ReceiveLinkSpeed — distinct from Tx
	ifRow2PutU64(raw, 1208, 1_000_000_001)       // InOctets
	ifRow2PutU64(raw, 1216, 1_000_000_002)       // InUcastPkts
	ifRow2PutU64(raw, 1224, 1_000_000_003)       // InNUcastPkts — decoy for InUcastPkts
	ifRow2PutU64(raw, 1240, 77)                  // InErrors
	ifRow2PutU64(raw, 1264, 123_456_789)         // InMulticastOctets — decoy for InErrors
	ifRow2PutU64(raw, 1280, 2_000_000_001)       // OutOctets
	ifRow2PutU64(raw, 1288, 2_000_000_002)       // OutUcastPkts
	ifRow2PutU64(raw, 1312, 76)                  // OutErrors
	ifRow2PutU64(raw, 1328, 987_654_321)         // OutMulticastOctets — decoy for OutErrors
	ifRow2PutU64(raw, 1344, 3)                   // OutQLen — last field of the row

	return raw
}

func TestParseIfRow2SDKLayout(t *testing.T) {
	raw := buildIfRow2Buffer()
	row := parseIfRow2(unsafe.Pointer(&raw[0]))

	if got, want := row.Index, uint32(0x11223344); got != want {
		t.Errorf("Index = %#x, want %#x", got, want)
	}
	if got, want := row.Alias, "Marker-A"; got != want {
		t.Errorf("Alias = %q, want %q", got, want)
	}
	if got, want := row.Description, "Marker Desc NIC"; got != want {
		t.Errorf("Description = %q, want %q", got, want)
	}
	if got, want := row.Type, uint32(6); got != want {
		t.Errorf("Type = %d, want %d", got, want)
	}
	if got, want := row.OperStatus, uint32(5); got != want {
		t.Errorf("OperStatus = %d, want %d (reading AdminStatus instead would yield 2)", got, want)
	}
	if got, want := row.SpeedTx, uint64(111_111_111); got != want {
		t.Errorf("SpeedTx = %d, want %d", got, want)
	}
	if got, want := row.SpeedRx, uint64(222_222_222); got != want {
		t.Errorf("SpeedRx = %d, want %d", got, want)
	}
	if got, want := row.InOctets, uint64(1_000_000_001); got != want {
		t.Errorf("InOctets = %d, want %d (reading InUcastPkts instead would yield %d)", got, want, uint64(1_000_000_002))
	}
	if got, want := row.InUcastPkts, uint64(1_000_000_002); got != want {
		t.Errorf("InUcastPkts = %d, want %d (reading InNUcastPkts instead would yield %d)", got, want, uint64(1_000_000_003))
	}
	if got, want := row.InErrors, uint64(77); got != want {
		t.Errorf("InErrors = %d, want %d (reading InMulticastOctets instead would yield %d)", got, want, uint64(123_456_789))
	}
	if got, want := row.OutOctets, uint64(2_000_000_001); got != want {
		t.Errorf("OutOctets = %d, want %d", got, want)
	}
	if got, want := row.OutUcastPkts, uint64(2_000_000_002); got != want {
		t.Errorf("OutUcastPkts = %d, want %d", got, want)
	}
	if got, want := row.OutErrors, uint64(76); got != want {
		t.Errorf("OutErrors = %d, want %d (reading OutMulticastOctets instead would yield %d)", got, want, uint64(987_654_321))
	}
}

// TestIfRow2BufferSize pins the row stride used when walking the
// MIB_IF_TABLE2 buffer. sizeof(MIB_IF_ROW2) == 1352 on x64 per the SDK.
func TestIfRow2BufferSize(t *testing.T) {
	if ifRow2Size != 1352 {
		t.Errorf("ifRow2Size = %d, want 1352 (sizeof(MIB_IF_ROW2) on x64)", ifRow2Size)
	}
}
