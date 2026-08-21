package main

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunTextAndJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "capture.acp")
	if err := os.WriteFile(path, commandTestPCAP(), 0600); err != nil {
		t.Fatal(err)
	}
	for _, format := range []string{"text", "json"} {
		t.Run(format, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := run([]string{"-format", format, path}, &stdout, &stderr); code != 0 {
				t.Fatalf("code=%d stderr=%s", code, stderr.String())
			}
			if format == "json" && !strings.Contains(stdout.String(), `"http"`) {
				t.Fatalf("JSON output omitted HTTP records: %s", stdout.String())
			}
			if format == "json" && strings.Contains(stdout.String(), "payloadHex") {
				t.Fatalf("JSON output disclosed raw payload bytes: %s", stdout.String())
			}
			if !strings.Contains(stdout.String(), "1") || strings.Contains(strings.ToLower(stdout.String()), "bearer") {
				t.Fatalf("unexpected output: %s", stdout.String())
			}
		})
	}
}

func TestRunRejectsMissingInputAndInvalidFormat(t *testing.T) {
	for _, args := range [][]string{{}, {"-format", "xml", "x"}} {
		var stdout, stderr bytes.Buffer
		if code := run(args, &stdout, &stderr); code == 0 {
			t.Fatalf("expected failure for %#v", args)
		}
	}
}

func commandTestPCAP() []byte {
	var out bytes.Buffer
	out.Write([]byte{0xd4, 0xc3, 0xb2, 0xa1})
	write := func(v any) { _ = binary.Write(&out, binary.LittleEndian, v) }
	write(uint16(2))
	write(uint16(4))
	write(uint32(0))
	write(uint32(0))
	write(uint32(65535))
	write(uint32(1))
	frame := []byte{0, 0, 0, 0, 0, 0, 0, 1, 0, 10, 0, 0, 7, 0, 0, 0}
	packet := make([]byte, 54+len(frame))
	packet[12] = 8
	packet[13] = 0
	packet[14] = 0x45
	packet[23] = 6
	packet[26] = 127
	packet[29] = 1
	packet[30] = 192
	packet[31] = 168
	packet[32] = 1
	packet[33] = 1
	binary.BigEndian.PutUint16(packet[34:36], 40000)
	binary.BigEndian.PutUint16(packet[36:38], 443)
	packet[46] = 0x50
	copy(packet[54:], frame)
	write(uint32(1))
	write(uint32(2))
	write(uint32(len(packet)))
	write(uint32(len(packet)))
	out.Write(packet)
	return out.Bytes()
}
