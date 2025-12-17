package logger

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

func TestInit_DevelopmentMode(t *testing.T) {
	// Save original logger
	originalLogger := Logger

	// Test development mode
	Init(true)

	// Verify logger is initialized
	if Logger.GetLevel() != zerolog.DebugLevel {
		t.Errorf("Expected DebugLevel in development mode, got %v", Logger.GetLevel())
	}

	// Restore original logger
	Logger = originalLogger
}

func TestInit_ProductionMode(t *testing.T) {
	// Save original logger
	originalLogger := Logger

	// Test production mode
	Init(false)

	// Verify logger is initialized
	if Logger.GetLevel() != zerolog.InfoLevel {
		t.Errorf("Expected InfoLevel in production mode, got %v", Logger.GetLevel())
	}

	// Restore original logger
	Logger = originalLogger
}

func TestSetLevel(t *testing.T) {
	// Save original logger and level
	originalLogger := Logger
	originalLevel := zerolog.GlobalLevel()

	// Initialize logger
	Init(true)

	// Test setting different levels
	levels := []zerolog.Level{
		zerolog.DebugLevel,
		zerolog.InfoLevel,
		zerolog.WarnLevel,
		zerolog.ErrorLevel,
	}

	for _, level := range levels {
		SetLevel(level)
		if Logger.GetLevel() != level {
			t.Errorf("Expected level %v, got %v", level, Logger.GetLevel())
		}
		if zerolog.GlobalLevel() != level {
			t.Errorf("Expected global level %v, got %v", level, zerolog.GlobalLevel())
		}
	}

	// Restore
	Logger = originalLogger
	zerolog.SetGlobalLevel(originalLevel)
}

func TestLogger_Output_Development(t *testing.T) {
	// Save original logger
	originalLogger := Logger

	// Create a buffer to capture output
	var buf bytes.Buffer

	// Initialize logger in development mode with custom output
	zerolog.CallerMarshalFunc = func(pc uintptr, file string, line int) string {
		return file + ":" + string(rune(line))
	}
	zerolog.TimeFieldFormat = "2006-01-02T15:04:05Z07:00"

	output := zerolog.ConsoleWriter{
		Out:        &buf,
		TimeFormat: "2006-01-02T15:04:05Z07:00",
		NoColor:    true, // Disable colors for testing
	}
	Logger = zerolog.New(output).
		With().
		Timestamp().
		Caller().
		Logger().
		Level(zerolog.DebugLevel)

	// Test logging at different levels
	Logger.Debug().Msg("debug message")
	Logger.Info().Msg("info message")
	Logger.Warn().Msg("warn message")
	Logger.Error().Msg("error message")

	outputStr := buf.String()

	// Verify output contains messages
	if !strings.Contains(outputStr, "debug message") {
		t.Error("Expected output to contain 'debug message'")
	}
	if !strings.Contains(outputStr, "info message") {
		t.Error("Expected output to contain 'info message'")
	}
	if !strings.Contains(outputStr, "warn message") {
		t.Error("Expected output to contain 'warn message'")
	}
	if !strings.Contains(outputStr, "error message") {
		t.Error("Expected output to contain 'error message'")
	}

	// Restore original logger
	Logger = originalLogger
}

func TestLogger_Output_Production(t *testing.T) {
	// Save original logger
	originalLogger := Logger

	// Create a buffer to capture output
	var buf bytes.Buffer

	// Initialize logger in production mode (JSON output)
	zerolog.TimeFieldFormat = "2006-01-02T15:04:05Z07:00"
	Logger = zerolog.New(&buf).
		With().
		Timestamp().
		Caller().
		Logger().
		Level(zerolog.InfoLevel)

	// Test logging
	Logger.Info().Str("key", "value").Msg("test message")

	outputStr := buf.String()

	// Verify output is JSON
	var jsonData map[string]interface{}
	if err := json.Unmarshal([]byte(outputStr), &jsonData); err != nil {
		t.Errorf("Expected JSON output, got error: %v. Output: %s", err, outputStr)
	}

	// Verify JSON contains expected fields
	if jsonData["level"] == nil {
		t.Error("Expected JSON to contain 'level' field")
	}
	if jsonData["message"] != "test message" {
		t.Errorf("Expected message 'test message', got %v", jsonData["message"])
	}
	if jsonData["key"] != "value" {
		t.Errorf("Expected key 'value', got %v", jsonData["key"])
	}

	// Restore original logger
	Logger = originalLogger
}

func TestLogger_CallerInformation(t *testing.T) {
	// Save original logger
	originalLogger := Logger

	// Create a buffer to capture output
	var buf bytes.Buffer

	// Initialize logger with caller information
	zerolog.CallerMarshalFunc = func(pc uintptr, file string, line int) string {
		return file + ":" + string(rune(line))
	}
	zerolog.TimeFieldFormat = "2006-01-02T15:04:05Z07:00"
	Logger = zerolog.New(&buf).
		With().
		Timestamp().
		Caller().
		Logger().
		Level(zerolog.InfoLevel)

	// Log a message
	Logger.Info().Msg("test with caller")

	outputStr := buf.String()

	// Verify output contains caller information
	var jsonData map[string]interface{}
	if err := json.Unmarshal([]byte(outputStr), &jsonData); err != nil {
		t.Fatalf("Failed to parse JSON: %v. Output: %s", err, outputStr)
	}

	if jsonData["caller"] == nil {
		t.Error("Expected JSON to contain 'caller' field")
	}

	// Restore original logger
	Logger = originalLogger
}

func TestLogger_HelperFunctions(t *testing.T) {
	// Save original logger
	originalLogger := Logger

	// Initialize logger with buffer output
	var buf bytes.Buffer
	zerolog.TimeFieldFormat = "2006-01-02T15:04:05Z07:00"
	Logger = zerolog.New(&buf).
		With().
		Timestamp().
		Logger().
		Level(zerolog.DebugLevel)

	// Test helper functions
	Debug("debug message")
	Info("info message")
	Warn("warn message")
	Error("error message")

	outputStr := buf.String()

	// Verify messages were logged
	if !strings.Contains(outputStr, "debug message") {
		t.Error("Expected Debug() to log message")
	}
	if !strings.Contains(outputStr, "info message") {
		t.Error("Expected Info() to log message")
	}
	if !strings.Contains(outputStr, "warn message") {
		t.Error("Expected Warn() to log message")
	}
	if !strings.Contains(outputStr, "error message") {
		t.Error("Expected Error() to log message")
	}

	// Restore original logger
	Logger = originalLogger
}

func TestWithError(t *testing.T) {
	// Save original logger
	originalLogger := Logger

	// Initialize logger with buffer output
	var buf bytes.Buffer
	zerolog.TimeFieldFormat = "2006-01-02T15:04:05Z07:00"
	Logger = zerolog.New(&buf).
		With().
		Timestamp().
		Logger().
		Level(zerolog.ErrorLevel)

	// Test WithError
	testErr := &testLoggerError{msg: "test error"}
	WithError(testErr).Msg("error with stack")

	outputStr := buf.String()

	// Verify error was logged
	if !strings.Contains(outputStr, "test error") {
		t.Error("Expected WithError() to log error message")
	}

	// Restore original logger
	Logger = originalLogger
}

func TestErrorWithStack(t *testing.T) {
	// Save original logger
	originalLogger := Logger

	// Initialize logger with buffer output
	var buf bytes.Buffer
	zerolog.TimeFieldFormat = "2006-01-02T15:04:05Z07:00"
	Logger = zerolog.New(&buf).
		With().
		Timestamp().
		Logger().
		Level(zerolog.ErrorLevel)

	// Test ErrorWithStack
	testErr := &testLoggerError{msg: "test error"}
	ErrorWithStack(testErr, "error message")

	outputStr := buf.String()

	// Verify error was logged
	if !strings.Contains(outputStr, "test error") {
		t.Error("Expected ErrorWithStack() to log error message")
	}
	if !strings.Contains(outputStr, "error message") {
		t.Error("Expected ErrorWithStack() to log message")
	}

	// Restore original logger
	Logger = originalLogger
}

func TestLogger_StructuredFields(t *testing.T) {
	// Save original logger
	originalLogger := Logger

	// Create a buffer to capture output
	var buf bytes.Buffer

	// Initialize logger in production mode (JSON)
	zerolog.TimeFieldFormat = "2006-01-02T15:04:05Z07:00"
	Logger = zerolog.New(&buf).
		With().
		Timestamp().
		Logger().
		Level(zerolog.InfoLevel)

	// Test structured logging
	Logger.Info().
		Str("anime", "Test Anime").
		Int("episode", 5).
		Bool("downloaded", true).
		Msg("episode processed")

	outputStr := buf.String()

	// Verify JSON structure
	var jsonData map[string]interface{}
	if err := json.Unmarshal([]byte(outputStr), &jsonData); err != nil {
		t.Fatalf("Failed to parse JSON: %v. Output: %s", err, outputStr)
	}

	// Verify structured fields
	if jsonData["anime"] != "Test Anime" {
		t.Errorf("Expected anime 'Test Anime', got %v", jsonData["anime"])
	}
	if jsonData["episode"] != float64(5) { // JSON numbers are float64
		t.Errorf("Expected episode 5, got %v", jsonData["episode"])
	}
	if jsonData["downloaded"] != true {
		t.Errorf("Expected downloaded true, got %v", jsonData["downloaded"])
	}

	// Restore original logger
	Logger = originalLogger
}

func TestLogger_DefaultOutput(t *testing.T) {
	// Save original logger
	originalLogger := Logger

	// Test that Init doesn't panic with default stderr output
	Init(true)
	if Logger.GetLevel() != zerolog.DebugLevel {
		t.Error("Expected logger to be initialized")
	}

	// Test that logging doesn't panic
	Logger.Info().Msg("test message")

	// Restore original logger
	Logger = originalLogger
}

func TestLogger_LevelFiltering(t *testing.T) {
	// Save original logger
	originalLogger := Logger

	// Create a buffer to capture output
	var buf bytes.Buffer

	// Initialize logger at Info level
	zerolog.TimeFieldFormat = "2006-01-02T15:04:05Z07:00"
	Logger = zerolog.New(&buf).
		With().
		Timestamp().
		Logger().
		Level(zerolog.InfoLevel)

	// Log at different levels
	Logger.Debug().Msg("debug message - should not appear")
	Logger.Info().Msg("info message - should appear")
	Logger.Warn().Msg("warn message - should appear")
	Logger.Error().Msg("error message - should appear")

	outputStr := buf.String()

	// Verify debug message is filtered out
	if strings.Contains(outputStr, "debug message") {
		t.Error("Debug message should be filtered out at Info level")
	}

	// Verify other messages appear
	if !strings.Contains(outputStr, "info message") {
		t.Error("Info message should appear")
	}
	if !strings.Contains(outputStr, "warn message") {
		t.Error("Warn message should appear")
	}
	if !strings.Contains(outputStr, "error message") {
		t.Error("Error message should appear")
	}

	// Restore original logger
	Logger = originalLogger
}

// testLoggerError é um tipo de erro simples para testes
type testLoggerError struct {
	msg string
}

func (e *testLoggerError) Error() string {
	return e.msg
}

func init() {
	// Ensure we don't interfere with actual logging during tests
	// by initializing with a test-friendly setup
	if os.Getenv("TEST_LOGGER_INIT") == "" {
		// Only initialize if not already initialized by test
		Init(true)
	}
}

