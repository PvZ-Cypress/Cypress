// cfb27capturedump decodes a capture.acp2 produced by capturehook.dll.
//
// The capture is a stream of [dir:1][conn:8 LE][len:4 LE][bytes] records, where
// bytes are the decrypted ProtoSSL payload — the raw Blaze frames. Records are
// grouped by connection and direction, reassembled, and parsed with the shared
// Blaze decoder so the output is a readable transcript of every command the game
// exchanged with EA.
package main

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"

	"cypress-servers/internal/blaze"
)

type record struct {
	direction  uint8
	connection uint64
	payload    []byte
}

type decodedFrame struct {
	Connection  string        `json:"connection"`
	Direction   string        `json:"direction"`
	Component   uint16        `json:"component"`
	Command     uint16        `json:"command"`
	MessageType uint8         `json:"messageType"`
	MessageID   uint32        `json:"messageId"`
	ErrorCode   uint16        `json:"errorCode"`
	PayloadSize int           `json:"payloadSize"`
	Decoded     []blaze.Field `json:"decoded,omitempty"`
	DecodeError string        `json:"decodeError,omitempty"`
	PayloadHex  string        `json:"payloadHex,omitempty"`
}

func main() {
	var input string
	var jsonOut bool
	var withHex bool
	flag.StringVar(&input, "in", "", "path to capture.acp2")
	flag.BoolVar(&jsonOut, "json", false, "emit JSON lines instead of a summary")
	flag.BoolVar(&withHex, "hex", false, "include payload hex for undecodable frames")
	flag.Parse()

	if input == "" {
		fmt.Fprintln(os.Stderr, "fatal: -in is required")
		os.Exit(1)
	}
	raw, err := os.ReadFile(input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}

	records, err := parseRecords(raw)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}

	// Reassemble each (connection, direction) as an ordered byte stream, then
	// pull Blaze frames out of it.
	type key struct {
		connection uint64
		direction  uint8
	}
	streams := map[key][]byte{}
	order := []key{}
	for _, r := range records {
		k := key{r.connection, r.direction}
		if _, ok := streams[k]; !ok {
			order = append(order, k)
		}
		streams[k] = append(streams[k], r.payload...)
	}

	var frames []decodedFrame
	for _, k := range order {
		data := streams[k]
		reader := bytes.NewReader(data)
		for reader.Len() > 0 {
			before := reader.Len()
			frame, err := blaze.ReadFrame(reader)
			if err != nil {
				// Not Blaze (auth/CDN stream) or a partial tail; skip the rest.
				break
			}
			if reader.Len() == before {
				break
			}
			df := decodedFrame{
				Connection:  fmt.Sprintf("0x%x", k.connection),
				Direction:   directionName(k.direction),
				Component:   frame.Header.Component,
				Command:     frame.Header.Command,
				MessageType: uint8(frame.Header.MessageType),
				MessageID:   frame.Header.MessageID,
				ErrorCode:   frame.Header.ErrorCode,
				PayloadSize: len(frame.Payload),
			}
			// Hex is emitted for every frame when asked, not just undecodable ones:
			// replaying a captured reply needs its raw bytes even when it decodes.
			if withHex {
				df.PayloadHex = hex.EncodeToString(frame.Payload)
			}
			if fields, decErr := blaze.Decode(frame.Payload); decErr != nil {
				df.DecodeError = decErr.Error()
			} else {
				df.Decoded = fields
			}
			frames = append(frames, df)
		}
	}

	if jsonOut {
		encoder := json.NewEncoder(os.Stdout)
		for _, f := range frames {
			_ = encoder.Encode(f)
		}
		return
	}
	printSummary(records, frames)
}

func parseRecords(raw []byte) ([]record, error) {
	var records []record
	reader := bytes.NewReader(raw)
	header := make([]byte, 13)
	for {
		if _, err := io.ReadFull(reader, header); err != nil {
			if err == io.EOF {
				return records, nil
			}
			return records, nil // tolerate a truncated tail
		}
		direction := header[0]
		connection := binary.LittleEndian.Uint64(header[1:9])
		length := binary.LittleEndian.Uint32(header[9:13])
		if length == 0 || length > 8<<20 {
			return records, fmt.Errorf("implausible record length %d; capture likely corrupt", length)
		}
		payload := make([]byte, length)
		if _, err := io.ReadFull(reader, payload); err != nil {
			return records, nil // truncated final record
		}
		records = append(records, record{direction: direction, connection: connection, payload: payload})
	}
}

func directionName(direction uint8) string {
	if direction == 1 {
		return "client_to_server"
	}
	return "server_to_client"
}

func printSummary(records []record, frames []decodedFrame) {
	fmt.Printf("records: %d   frames decoded: %d\n\n", len(records), len(frames))

	type routeKey struct {
		direction string
		component uint16
		command   uint16
	}
	counts := map[routeKey]int{}
	for _, f := range frames {
		counts[routeKey{f.Direction, f.Component, f.Command}]++
	}
	keys := make([]routeKey, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].component != keys[j].component {
			return keys[i].component < keys[j].component
		}
		if keys[i].command != keys[j].command {
			return keys[i].command < keys[j].command
		}
		return keys[i].direction < keys[j].direction
	})
	fmt.Println("component/command   dir                 count")
	for _, k := range keys {
		fmt.Printf("  %d/%-6d        %-18s  %d\n", k.component, k.command, k.direction, counts[k])
	}
	fmt.Println("\nRun with -json for the full decoded transcript.")
}
