package gobinder

import (
	"context"

	"github.com/u007/gobinder/lib"
)

func logging(ctx context.Context) lib.Logger {
	return *ctx.Value("log").(*lib.Logger)
}
