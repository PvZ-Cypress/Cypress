package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"cypress-servers/internal/blaze"
	"cypress-servers/internal/cfb27capture"
)

func main() {
	commandsText := flag.String("commands", "", "comma-separated Dynasty command IDs")
	startNotifications := flag.Bool("start-notifications", false, "extract notifications between the first Dynasty 107 and 542 calls")
	flag.Parse()
	if flag.NArg() != 2 || (*commandsText == "" && !*startNotifications) {
		fmt.Fprintln(os.Stderr, "usage: cfb27fixtureextract [-commands 161,541] [-start-notifications] <capture.acp> <output-dir>")
		os.Exit(2)
	}
	commands := make(map[uint16]bool)
	if *commandsText != "" {
		for _, text := range strings.Split(*commandsText, ",") {
			value, err := strconv.ParseUint(strings.TrimSpace(text), 10, 16)
			if err != nil {
				fmt.Fprintf(os.Stderr, "invalid command %q: %v\n", text, err)
				os.Exit(2)
			}
			commands[uint16(value)] = true
		}
	}
	input, err := os.Open(flag.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer input.Close()
	report, err := cfb27capture.Parse(input)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if len(commands) > 0 {
		if err := writeFixtures(report, flag.Arg(1), commands); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	if *startNotifications {
		if err := os.MkdirAll(flag.Arg(1), 0o755); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		windows := []struct {
			trigger uint16
			until   uint16
		}{
			{304, 1112},
			{541, 107},
			{107, 542},
		}
		for _, window := range windows {
			batches := notificationBatches(report, 2098, window.trigger, 2098, window.until)
			for index, frames := range batches {
				if len(frames) == 0 {
					continue
				}
				name := filepath.Join(flag.Arg(1), fmt.Sprintf("dynasty-%d-notifications-%03d.bin", window.trigger, index+1))
				if err := writeNotificationFixture(name, frames); err != nil {
					fmt.Fprintln(os.Stderr, err)
					os.Exit(1)
				}
			}
		}
	}
}

func notificationFramesAfterFirstCommand(report cfb27capture.Report, triggerComponent, triggerCommand, untilComponent, untilCommand uint16) []blaze.Frame {
	batches := notificationBatches(report, triggerComponent, triggerCommand, untilComponent, untilCommand)
	if len(batches) == 0 {
		return nil
	}
	return batches[0]
}

func notificationBatches(report cfb27capture.Report, triggerComponent, triggerCommand, untilComponent, untilCommand uint16) [][]blaze.Frame {
	frames := append([]cfb27capture.FrameRecord(nil), report.Frames...)
	sort.SliceStable(frames, func(i, j int) bool { return frames[i].Timestamp.Before(frames[j].Timestamp) })
	active := false
	batches := make([][]blaze.Frame, 0)
	for _, frame := range frames {
		if frame.Direction == "client_to_server" && frame.Component == triggerComponent && frame.Command == triggerCommand {
			batches = append(batches, nil)
			active = true
			continue
		}
		if active && frame.Direction == "client_to_server" && frame.Component == untilComponent && frame.Command == untilCommand {
			active = false
			continue
		}
		if !active || frame.Direction != "server_to_client" || frame.MessageType != blaze.MessageTypeNotification {
			continue
		}
		batch := len(batches) - 1
		batches[batch] = append(batches[batch], blaze.Frame{
			Header: blaze.Header{
				Component: frame.Component, Command: frame.Command, MessageType: frame.MessageType,
				UserIndex: frame.UserIndex, Options: frame.Options,
			},
			Metadata: append([]byte(nil), frame.RawMetadata...),
			Payload:  append([]byte(nil), frame.RawPayload...),
		})
	}
	return batches
}

func writeNotificationFixture(path string, frames []blaze.Frame) error {
	var data bytes.Buffer
	for _, frame := range frames {
		if err := blaze.WriteFrame(&data, frame); err != nil {
			return err
		}
	}
	return os.WriteFile(path, data.Bytes(), 0o644)
}

func writeFixtures(report cfb27capture.Report, outputDir string, commands map[uint16]bool) error {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return err
	}
	written := make(map[uint16]bool)
	for _, frame := range report.Frames {
		if frame.Direction != "server_to_client" || frame.Component != 2098 ||
			!commands[frame.Command] || written[frame.Command] || len(frame.RawPayload) == 0 {
			continue
		}
		name := filepath.Join(outputDir, fmt.Sprintf("dynasty-%d-reply.bin", frame.Command))
		if err := os.WriteFile(name, frame.RawPayload, 0o644); err != nil {
			return err
		}
		written[frame.Command] = true
	}
	for command := range commands {
		if !written[command] {
			return fmt.Errorf("capture has no non-empty server reply for Dynasty command %d", command)
		}
	}
	return nil
}
