// Command inspectpayload decodes a raw Blaze TDF payload file to JSON.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"cypress-servers/internal/blaze"
)

func main() {
	in := flag.String("in", "", "payload file")
	flag.Parse()
	data, err := os.ReadFile(*in)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fields, err := blaze.Decode(data)
	if err != nil {
		fmt.Fprintln(os.Stderr, "decode error:", err)
	}
	out, _ := json.MarshalIndent(fields, "", "  ")
	fmt.Println(string(out))
}
