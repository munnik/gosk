package logger

import (
	"go.uber.org/zap"
)

var logger *zap.Logger

// GetLogger returns the logger of the application
func GetLogger() *zap.Logger {
	if logger == nil {
		logger, _ = zap.NewProduction()
	}
	return logger
}

// SetLogger overwrites the default logger, used for testing
func SetLogger(newLogger *zap.Logger) {
	logger = newLogger
}
