package lib

import (
	"context"
)

type Logger interface {
	Debugf(string, ...interface{})
	Infof(string, ...interface{})
	Warnf(string, ...interface{})
	Errorf(string, ...interface{})
}

func logging(ctx context.Context) Logger {
	return *ctx.Value("log").(*Logger)
}
