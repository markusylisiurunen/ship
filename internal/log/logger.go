package log

import (
	"context"
	"fmt"
	"log"
	"os"
)

type Logger interface {
	Debugf(ctx context.Context, format string, v ...any)
	Infof(ctx context.Context, format string, v ...any)
	Errorf(ctx context.Context, format string, v ...any)
}

type defaultLogger struct {
	log *log.Logger
}

func (l *defaultLogger) Debugf(_ context.Context, format string, v ...any) {
	_ = l.log.Output(2, fmt.Sprintf(format, v...))
}

func (l *defaultLogger) Infof(_ context.Context, format string, v ...any) {
	_ = l.log.Output(2, fmt.Sprintf(format, v...))
}

func (l *defaultLogger) Errorf(_ context.Context, format string, v ...any) {
	_ = l.log.Output(2, fmt.Sprintf(format, v...))
}

var logger Logger = &defaultLogger{
	log: log.New(os.Stderr, "", log.LstdFlags|log.Lshortfile),
}

func SetLogger(l Logger) {
	logger = l
}
