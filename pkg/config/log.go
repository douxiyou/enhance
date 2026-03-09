package config

import (
	"log"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func (c *Config) Logger() *zap.Logger {
	return c.logger
}
func (c *Config) InitLog() *zap.Logger {
	log := zap.New(zapcore.NewCore(
		zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
		zapcore.AddSync(log.Writer()),
		zap.DebugLevel,
	))
	return log
}
