package logger

import (
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var (
	Logger zerolog.Logger
)

func Init(development bool) {
	zerolog.CallerMarshalFunc = func(pc uintptr, file string, line int) string {
		return filepath.Base(file) + ":" + strconv.Itoa(line)
	}

	zerolog.TimeFieldFormat = time.RFC3339

	if development {
		output := zerolog.ConsoleWriter{
			Out:        os.Stderr,
			TimeFormat: time.RFC3339,
			NoColor:    false,
		}
		Logger = zerolog.New(output).
			With().
			Timestamp().
			Caller().
			Logger().
			Level(zerolog.DebugLevel)
	} else {
		Logger = zerolog.New(os.Stderr).
			With().
			Timestamp().
			Caller().
			Logger().
			Level(zerolog.InfoLevel)
	}

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
