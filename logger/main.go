package logger

import (
	"sync"

	"go.uber.org/zap"
)

var (
	logger *zap.Logger
	// once guards the lazy default: the first callers of GetLogger are
	// routinely a set of goroutines starting at the same moment - one
	// per modbus register group, say - and they used to race each other
	// building and assigning a logger apiece.
	once sync.Once
)

// GetLogger returns the logger of the application
func GetLogger() *zap.Logger {
	once.Do(func() {
		if logger == nil {
			logger, _ = zap.NewProduction()
		}
	})
	return logger
}

// SetLogger overwrites the default logger, used for testing
func SetLogger(newLogger *zap.Logger) {
	logger = newLogger
}
