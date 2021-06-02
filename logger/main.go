package logger

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var logger *zap.Logger

// GetLogger returns the logger of the application
func GetLogger() *zap.Logger {
	if logger == nil {
		writerSyncer := getLogWriter()
		encoder := getEncoder()
		core := zapcore.NewCore(encoder, writerSyncer, zapcore.DebugLevel)
		logger = zap.New(core)
	}
	return logger
}

func getEncoder() zapcore.Encoder {
	return zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
}
func getLogWriter() zapcore.WriteSyncer {
	file, _ := os.Create("./gosk.log")
	return zapcore.AddSync(file)
}
