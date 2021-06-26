package logger

import (
	"log/syslog"

	"github.com/tchap/zapext/v2/zapsyslog"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var logger *zap.Logger

// GetLogger returns the logger of the application
func GetLogger() *zap.Logger {
	if logger == nil {
		writer, _ := syslog.New(syslog.LOG_ERR|syslog.LOG_LOCAL0, "")
		encoder := zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
		core := zapsyslog.NewCore(zapcore.InfoLevel, encoder, writer)
		logger = zap.New(core)
	}
	return logger
}

// SetLogger overwrites the default logger, used for testing
func SetLogger(newLogger *zap.Logger) {
	logger = newLogger
}
