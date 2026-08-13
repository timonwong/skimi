package linker

import (
	"encoding/binary"
	"strings"
	"testing"
	"unicode/utf16"
)

// decodedReparseData is the parsed form of a mount point REPARSE_DATA_BUFFER,
// used to check the bytes encodeMountPointReparseData hands to the kernel.
type decodedReparseData struct {
	tag            uint32
	dataLen        int
	reserved       uint16
	substituteName string
	printName      string
	nulTerminated  bool
}

func decodeMountPointReparseData(t *testing.T, buf []byte) decodedReparseData {
	t.Helper()
	if len(buf) < reparseHeaderSize+mountPointHeaderSize {
		t.Fatalf("buffer too short: %d bytes", len(buf))
	}

	got := decodedReparseData{
		tag:      binary.LittleEndian.Uint32(buf[0:]),
		dataLen:  int(binary.LittleEndian.Uint16(buf[4:])),
		reserved: binary.LittleEndian.Uint16(buf[6:]),
	}
	if want := len(buf) - reparseHeaderSize; got.dataLen != want {
		t.Errorf("ReparseDataLength = %d, want %d", got.dataLen, want)
	}

	subOffset := int(binary.LittleEndian.Uint16(buf[8:]))
	subLen := int(binary.LittleEndian.Uint16(buf[10:]))
	printOffset := int(binary.LittleEndian.Uint16(buf[12:]))
	printLen := int(binary.LittleEndian.Uint16(buf[14:]))

	pathBuffer := buf[reparseHeaderSize+mountPointHeaderSize:]
	readUTF16 := func(offset, length int) string {
		if offset+length > len(pathBuffer) {
			t.Fatalf("name at offset %d length %d exceeds path buffer of %d bytes", offset, length, len(pathBuffer))
		}
		words := make([]uint16, length/2)
		for i := range words {
			words[i] = binary.LittleEndian.Uint16(pathBuffer[offset+2*i:])
		}
		return string(utf16.Decode(words))
	}
	got.substituteName = readUTF16(subOffset, subLen)
	got.printName = readUTF16(printOffset, printLen)

	// Both names must be NUL-terminated inside the path buffer, and the
	// print name must start right after the substitute name's terminator.
	subTerm := binary.LittleEndian.Uint16(pathBuffer[subOffset+subLen:])
	printTerm := binary.LittleEndian.Uint16(pathBuffer[printOffset+printLen:])
	got.nulTerminated = subTerm == 0 && printTerm == 0 && printOffset == subOffset+subLen+2
	return got
}

func TestEncodeMountPointReparseData(t *testing.T) {
	tests := []struct {
		name              string
		target            string
		wantSubstituteRaw string
	}{
		{
			name:              "drive letter path",
			target:            `C:\Users\me\.local\share\skimi\skills\demo`,
			wantSubstituteRaw: `\??\C:\Users\me\.local\share\skimi\skills\demo`,
		},
		{
			name:              "drive root",
			target:            `D:\`,
			wantSubstituteRaw: `\??\D:\`,
		},
		{
			name:              "unc path",
			target:            `\\server\share\skills`,
			wantSubstituteRaw: `\??\\\server\share\skills`,
		},
		{
			name:              "non-ascii path",
			target:            `C:\用户\技能\写作`,
			wantSubstituteRaw: `\??\C:\用户\技能\写作`,
		},
		{
			name:              "path outside the basic multilingual plane",
			target:            `C:\skills\🚀`,
			wantSubstituteRaw: `\??\C:\skills\🚀`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf, err := encodeMountPointReparseData(tt.target)
			if err != nil {
				t.Fatalf("encodeMountPointReparseData: %v", err)
			}

			got := decodeMountPointReparseData(t, buf)
			if got.tag != ioReparseTagMountPoint {
				t.Errorf("ReparseTag = %#x, want %#x", got.tag, ioReparseTagMountPoint)
			}
			if got.reserved != 0 {
				t.Errorf("Reserved = %d, want 0", got.reserved)
			}
			if got.substituteName != tt.wantSubstituteRaw {
				t.Errorf("SubstituteName = %q, want %q", got.substituteName, tt.wantSubstituteRaw)
			}
			if got.printName != tt.target {
				t.Errorf("PrintName = %q, want %q", got.printName, tt.target)
			}
			if !got.nulTerminated {
				t.Error("names are not NUL-terminated back to back in the path buffer")
			}

			// The kernel reads exactly ReparseDataLength bytes past the
			// header, so the buffer must not carry slack.
			wantLen := reparseHeaderSize + mountPointHeaderSize +
				2*(len(utf16.Encode([]rune(tt.wantSubstituteRaw)))+1+len(utf16.Encode([]rune(tt.target)))+1)
			if len(buf) != wantLen {
				t.Errorf("len(buf) = %d, want %d", len(buf), wantLen)
			}
		})
	}
}

func TestEncodeMountPointReparseDataErrors(t *testing.T) {
	tests := []struct {
		name    string
		target  string
		wantMsg string
	}{
		{
			name:    "empty target",
			target:  "",
			wantMsg: "empty target",
		},
		{
			name:    "target with NUL",
			target:  "C:\\skills\x00\\demo",
			wantMsg: "contains NUL",
		},
		{
			name:    "target too long for the reparse buffer",
			target:  `C:\` + strings.Repeat("a", maxReparseDataBufferSize),
			wantMsg: "max is",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf, err := encodeMountPointReparseData(tt.target)
			if err == nil {
				t.Fatalf("encodeMountPointReparseData(%q) = %d bytes, want error", tt.target, len(buf))
			}
			if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), tt.wantMsg)
			}
		})
	}
}
