package logger

import (
	"bytes"
	"strings"
	"testing"
)

func TestLogger(t *testing.T) {
	// Test debug level
	SetLevel(LevelDebug)

	// Capture output
	var buf bytes.Buffer
	instance.stdlog.SetOutput(&buf)

	// Test all log levels
	Debug("Debug message", "key 1", "value 1")
	if !strings.Contains(buf.String(), "[DEBUG]") {
		t.Error("Debug log not found")
	}
	buf.Reset()

	Info("Info message", "key 2", "value 2")
	if !strings.Contains(buf.String(), "[INFO]") {
		t.Error("Info log not found")
	}
	buf.Reset()

	Warn("Warn message", "key 3", "value 3")
	if !strings.Contains(buf.String(), "[WARN]") {
		t.Error("Warn log not found")
	}
	buf.Reset()

	Error("Error message", "key 4", "value 4")
	if !strings.Contains(buf.String(), "[ERROR]") {
		t.Error("Error log not found")
	}
	buf.Reset()

	// Test level filtering
	SetLevel(LevelError)
	Debug("This should not appear")
	if strings.Contains(buf.String(), "[DEBUG]") {
		t.Error("Debug message appeared when level is Error")
	}

	SetLevel(LevelInfo) // Reset to default
}

func TestLoggerInit(t *testing.T) {
	// Test that logger initializes automatically
	Close() // Close any existing instance

	// This should create a new instance
	Info("Test message")
	if instance == nil {
		t.Error("Logger instance not created automatically")
	}

	// Clean up
	Close()
}
