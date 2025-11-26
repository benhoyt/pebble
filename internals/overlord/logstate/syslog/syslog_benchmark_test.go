package syslog

import (
	"testing"
	"time"
)

func BenchmarkEncodeEntryOld(b *testing.B) {
	client, err := NewClient(&ClientOptions{
		Location: "tcp://dummy:1234",
		Hostname: "myhostname",
		SDID:     "pebble",
	})
	if err != nil {
		b.Fatalf("Failed to create syslog client: %v", err)
	}
	timestamp := time.Date(2025, 1, 2, 15, 4, 5, 123456789, time.UTC)
	entry := entryWithService{
		service:   "redis",
		Timestamp: timestamp.Format(time.RFC3339Nano),
		Message:   "This is a reasonably long log message to benchmark encodeEntry.",
	}
	b.ResetTimer()
	for b.Loop() {
		client.encodeEntryOld(entry)
		client.sendBuf.Reset()
	}
}

func BenchmarkEncodeEntry(b *testing.B) {
	client, err := NewClient(&ClientOptions{
		Location: "tcp://dummy:1234",
		Hostname: "myhostname",
		SDID:     "pebble",
	})
	if err != nil {
		b.Fatalf("Failed to create syslog client: %v", err)
	}
	timestamp := time.Date(2025, 1, 2, 15, 4, 5, 123456789, time.UTC)
	entry := entryWithService{
		service:   "redis",
		Timestamp: timestamp.Format(time.RFC3339Nano),
		Message:   "This is a reasonably long log message to benchmark the encodeEntry function.",
	}
	b.ResetTimer()
	for b.Loop() {
		client.encodeEntry(entry)
		client.sendBuf.Reset()
	}
}
