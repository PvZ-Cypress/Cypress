package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"cypress-servers/internal/blaze"
	"cypress-servers/internal/cfb27capture"
)

func TestWriteFixturesSelectsFirstCapturedDynastyReply(t *testing.T) {
	report := cfb27capture.Report{Frames: []cfb27capture.FrameRecord{
		{Direction: "client_to_server", Component: 2098, Command: 161, RawPayload: []byte("request")},
		{Direction: "server_to_client", Component: 2098, Command: 161, RawPayload: []byte("first")},
		{Direction: "server_to_client", Component: 2098, Command: 161, RawPayload: []byte("second")},
		{Direction: "server_to_client", Component: 2098, Command: 541, RawPayload: []byte("hub")},
	}}
	dir := t.TempDir()
	if err := writeFixtures(report, dir, map[uint16]bool{161: true, 541: true}); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{
		"dynasty-161-reply.bin": "first",
		"dynasty-541-reply.bin": "hub",
	} {
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != want {
			t.Fatalf("%s = %q, want %q", name, got, want)
		}
	}
}

func TestNotificationFramesAfterFirstCommandPreserveCaptureOrder(t *testing.T) {
	base := time.Date(2026, 8, 5, 4, 36, 20, 0, time.UTC)
	report := cfb27capture.Report{Frames: []cfb27capture.FrameRecord{
		{Timestamp: base.Add(4 * time.Second), Direction: "client_to_server", Component: 2098, Command: 542},
		{Timestamp: base.Add(2 * time.Second), Direction: "server_to_client", Component: 2099, Command: 101, MessageType: blaze.MessageTypeNotification, RawMetadata: []byte("m2"), RawPayload: []byte("second")},
		{Timestamp: base, Direction: "client_to_server", Component: 2098, Command: 107},
		{Timestamp: base.Add(time.Second), Direction: "server_to_client", Component: 2099, Command: 101, MessageType: blaze.MessageTypeNotification, RawMetadata: []byte("m1"), RawPayload: []byte("first")},
		{Timestamp: base.Add(5 * time.Second), Direction: "server_to_client", Component: 2099, Command: 101, MessageType: blaze.MessageTypeNotification, RawPayload: []byte("too-late")},
	}}

	frames := notificationFramesAfterFirstCommand(report, 2098, 107, 2098, 542)
	if len(frames) != 2 {
		t.Fatalf("got %d notification frames, want 2", len(frames))
	}
	if !bytes.Equal(frames[0].Payload, []byte("first")) || !bytes.Equal(frames[1].Payload, []byte("second")) {
		t.Fatalf("payload order = %q, %q", frames[0].Payload, frames[1].Payload)
	}
	if !bytes.Equal(frames[0].Metadata, []byte("m1")) || !bytes.Equal(frames[1].Metadata, []byte("m2")) {
		t.Fatalf("metadata was not preserved: %q, %q", frames[0].Metadata, frames[1].Metadata)
	}
}

func TestWriteNotificationFixtureWritesReplayableBlazeFrames(t *testing.T) {
	frames := []blaze.Frame{
		{Header: blaze.Header{Component: 2099, Command: 101, MessageType: blaze.MessageTypeNotification}, Metadata: []byte("one"), Payload: []byte("alpha")},
		{Header: blaze.Header{Component: 2099, Command: 101, MessageType: blaze.MessageTypeNotification}, Metadata: []byte("two"), Payload: []byte("beta")},
	}
	path := filepath.Join(t.TempDir(), "notifications.bin")
	if err := writeNotificationFixture(path, frames); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	reader := bytes.NewReader(data)
	for i, want := range frames {
		got, err := blaze.ReadFrame(reader)
		if err != nil {
			t.Fatalf("read frame %d: %v", i, err)
		}
		if !bytes.Equal(got.Metadata, want.Metadata) || !bytes.Equal(got.Payload, want.Payload) {
			t.Fatalf("frame %d = metadata %q payload %q", i, got.Metadata, got.Payload)
		}
	}
	if reader.Len() != 0 {
		t.Fatalf("fixture has %d trailing bytes", reader.Len())
	}
}

func TestNotificationBatchesKeepEmptyCommandWindows(t *testing.T) {
	base := time.Date(2026, 8, 5, 4, 36, 20, 0, time.UTC)
	report := cfb27capture.Report{Frames: []cfb27capture.FrameRecord{
		{Timestamp: base, Direction: "client_to_server", Component: 2098, Command: 107},
		{Timestamp: base.Add(time.Second), Direction: "server_to_client", Component: 2099, Command: 101, MessageType: blaze.MessageTypeNotification, RawPayload: []byte("first")},
		{Timestamp: base.Add(2 * time.Second), Direction: "client_to_server", Component: 2098, Command: 542},
		{Timestamp: base.Add(3 * time.Second), Direction: "client_to_server", Component: 2098, Command: 107},
		{Timestamp: base.Add(4 * time.Second), Direction: "client_to_server", Component: 2098, Command: 542},
		{Timestamp: base.Add(5 * time.Second), Direction: "client_to_server", Component: 2098, Command: 107},
		{Timestamp: base.Add(6 * time.Second), Direction: "server_to_client", Component: 2099, Command: 101, MessageType: blaze.MessageTypeNotification, RawPayload: []byte("third")},
		{Timestamp: base.Add(7 * time.Second), Direction: "client_to_server", Component: 2098, Command: 542},
	}}

	batches := notificationBatches(report, 2098, 107, 2098, 542)
	if len(batches) != 3 {
		t.Fatalf("got %d batches, want 3", len(batches))
	}
	if len(batches[0]) != 1 || len(batches[1]) != 0 || len(batches[2]) != 1 {
		t.Fatalf("batch sizes = %d, %d, %d; want 1, 0, 1", len(batches[0]), len(batches[1]), len(batches[2]))
	}
}
