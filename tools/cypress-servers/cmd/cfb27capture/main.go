package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"

	"cypress-servers/internal/cfb27capture"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("cfb27capture", flag.ContinueOnError)
	flags.SetOutput(stderr)
	format := flags.String("format", "text", "output format: text or json")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: cfb27capture [-format text|json] <capture.acp>")
		return 2
	}
	if *format != "text" && *format != "json" {
		fmt.Fprintln(stderr, "format must be text or json")
		return 2
	}

	input, err := os.Open(flags.Arg(0))
	if err != nil {
		fmt.Fprintf(stderr, "open capture: %v\n", err)
		return 1
	}
	defer input.Close()
	report, err := cfb27capture.Parse(input)
	if err != nil {
		fmt.Fprintf(stderr, "parse capture: %v\n", err)
		return 1
	}

	if *format == "json" {
		result := struct {
			Packets      int                        `json:"packets"`
			Frames       int                        `json:"frameCount"`
			FrameRecords []cfb27capture.FrameRecord `json:"frames"`
			HTTP         []cfb27capture.HTTPRecord  `json:"http"`
			Skipped      map[string]int             `json:"skipped"`
			Routes       []cfb27capture.RouteCount  `json:"routes"`
		}{report.Packets, len(report.Frames), report.Frames, report.HTTP, report.Skipped, report.Routes()}
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(result); err != nil {
			fmt.Fprintf(stderr, "write JSON: %v\n", err)
			return 1
		}
		return 0
	}

	fmt.Fprintf(stdout, "packets=%d frames=%d\n", report.Packets, len(report.Frames))
	fmt.Fprintf(stdout, "http=%d\n", len(report.HTTP))
	for _, request := range report.HTTP {
		if request.Method != "" {
			fmt.Fprintf(stdout, "http %s host=%s path=%s status=%d\n", request.Method, request.Host, request.Path, request.Status)
		} else {
			fmt.Fprintf(stdout, "http response host=%s status=%d\n", request.Host, request.Status)
		}
	}
	for _, route := range report.Routes() {
		fmt.Fprintf(stdout, "%s component=0x%04X command=0x%04X type=%d error=0x%04X count=%d\n",
			route.Direction, route.Component, route.Command, route.MessageType, route.ErrorCode, route.Count)
	}
	reasons := make([]string, 0, len(report.Skipped))
	for reason := range report.Skipped {
		reasons = append(reasons, reason)
	}
	sort.Strings(reasons)
	for _, reason := range reasons {
		count := report.Skipped[reason]
		fmt.Fprintf(stdout, "skipped reason=%s count=%d\n", reason, count)
	}
	return 0
}
