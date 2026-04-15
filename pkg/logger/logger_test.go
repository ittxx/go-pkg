package logger

import (
	"bytes"
	"testing"
)

func TestNewLoggerDefault(t *testing.T) {
	cfg := DefaultConfig()
	var buf bytes.Buffer
	l := NewWithWriter(cfg, &buf)
	if l == nil {
		t.Fatal("expected logger instance, got nil")
	}

	l.Info("test info", "key", "value")
	if buf.Len() == 0 {
		t.Fatal("expected output written to buffer")
	}
}
