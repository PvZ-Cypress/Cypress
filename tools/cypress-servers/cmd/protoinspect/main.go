// protoinspect prints the structure of a length-prefixed gRPC message without a
// schema: field numbers, wire types, and any values that look like text. It is
// how the shape of EA's responses is read when no .proto is available.
package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"strings"
	"unicode"
)

func printable(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	for _, c := range b {
		if c != '\t' && c != '\n' && c != '\r' && !unicode.IsPrint(rune(c)) {
			return false
		}
	}
	return true
}

func walk(data []byte, indent string, depth int) {
	if depth > 6 {
		return
	}
	for len(data) > 0 {
		key, n := binary.Uvarint(data)
		if n <= 0 {
			return
		}
		data = data[n:]
		field := key >> 3
		wire := key & 7
		switch wire {
		case 0:
			value, m := binary.Uvarint(data)
			if m <= 0 {
				return
			}
			data = data[m:]
			fmt.Printf("%s%d: varint %d\n", indent, field, value)
		case 1:
			if len(data) < 8 {
				return
			}
			fmt.Printf("%s%d: fixed64\n", indent, field)
			data = data[8:]
		case 2:
			length, m := binary.Uvarint(data)
			if m <= 0 || int(length) > len(data)-m {
				return
			}
			data = data[m:]
			value := data[:length]
			data = data[length:]
			if printable(value) {
				fmt.Printf("%s%d: %q\n", indent, field, string(value))
			} else {
				fmt.Printf("%s%d: message/bytes (%d)\n", indent, field, len(value))
				walk(value, indent+"  ", depth+1)
			}
		case 5:
			if len(data) < 4 {
				return
			}
			fmt.Printf("%s%d: fixed32\n", indent, field)
			data = data[4:]
		default:
			return
		}
	}
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: protoinspect <file.bin>")
		os.Exit(2)
	}
	raw, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	// gRPC frames each message: 1 compression byte + 4-byte big-endian length.
	for len(raw) >= 5 {
		length := binary.BigEndian.Uint32(raw[1:5])
		if int(length) > len(raw)-5 {
			break
		}
		fmt.Printf("--- gRPC message, %d bytes (compressed=%d) ---\n", length, raw[0])
		walk(raw[5:5+length], "", 0)
		raw = raw[5+length:]
	}
	fmt.Println(strings.Repeat("-", 40))
}
