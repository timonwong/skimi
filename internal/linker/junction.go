package linker

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"unicode/utf16"
)

// Windows reparse point constants, mirrored here so the encoder below has no
// windows-only imports and stays unit-testable on every platform.
const (
	// IO_REPARSE_TAG_MOUNT_POINT, the tag every directory junction carries.
	ioReparseTagMountPoint = 0xA0000003
	// REPARSE_DATA_BUFFER header: ReparseTag (4) + ReparseDataLength (2) + Reserved (2).
	reparseHeaderSize = 8
	// MountPointReparseBuffer fields ahead of PathBuffer: four uint16.
	mountPointHeaderSize = 8
	// MAXIMUM_REPARSE_DATA_BUFFER_SIZE, the largest buffer the kernel accepts.
	maxReparseDataBufferSize = 16 * 1024
)

// encodeMountPointReparseData builds the REPARSE_DATA_BUFFER that turns an
// empty directory into a junction pointing at target, which must be an
// absolute Windows path such as `C:\Users\me\skills\demo`.
//
// The layout is fixed by MS-FSCC 2.1.2.5 and matches what `mklink /J` writes:
// the 8-byte header, four uint16 name offsets and lengths, then a UTF-16 path
// buffer holding the NT substitute name (`\??\` + target) followed by the
// printable name (target), each NUL-terminated. Offsets and lengths are byte
// counts and exclude those terminators.
func encodeMountPointReparseData(target string) ([]byte, error) {
	if target == "" {
		return nil, errors.New("encode junction data: empty target")
	}
	if strings.ContainsRune(target, 0) {
		return nil, fmt.Errorf("encode junction data: target %q contains NUL", target)
	}

	substituteName := utf16.Encode([]rune(`\??\` + target))
	printName := utf16.Encode([]rune(target))
	pathWords := len(substituteName) + 1 + len(printName) + 1 // both names are NUL-terminated

	dataLen := mountPointHeaderSize + 2*pathWords
	total := reparseHeaderSize + dataLen
	if total > maxReparseDataBufferSize {
		return nil, fmt.Errorf("encode junction data: target %q needs %d bytes, max is %d", target, total, maxReparseDataBufferSize)
	}

	buf := make([]byte, total)
	binary.LittleEndian.PutUint32(buf[0:], ioReparseTagMountPoint)
	binary.LittleEndian.PutUint16(buf[4:], uint16(dataLen))
	// buf[6:8] is Reserved and stays zero.
	binary.LittleEndian.PutUint16(buf[8:], 0)                                  // SubstituteNameOffset
	binary.LittleEndian.PutUint16(buf[10:], uint16(2*len(substituteName)))     // SubstituteNameLength
	binary.LittleEndian.PutUint16(buf[12:], uint16(2*(len(substituteName)+1))) // PrintNameOffset
	binary.LittleEndian.PutUint16(buf[14:], uint16(2*len(printName)))          // PrintNameLength

	pathBuffer := buf[reparseHeaderSize+mountPointHeaderSize:]
	for i, w := range substituteName {
		binary.LittleEndian.PutUint16(pathBuffer[2*i:], w)
	}
	printOffset := 2 * (len(substituteName) + 1)
	for i, w := range printName {
		binary.LittleEndian.PutUint16(pathBuffer[printOffset+2*i:], w)
	}
	return buf, nil
}
