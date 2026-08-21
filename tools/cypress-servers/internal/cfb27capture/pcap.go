package cfb27capture

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"cypress-servers/internal/blaze"
)

type FrameRecord struct {
	Timestamp       time.Time         `json:"timestamp"`
	Direction       string            `json:"direction"`
	Source          string            `json:"source"`
	Destination     string            `json:"destination"`
	Component       uint16            `json:"component"`
	Command         uint16            `json:"command"`
	ErrorCode       uint16            `json:"errorCode"`
	MessageType     blaze.MessageType `json:"messageType"`
	UserIndex       uint8             `json:"userIndex"`
	MessageID       uint32            `json:"messageId"`
	Options         uint8             `json:"options"`
	MetadataSize    uint16            `json:"metadataSize"`
	PayloadSize     uint32            `json:"payloadSize"`
	DecodedMetadata []blaze.Field     `json:"decodedMetadata,omitempty"`
	DecodedFields   []blaze.Field     `json:"decodedFields,omitempty"`
	RawMetadata     []byte            `json:"-"`
	RawPayload      []byte            `json:"-"`
}

type HTTPRecord struct {
	Timestamp            time.Time `json:"timestamp"`
	Direction            string    `json:"direction"`
	Source               string    `json:"source"`
	Destination          string    `json:"destination"`
	Method               string    `json:"method,omitempty"`
	Host                 string    `json:"host,omitempty"`
	Path                 string    `json:"path,omitempty"`
	Status               int       `json:"status,omitempty"`
	BodyBytes            int       `json:"bodyBytes,omitempty"`
	AuthorizationPresent bool      `json:"authorizationPresent,omitempty"`
}

type Report struct {
	Packets int            `json:"packets"`
	Frames  []FrameRecord  `json:"frames"`
	HTTP    []HTTPRecord   `json:"http"`
	Skipped map[string]int `json:"skipped"`
	streams map[string]*captureStream
}

type RouteCount struct {
	Direction   string            `json:"direction"`
	Component   uint16            `json:"component"`
	Command     uint16            `json:"command"`
	MessageType blaze.MessageType `json:"messageType"`
	ErrorCode   uint16            `json:"errorCode"`
	Count       int               `json:"count"`
}

type captureStream struct {
	direction string
	source    string
	dest      string
	data      []byte
	ends      []int
	times     []time.Time
}

func Parse(r io.Reader) (Report, error) {
	report := Report{Skipped: make(map[string]int), streams: make(map[string]*captureStream)}
	header := make([]byte, 24)
	if _, err := io.ReadFull(r, header); err != nil {
		return report, fmt.Errorf("read PCAP header: %w", err)
	}
	var order binary.ByteOrder
	switch string(header[:4]) {
	case "\xd4\xc3\xb2\xa1":
		order = binary.LittleEndian
	case "\xa1\xb2\xc3\xd4":
		order = binary.BigEndian
	default:
		return report, fmt.Errorf("unsupported PCAP magic")
	}
	if linkType := order.Uint32(header[20:24]); linkType != 1 {
		return report, fmt.Errorf("unsupported PCAP link type %d", linkType)
	}

	for recordNumber := 1; ; recordNumber++ {
		recordHeader := make([]byte, 16)
		if _, err := io.ReadFull(r, recordHeader); err != nil {
			if err == io.EOF {
				break
			}
			return report, fmt.Errorf("read PCAP record %d header: %w", recordNumber, err)
		}
		seconds := order.Uint32(recordHeader[0:4])
		micros := order.Uint32(recordHeader[4:8])
		captured := order.Uint32(recordHeader[8:12])
		if captured > 64<<20 {
			return report, fmt.Errorf("PCAP record %d length %d exceeds limit", recordNumber, captured)
		}
		packet := make([]byte, int(captured))
		if _, err := io.ReadFull(r, packet); err != nil {
			return report, fmt.Errorf("read PCAP record %d payload: %w", recordNumber, err)
		}
		report.Packets++
		parsePacket(&report, packet, time.Unix(int64(seconds), int64(micros)*1000).UTC())
	}

	for _, stream := range report.streams {
		parseHTTPStream(&report, stream)
		parseBlazeStream(&report, stream)
	}
	return report, nil
}

func parsePacket(report *Report, packet []byte, timestamp time.Time) {
	if len(packet) < 14 || !bytes.Equal(packet[12:14], []byte{0x08, 0x00}) {
		report.Skipped["non_ipv4"]++
		return
	}
	ip := packet[14:]
	if len(ip) < 20 || ip[0]>>4 != 4 {
		report.Skipped["invalid_ipv4"]++
		return
	}
	ipHeaderLength := int(ip[0]&0x0f) * 4
	if ipHeaderLength < 20 || len(ip) < ipHeaderLength || ip[9] != 6 {
		report.Skipped["non_tcp"]++
		return
	}
	tcp := ip[ipHeaderLength:]
	if len(tcp) < 20 {
		report.Skipped["invalid_tcp"]++
		return
	}
	tcpHeaderLength := int(tcp[12]>>4) * 4
	if tcpHeaderLength < 20 || len(tcp) < tcpHeaderLength {
		report.Skipped["invalid_tcp"]++
		return
	}
	payload := tcp[tcpHeaderLength:]
	if len(payload) == 0 {
		report.Skipped["empty_tcp_payload"]++
		return
	}
	sourceIP, destinationIP := net.IP(ip[12:16]).String(), net.IP(ip[16:20]).String()
	sourcePort, destinationPort := binary.BigEndian.Uint16(tcp[0:2]), binary.BigEndian.Uint16(tcp[2:4])
	direction := "unknown"
	if sourceIP == "127.0.0.1" {
		direction = "client_to_server"
	} else if destinationIP == "127.0.0.1" {
		direction = "server_to_client"
	}
	source := net.JoinHostPort(sourceIP, strconv.Itoa(int(sourcePort)))
	dest := net.JoinHostPort(destinationIP, strconv.Itoa(int(destinationPort)))
	key := direction + "|" + source + "|" + dest
	stream := report.streams[key]
	if stream == nil {
		stream = &captureStream{direction: direction, source: source, dest: dest}
		report.streams[key] = stream
	}
	stream.data = append(stream.data, payload...)
	stream.ends = append(stream.ends, len(stream.data))
	stream.times = append(stream.times, timestamp)
}

func (s *captureStream) timestampAt(offset int) time.Time {
	for i, end := range s.ends {
		if offset < end {
			return s.times[i]
		}
	}
	if len(s.times) > 0 {
		return s.times[len(s.times)-1]
	}
	return time.Time{}
}

func parseBlazeStream(report *Report, stream *captureStream) {
	data := stream.data
	if len(data) < blaze.HeaderSize {
		if len(data) > 0 && !bytes.Contains(data, []byte("HTTP/1.")) {
			report.Skipped["partial_blaze_frame"]++
		}
		return
	}
	// HTTP/TLS-adjacent streams are handled by the HTTP parser, not Blaze.
	if bytes.Contains(data[:min(len(data), 4096)], []byte("HTTP/1.")) {
		return
	}
	offset := 0
	for offset < len(data) {
		if len(data)-offset < blaze.HeaderSize {
			report.Skipped["partial_blaze_frame"]++
			return
		}
		length := binary.BigEndian.Uint32(data[offset : offset+4])
		metadataSize := binary.BigEndian.Uint16(data[offset+4 : offset+6])
		total := uint64(blaze.HeaderSize) + uint64(metadataSize) + uint64(length)
		if total > uint64(len(data)-offset) || total > blaze.HeaderSize+blaze.MaxPayloadSize+65535 {
			report.Skipped["partial_blaze_frame"]++
			return
		}
		frame, err := blaze.ReadFrame(bytes.NewReader(data[offset : offset+int(total)]))
		if err != nil {
			report.Skipped["invalid_blaze_frame"]++
			return
		}
		decodedFields, _ := blaze.Decode(frame.Payload)
		for i := range decodedFields {
			decodedFields[i] = sanitizeField(decodedFields[i])
		}
		decodedMetadata, _ := blaze.Decode(frame.Metadata)
		for i := range decodedMetadata {
			decodedMetadata[i] = sanitizeField(decodedMetadata[i])
		}
		report.Frames = append(report.Frames, FrameRecord{
			Timestamp: stream.timestampAt(offset), Direction: stream.direction,
			Source: stream.source, Destination: stream.dest,
			Component: frame.Header.Component, Command: frame.Header.Command, ErrorCode: frame.Header.ErrorCode,
			MessageType: frame.Header.MessageType, UserIndex: frame.Header.UserIndex, MessageID: frame.Header.MessageID,
			Options: frame.Header.Options, MetadataSize: frame.Header.MetadataSize, PayloadSize: frame.Header.Length,
			DecodedMetadata: decodedMetadata,
			DecodedFields:   decodedFields,
			RawMetadata:     append([]byte(nil), frame.Metadata...),
			RawPayload:      append([]byte(nil), frame.Payload...),
		})
		offset += int(total)
	}
}

func parseHTTPStream(report *Report, stream *captureStream) {
	data := string(stream.data)
	requestPattern := regexp.MustCompile(`(?i)(GET|POST|PUT|DELETE|PATCH|HEAD)\s+([^\s\r\n]+)\s+HTTP/1\.[01]\r\n`)
	responsePattern := regexp.MustCompile(`(?i)HTTP/1\.[01]\s+(\d{3})\s+[^\r\n]*\r\n`)
	type match struct {
		index, end     int
		method, target string
		status         int
	}
	matches := make([]match, 0)
	for _, found := range requestPattern.FindAllStringSubmatchIndex(data, -1) {
		method, target := data[found[2]:found[3]], data[found[4]:found[5]]
		matches = append(matches, match{index: found[0], end: found[1], method: method, target: target})
	}
	for _, found := range responsePattern.FindAllStringSubmatchIndex(data, -1) {
		status, _ := strconv.Atoi(data[found[2]:found[3]])
		matches = append(matches, match{index: found[0], end: found[1], status: status})
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].index < matches[j].index })
	for _, found := range matches {
		headStart := found.index
		if strings.HasPrefix(data[headStart:], "\r\n") {
			headStart += 2
		}
		headEndRel := strings.Index(data[headStart:], "\r\n\r\n")
		if headEndRel < 0 {
			continue
		}
		headEnd := headStart + headEndRel + 4
		headers := data[headStart:headEnd]
		record := HTTPRecord{Timestamp: stream.timestampAt(found.index), Direction: stream.direction, Source: stream.source, Destination: stream.dest, Method: found.method, Status: found.status}
		for _, headerLine := range strings.Split(headers, "\r\n") {
			parts := strings.SplitN(headerLine, ":", 2)
			if len(parts) != 2 {
				continue
			}
			switch strings.ToLower(strings.TrimSpace(parts[0])) {
			case "host":
				record.Host = strings.TrimSpace(parts[1])
			case "authorization":
				record.AuthorizationPresent = true
			}
		}
		if record.Method != "" {
			if parsed, err := url.ParseRequestURI(found.target); err == nil {
				record.Path = parsed.Path
			} else {
				record.Path = strings.Split(found.target, "?")[0]
			}
		}
		if record.Method != "" || record.Status != 0 {
			report.HTTP = append(report.HTTP, record)
		}
	}
}

func sanitizeField(field blaze.Field) blaze.Field {
	if isSensitiveTag(field.Tag) {
		field.Value = "REDACTED"
		return field
	}
	switch value := field.Value.(type) {
	case []byte:
		field.Value = "REDACTED"
	case []blaze.Field:
		fields := make([]blaze.Field, len(value))
		for i := range value {
			fields[i] = sanitizeField(value[i])
		}
		field.Value = fields
	case blaze.List:
		for i, item := range value.Values {
			if nested, ok := item.(blaze.Field); ok {
				value.Values[i] = sanitizeField(nested)
			}
		}
		field.Value = value
	case blaze.Map:
		for i := range value.Entries {
			if nested, ok := value.Entries[i].Value.(blaze.Field); ok {
				value.Entries[i].Value = sanitizeField(nested)
			}
		}
		field.Value = value
	case blaze.Union:
		if value.Value != nil {
			nested := sanitizeField(*value.Value)
			value.Value = &nested
		}
		field.Value = value
	case blaze.Variable:
		if value.Value != nil {
			nested := sanitizeField(*value.Value)
			value.Value = &nested
		}
		field.Value = value
	}
	return field
}

func isSensitiveTag(tag string) bool {
	upper := strings.ToUpper(strings.TrimSpace(tag))
	for _, exact := range []string{"AUTH", "TOKN", "BNDL", "SESS", "TICK", "PASS", "KEY", "TKN", "COOKIE", "SECRET"} {
		if upper == exact || strings.Contains(upper, exact) {
			return true
		}
	}
	return false
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (r Report) Routes() []RouteCount {
	type key struct {
		direction          string
		component, command uint16
		messageType        blaze.MessageType
		errorCode          uint16
	}
	counts := make(map[key]int)
	for _, frame := range r.Frames {
		counts[key{frame.Direction, frame.Component, frame.Command, frame.MessageType, frame.ErrorCode}]++
	}
	routes := make([]RouteCount, 0, len(counts))
	for k, count := range counts {
		routes = append(routes, RouteCount{k.direction, k.component, k.command, k.messageType, k.errorCode, count})
	}
	sort.Slice(routes, func(i, j int) bool {
		a, b := routes[i], routes[j]
		if a.Direction != b.Direction {
			return a.Direction < b.Direction
		}
		if a.Component != b.Component {
			return a.Component < b.Component
		}
		if a.Command != b.Command {
			return a.Command < b.Command
		}
		if a.MessageType != b.MessageType {
			return a.MessageType < b.MessageType
		}
		return a.ErrorCode < b.ErrorCode
	})
	return routes
}
