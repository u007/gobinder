package gobinder

import (
	"fmt"

	"github.com/gobuffalo/envy"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var ENV = envy.Get("GO_ENV", "development")

func SetupLogging() *zap.SugaredLogger {
	var logger *zap.Logger
	var err error

	switch ENV {
	case "production":
		config := zap.NewProductionConfig()
		config.Encoding = "console"
		config.Level = zap.NewAtomicLevelAt(zap.DebugLevel)

		config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		config.EncoderConfig.TimeKey = ""
		config.OutputPaths = []string{"log/production.log"}
		config.ErrorOutputPaths = []string{"log/production.log", "log/error.log"}
		logger, err = config.Build()
	case "staging":
		config := zap.NewDevelopmentConfig()
		config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		config.EncoderConfig.TimeKey = ""
		config.OutputPaths = []string{"log/staging.log"}
		config.ErrorOutputPaths = []string{"log/staging.log", "log/error.log"}
		logger, err = config.Build()
	default:
		config := zap.NewDevelopmentConfig()
		config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		config.EncoderConfig.TimeKey = ""
		config.DisableStacktrace = false
		if ENV == "test" {
			config.OutputPaths = []string{"stdout"}
			// pop.Debug = true
			// log.SetOutput(os.Stdout)
		}
		logger, err = config.Build()
		// logger, err = zap.NewDevelopment()
	}

	if err != nil {
		panic(fmt.Sprintf("unable to initialize logger: %s", err.Error()))
	}

	// Logger = logger.WithOptions(zap.AddCallerSkip(1)).Sugar()

	defer logger.Sync()
	return logger.Sugar()
}
