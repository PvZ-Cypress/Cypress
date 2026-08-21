package cfb27capture

import (
	"bytes"
	"encoding/binary"
	"testing"

	"cypress-servers/internal/blaze"
)

func TestParseReassemblesBlazeAcrossTCPRecords(t *testing.T) {
	frame := blazeBytes(4, 7, 1, []byte("abc"))
	cut := len(frame) / 2
	report, err := Parse(bytes.NewReader(testPCAP(binary.LittleEndian,
		testTCPPacket(frame[:cut]), testTCPPacket(frame[cut:]))))
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Frames) != 1 {
		t.Fatalf("expected one reassembled frame, got %d skipped=%v", len(report.Frames), report.Skipped)
	}
	if report.Skipped["partial_blaze_frame"] != 0 {
		t.Fatalf("reassembly reported a partial frame: %#v", report.Skipped)
	}
}

func TestParseRetainsRawBlazePayload(t *testing.T) {
	payload := []byte{0xde, 0xad, 0xbe, 0xef}
	report, err := Parse(bytes.NewReader(testPCAP(binary.LittleEndian,
		testTCPPacket(blazeBytes(2098, 161, 7, payload)))))
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Frames) != 1 {
		t.Fatalf("expected one frame, got %d", len(report.Frames))
	}
	if !bytes.Equal(report.Frames[0].RawPayload, payload) {
		t.Fatalf("raw payload = %x, want %x", report.Frames[0].RawPayload, payload)
	}
}

func TestParseRetainsRawBlazeMetadata(t *testing.T) {
	metadata, err := blaze.Encode([]blaze.Field{{Tag: "CNTX", Type: blaze.TypeInteger, Value: int64(42)}})
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte{0xde, 0xad}
	var frame bytes.Buffer
	if err := blaze.WriteFrame(&frame, blaze.Frame{
		Header:   blaze.Header{Component: 2099, Command: 101, MessageType: blaze.MessageTypeNotification},
		Metadata: metadata,
		Payload:  payload,
	}); err != nil {
		t.Fatal(err)
	}
	report, err := Parse(bytes.NewReader(testPCAP(binary.LittleEndian, testTCPPacket(frame.Bytes()))))
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Frames) != 1 {
		t.Fatalf("expected one frame, got %d", len(report.Frames))
	}
	if !bytes.Equal(report.Frames[0].RawMetadata, metadata) {
		t.Fatalf("raw metadata = %x, want %x", report.Frames[0].RawMetadata, metadata)
	}
	if len(report.Frames[0].DecodedMetadata) != 1 || report.Frames[0].DecodedMetadata[0].Tag != "CNTX" || report.Frames[0].DecodedMetadata[0].Value != int64(42) {
		t.Fatalf("decoded metadata = %#v, want CNTX=42", report.Frames[0].DecodedMetadata)
	}
}

func TestParseExtractsRedactedHTTPPath(t *testing.T) {
	http := []byte("POST /dynasty/advance?token=secret HTTP/1.1\r\nHost: dynasty.example\r\nAuthorization: Bearer secret\r\n\r\n")
	report, err := Parse(bytes.NewReader(testPCAP(binary.LittleEndian, testTCPPacket(http))))
	if err != nil {
		t.Fatal(err)
	}
	if len(report.HTTP) != 1 {
		t.Fatalf("expected one HTTP record, got %d", len(report.HTTP))
	}
	got := report.HTTP[0]
	if got.Method != "POST" || got.Host != "dynasty.example" || got.Path != "/dynasty/advance" {
		t.Fatalf("unexpected HTTP record: %#v", got)
	}
	if bytes.Contains([]byte(got.Path), []byte("secret")) || got.AuthorizationPresent != true {
		t.Fatalf("HTTP secrets were not redacted: %#v", got)
	}
}

func TestParseRedactsSensitiveBlazeFields(t *testing.T) {
	payload, err := blaze.Encode([]blaze.Field{{Tag: "TOKN", Type: blaze.TypeString, Value: "secret-token"}})
	if err != nil {
		t.Fatal(err)
	}
	report, err := Parse(bytes.NewReader(testPCAP(binary.LittleEndian, testTCPPacket(blazeBytes(1, 10, 1, payload)))))
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Frames) != 1 || len(report.Frames[0].DecodedFields) != 1 {
		t.Fatalf("unexpected frame: %#v", report)
	}
	if got := report.Frames[0].DecodedFields[0].Value; got != "REDACTED" {
		t.Fatalf("sensitive field was not redacted: %#v", got)
	}
}

func TestParseExtractsFire2FrameAndRoutes(t *testing.T) {
	frame := []byte{0, 0, 0, 0, 0, 0, 0, 1, 0, 10, 0, 0, 7, 0, 0, 0}
	report, err := Parse(bytes.NewReader(testPCAP(binary.LittleEndian, testTCPPacket(frame))))
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Frames) != 1 {
		t.Fatalf("expected one frame, got %d", len(report.Frames))
	}
	got := report.Frames[0]
	if got.Direction != "client_to_server" || got.Component != 1 || got.Command != 10 || got.MessageID != 7 {
		t.Fatalf("unexpected frame: %#v", got)
	}
	routes := report.Routes()
	if len(routes) != 1 || routes[0].Count != 1 || routes[0].Component != 1 || routes[0].Command != 10 {
		t.Fatalf("unexpected routes: %#v", routes)
	}
}

func TestParseSupportsBigEndianPCAP(t *testing.T) {
	report, err := Parse(bytes.NewReader(testPCAP(binary.BigEndian, testTCPPacket(make([]byte, 16)))))
	if err != nil || len(report.Frames) != 1 {
		t.Fatalf("frames=%d err=%v", len(report.Frames), err)
	}
}

func TestParseCountsPartialFrameWithoutPayloadDisclosure(t *testing.T) {
	report, err := Parse(bytes.NewReader(testPCAP(binary.LittleEndian, testTCPPacket([]byte{0, 0, 0, 20}))))
	if err != nil {
		t.Fatal(err)
	}
	if report.Skipped["partial_blaze_frame"] != 1 || len(report.Frames) != 0 {
		t.Fatalf("unexpected report: %#v", report)
	}
}

func TestParseRejectsUnsupportedLinkType(t *testing.T) {
	data := testPCAP(binary.LittleEndian)
	binary.LittleEndian.PutUint32(data[20:24], 101)
	if _, err := Parse(bytes.NewReader(data)); err == nil {
		t.Fatal("expected unsupported link type error")
	}
}

func testPCAP(order binary.ByteOrder, packets ...[]byte) []byte {
	var out bytes.Buffer
	if order == binary.LittleEndian {
		out.Write([]byte{0xd4, 0xc3, 0xb2, 0xa1})
	} else {
		out.Write([]byte{0xa1, 0xb2, 0xc3, 0xd4})
	}
	write := func(v any) { _ = binary.Write(&out, order, v) }
	write(uint16(2))
	write(uint16(4))
	write(uint32(0))
	write(uint32(0))
	write(uint32(65535))
	write(uint32(1))
	for i, packet := range packets {
		write(uint32(100 + i))
		write(uint32(200))
		write(uint32(len(packet)))
		write(uint32(len(packet)))
		out.Write(packet)
	}
	return out.Bytes()
}

func testTCPPacket(payload []byte) []byte {
	p := make([]byte, 14+20+20+len(payload))
	p[12], p[13] = 0x08, 0x00
	p[14], p[23] = 0x45, 6
	p[26], p[27], p[28], p[29] = 127, 0, 0, 1
	p[30], p[31], p[32], p[33] = 192, 168, 1, 1
	binary.BigEndian.PutUint16(p[34:36], 40000)
	binary.BigEndian.PutUint16(p[36:38], 443)
	p[46] = 5 << 4
	copy(p[54:], payload)
	return p
}

func blazeBytes(component, command uint16, messageID uint32, payload []byte) []byte {
	frame := make([]byte, blaze.HeaderSize+len(payload))
	binary.BigEndian.PutUint32(frame[0:4], uint32(len(payload)))
	binary.BigEndian.PutUint16(frame[6:8], component)
	binary.BigEndian.PutUint16(frame[8:10], command)
	frame[10] = byte(messageID >> 16)
	frame[11] = byte(messageID >> 8)
	frame[12] = byte(messageID)
	copy(frame[blaze.HeaderSize:], payload)
	return frame
}
