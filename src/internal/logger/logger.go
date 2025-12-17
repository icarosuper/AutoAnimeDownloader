package logger

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	Logger      zerolog.Logger
	logFile     *lumberjack.Logger
	logFilePath string
)

func getLogDirectory() (string, error) {
	var baseFolder string

	if runtime.GOOS == "windows" {
		baseFolder = os.Getenv("APPDATA")
	} else {
		baseFolder = os.Getenv("HOME")
	}

	if baseFolder == "" {
		return "", fmt.Errorf("unable to determine home directory")
	}

	logDir := filepath.Join(baseFolder, ".autoAnimeDownloader")

	// Create directory if it doesn't exist
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create log directory: %w", err)
	}

	return logDir, nil
}

func Init(development bool) {
	zerolog.CallerMarshalFunc = func(pc uintptr, file string, line int) string {
		return filepath.Base(file) + ":" + strconv.Itoa(line)
	}

	zerolog.TimeFieldFormat = time.RFC3339

	logDir, err := getLogDirectory()
	if err != nil {
		logFilePath = ""
	} else {
		logFilePath = filepath.Join(logDir, "daemon.log")

		logFile = &lumberjack.Logger{
			Filename:   logFilePath,
			MaxSize:    10,
			MaxBackups: 5,
			MaxAge:     30,
			Compress:   true,
		}
	}

	// Setup console writer (for development) or stderr (for production)
	var consoleWriter io.Writer
	if development {
		consoleWriter = zerolog.ConsoleWriter{
			Out:        os.Stderr,
			TimeFormat: time.RFC3339,
			NoColor:    false,
		}
	} else {
		consoleWriter = os.Stderr
	}

	// Setup multi-writer: write to both console and file
	var writers []io.Writer
	writers = append(writers, consoleWriter)
	if logFile != nil {
		writers = append(writers, logFile)
	}

	multiWriter := io.MultiWriter(writers...)

	level := zerolog.DebugLevel
	if !development {
		level = zerolog.InfoLevel
	}

	Logger = zerolog.New(multiWriter).
		With().
		Timestamp().
		Caller().
		Logger().
		Level(level)

	log.Logger = Logger
}

func SetLevel(level zerolog.Level) {
	zerolog.SetGlobalLevel(level)
	Logger = Logger.Level(level)
	log.Logger = Logger
}

func WithError(err error) *zerolog.Event {
	return Logger.Error().Err(err).Stack()
}

func Debug(msg string) {
	Logger.Debug().Msg(msg)
}

func Info(msg string) {
	Logger.Info().Msg(msg)
}

func Warn(msg string) {
	Logger.Warn().Msg(msg)
}

func Error(msg string) {
	Logger.Error().Msg(msg)
}

func ErrorWithStack(err error, msg string) {
	Logger.Error().Err(err).Stack().Msg(msg)
}

func GetLogFilePath() string {
	return logFilePath
}

// GetExpectedLogFilePath returns the expected log file path without initializing the logger
// This is useful for CLI tools that need to read logs without initializing the logger
func GetExpectedLogFilePath() (string, error) {
	logDir, err := getLogDirectory()
	if err != nil {
		return "", err
	}
	return filepath.Join(logDir, "daemon.log"), nil
}

func Close() error {
	if logFile != nil {
		// lumberjack doesn't have a Close method, but we can set it to nil
		// The file will be closed when the program exits
		logFile = nil
	}
	return nil
}
