package logging

import (
	"os"

	"github.com/sirupsen/logrus"
)

var log = &logrus.Logger{
	Out:          os.Stderr,
	Formatter:    &logrus.TextFormatter{DisableTimestamp: true},
	Hooks:        make(logrus.LevelHooks),
	Level:        logrus.WarnLevel,
	ExitFunc:     os.Exit,
	ReportCaller: false,
}

// L returns the package-level logger instance used throughout dcx.
func L() *logrus.Logger {
	return log
}

// SetLevel configures the logging level from a string such as "debug",
// "info", "warn", or "error".
func SetLevel(level string) error {
	lvl, err := logrus.ParseLevel(level)
	if err != nil {
		return err
	}
	log.SetLevel(lvl)
	return nil
}
