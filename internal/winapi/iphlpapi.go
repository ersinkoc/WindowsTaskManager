//go:build windows

package winapi

import (
	"fmt"
	"unsafe"
)

// GetExtendedTcpTable wraps the Win32 call. Returns raw byte buffer; caller parses based on family.
func getExtendedTcpTableRaw(family uint32) ([]byte, error) {
	var size uint32
	r1, _, e := procGetExtendedTcpTable.Call(
		0,
		uintptr(unsafe.Pointer(&size)), // #nosec G103 -- Audited Win32 unsafe interop.
		1,                              // sorted = true
		uintptr(family),
		uintptr(TCP_TABLE_OWNER_PID_ALL),
		0,
	)
	if r1 != 0 && r1 != 122 { // ERROR_INSUFFICIENT_BUFFER
		return nil, fmt.Errorf("GetExtendedTcpTable(size): status %d (%w)", r1, e)
	}
	if size == 0 {
		return nil, nil
	}
	buf := make([]byte, size)
	r1, _, e = procGetExtendedTcpTable.Call(
		uintptr(unsafe.Pointer(&buf[0])), // #nosec G103 -- Audited Win32 unsafe interop.
		uintptr(unsafe.Pointer(&size)),   // #nosec G103 -- Audited Win32 unsafe interop.
		1,
		uintptr(family),
		uintptr(TCP_TABLE_OWNER_PID_ALL),
		0,
	)
	if r1 != 0 {
		return nil, fmt.Errorf("GetExtendedTcpTable: status %d (%w)", r1, e)
	}
	if uint64(size) > uint64(len(buf)) {
		return nil, fmt.Errorf("GetExtendedTcpTable: returned size %d exceeds buffer %d", size, len(buf))
	}
	return buf[:int(size)], nil
}

// GetTcp4Table returns IPv4 TCP rows with owning PIDs.
func GetTcp4Table() ([]MIB_TCPROW_OWNER_PID, error) {
	buf, err := getExtendedTcpTableRaw(AF_INET)
	if err != nil || len(buf) < 4 {
		return nil, err
	}
	count := *(*uint32)(unsafe.Pointer(&buf[0])) // #nosec G103 -- Audited Win32 unsafe interop.
	rowSize := unsafe.Sizeof(MIB_TCPROW_OWNER_PID{})
	rows := make([]MIB_TCPROW_OWNER_PID, 0, count)
	for i := uint32(0); i < count; i++ {
		offset := uintptr(4) + uintptr(i)*rowSize
		if offset+rowSize > uintptr(len(buf)) {
			return nil, fmt.Errorf("GetTcp4Table: truncated buffer at row %d/%d", i, count)
		}
		rows = append(rows, *(*MIB_TCPROW_OWNER_PID)(unsafe.Pointer(&buf[offset]))) // #nosec G103 -- Audited Win32 unsafe interop.
	}
	return rows, nil
}

// GetTcp6Table returns IPv6 TCP rows with owning PIDs.
func GetTcp6Table() ([]MIB_TCP6ROW_OWNER_PID, error) {
	buf, err := getExtendedTcpTableRaw(AF_INET6)
	if err != nil || len(buf) < 4 {
		return nil, err
	}
	count := *(*uint32)(unsafe.Pointer(&buf[0])) // #nosec G103 -- Audited Win32 unsafe interop.
	rowSize := unsafe.Sizeof(MIB_TCP6ROW_OWNER_PID{})
	rows := make([]MIB_TCP6ROW_OWNER_PID, 0, count)
	for i := uint32(0); i < count; i++ {
		offset := uintptr(4) + uintptr(i)*rowSize
		if offset+rowSize > uintptr(len(buf)) {
			return nil, fmt.Errorf("GetTcp6Table: truncated buffer at row %d/%d", i, count)
		}
		rows = append(rows, *(*MIB_TCP6ROW_OWNER_PID)(unsafe.Pointer(&buf[offset]))) // #nosec G103 -- Audited Win32 unsafe interop.
	}
	return rows, nil
}

func getExtendedUdpTableRaw(family uint32) ([]byte, error) {
	var size uint32
	r1, _, e := procGetExtendedUdpTable.Call(
		0,
		uintptr(unsafe.Pointer(&size)), // #nosec G103 -- Audited Win32 unsafe interop.
		1,
		uintptr(family),
		uintptr(UDP_TABLE_OWNER_PID),
		0,
	)
	if r1 != 0 && r1 != 122 { // ERROR_INSUFFICIENT_BUFFER
		return nil, fmt.Errorf("GetExtendedUdpTable(size): status %d (%w)", r1, e)
	}
	if size == 0 {
		return nil, nil
	}
	buf := make([]byte, size)
	r1, _, e = procGetExtendedUdpTable.Call(
		uintptr(unsafe.Pointer(&buf[0])), // #nosec G103 -- Audited Win32 unsafe interop.
		uintptr(unsafe.Pointer(&size)),   // #nosec G103 -- Audited Win32 unsafe interop.
		1,
		uintptr(family),
		uintptr(UDP_TABLE_OWNER_PID),
		0,
	)
	if r1 != 0 {
		return nil, fmt.Errorf("GetExtendedUdpTable: status %d (%w)", r1, e)
	}
	if uint64(size) > uint64(len(buf)) {
		return nil, fmt.Errorf("GetExtendedUdpTable: returned size %d exceeds buffer %d", size, len(buf))
	}
	return buf[:int(size)], nil
}

// GetUdp4Table returns IPv4 UDP rows with owning PIDs.
func GetUdp4Table() ([]MIB_UDPROW_OWNER_PID, error) {
	buf, err := getExtendedUdpTableRaw(AF_INET)
	if err != nil || len(buf) < 4 {
		return nil, err
	}
	count := *(*uint32)(unsafe.Pointer(&buf[0])) // #nosec G103 -- Audited Win32 unsafe interop.
	rowSize := unsafe.Sizeof(MIB_UDPROW_OWNER_PID{})
	rows := make([]MIB_UDPROW_OWNER_PID, 0, count)
	for i := uint32(0); i < count; i++ {
		offset := uintptr(4) + uintptr(i)*rowSize
		if offset+rowSize > uintptr(len(buf)) {
			return nil, fmt.Errorf("GetUdp4Table: truncated buffer at row %d/%d", i, count)
		}
		rows = append(rows, *(*MIB_UDPROW_OWNER_PID)(unsafe.Pointer(&buf[offset]))) // #nosec G103 -- Audited Win32 unsafe interop.
	}
	return rows, nil
}

// GetUdp6Table returns IPv6 UDP rows with owning PIDs.
func GetUdp6Table() ([]MIB_UDP6ROW_OWNER_PID, error) {
	buf, err := getExtendedUdpTableRaw(AF_INET6)
	if err != nil || len(buf) < 4 {
		return nil, err
	}
	count := *(*uint32)(unsafe.Pointer(&buf[0])) // #nosec G103 -- Audited Win32 unsafe interop.
	rowSize := unsafe.Sizeof(MIB_UDP6ROW_OWNER_PID{})
	rows := make([]MIB_UDP6ROW_OWNER_PID, 0, count)
	for i := uint32(0); i < count; i++ {
		offset := uintptr(4) + uintptr(i)*rowSize
		if offset+rowSize > uintptr(len(buf)) {
			return nil, fmt.Errorf("GetUdp6Table: truncated buffer at row %d/%d", i, count)
		}
		rows = append(rows, *(*MIB_UDP6ROW_OWNER_PID)(unsafe.Pointer(&buf[offset]))) // #nosec G103 -- Audited Win32 unsafe interop.
	}
	return rows, nil
}

// MIB_IF_ROW2 (partial â€” we read into a fixed-size raw buffer and parse manually).
const ifRow2Size = 1352

// IfRow2 holds the fields we care about parsed from MIB_IF_ROW2.
type IfRow2 struct {
	Index        uint32
	Alias        string
	Description  string
	OperStatus   uint32
	Type         uint32
	SpeedRx      uint64
	SpeedTx      uint64
	InOctets     uint64
	OutOctets    uint64
	InUcastPkts  uint64
	OutUcastPkts uint64
	InErrors     uint64
	OutErrors    uint64
}

// GetIfTable2 returns the system network interface table.
// We let the Win32 API allocate a MIB_IF_TABLE2 buffer, walk the rows at
// their documented offsets, then free the buffer via FreeMibTable.
//
// tablePtr is declared as unsafe.Pointer (not uintptr) so vet's
// unsafeptr analyzer is satisfied: we never do arithmetic on a uintptr
// and convert it back â€” all offsetting goes through unsafe.Add.
func GetIfTable2() ([]IfRow2, error) {
	var tablePtr unsafe.Pointer
	r1, _, e := procGetIfTable2.Call(uintptr(unsafe.Pointer(&tablePtr))) // #nosec G103 -- Audited Win32 unsafe interop.
	if r1 != 0 {
		return nil, fmt.Errorf("GetIfTable2: %w", e)
	}
	if tablePtr == nil {
		return nil, nil
	}
	defer procFreeMibTable.Call(uintptr(tablePtr))

	count := *(*uint32)(tablePtr)
	// MIB_IF_TABLE2 has 8-byte alignment after the count field, then rows.
	tableBase := unsafe.Add(tablePtr, 8)

	rows := make([]IfRow2, 0, count)
	for i := range count {
		base := unsafe.Add(tableBase, uintptr(i)*ifRow2Size)
		rows = append(rows, parseIfRow2(base))
	}
	return rows, nil
}

// parseIfRow2 reads the fields we need at known offsets within MIB_IF_ROW2.
func parseIfRow2(base unsafe.Pointer) IfRow2 {
	// Layout per the Windows SDK netioapi.h (offsets verified with offsetof
	// against the real headers on Windows x64):
	// 0    InterfaceLuid (8)
	// 8    InterfaceIndex (4)
	// 12   InterfaceGuid (16)
	// 28   Alias[257] uint16 (514 bytes)
	// 542  Description[257] uint16 (514 bytes)
	// 1056 PhysicalAddressLength (4)
	// 1060 PhysicalAddress[32]
	// 1092 PermanentPhysicalAddress[32]
	// 1124 Mtu (4)
	// 1128 Type (4)
	// 1132 TunnelType (4)
	// 1136 MediaType (4)
	// 1140 PhysicalMediumType (4)
	// 1144 AccessType (4)
	// 1148 DirectionType (4)
	// 1152 InterfaceAndOperStatusFlags (1-byte bitfield) + 3 padding
	// 1156 OperStatus (4)
	// 1160 AdminStatus (4)
	// 1164 MediaConnectState (4)
	// 1168 NetworkGuid GUID (16)
	// 1184 ConnectionType (4) + 4 padding
	// 1192 TransmitLinkSpeed (8)
	// 1200 ReceiveLinkSpeed (8)
	// 1208 InOctets (8)
	// 1216 InUcastPkts (8)
	// 1224 InNUcastPkts (8)
	// 1232 InDiscards (8)
	// 1240 InErrors (8)
	// 1248 InUnknownProtos (8)
	// 1256 InUcastOctets (8)
	// 1264 InMulticastOctets (8)
	// 1272 InBroadcastOctets (8)
	// 1280 OutOctets (8)
	// 1288 OutUcastPkts (8)
	// 1296 OutNUcastPkts (8)
	// 1304 OutDiscards (8)
	// 1312 OutErrors (8)
	// 1320 OutUcastOctets (8)
	// 1328 OutMulticastOctets (8)
	// 1336 OutBroadcastOctets (8)
	// 1344 OutQLen (8) => sizeof(MIB_IF_ROW2) == 1352
	row := IfRow2{}
	row.Index = *(*uint32)(unsafe.Add(base, 8))

	aliasU16 := (*[257]uint16)(unsafe.Add(base, 28))
	descU16 := (*[257]uint16)(unsafe.Add(base, 28+514))
	row.Alias = utf16NulString(aliasU16[:])
	row.Description = utf16NulString(descU16[:])

	row.Type = *(*uint32)(unsafe.Add(base, 1128))
	row.OperStatus = *(*uint32)(unsafe.Add(base, 1156))

	row.SpeedTx = *(*uint64)(unsafe.Add(base, 1192))
	row.SpeedRx = *(*uint64)(unsafe.Add(base, 1200))

	row.InOctets = *(*uint64)(unsafe.Add(base, 1208))
	row.InUcastPkts = *(*uint64)(unsafe.Add(base, 1216))
	row.InErrors = *(*uint64)(unsafe.Add(base, 1240))

	row.OutOctets = *(*uint64)(unsafe.Add(base, 1280))
	row.OutUcastPkts = *(*uint64)(unsafe.Add(base, 1288))
	row.OutErrors = *(*uint64)(unsafe.Add(base, 1312))
	return row
}

func utf16NulString(s []uint16) string {
	for i, c := range s {
		if c == 0 {
			s = s[:i]
			break
		}
	}
	out := make([]rune, 0, len(s))
	for _, c := range s {
		out = append(out, rune(c))
	}
	return string(out)
}
