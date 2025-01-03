package log

import (
	"fmt"
	"log"
	"os"
	"sync"
)

type Logger interface {
	Debugf(format string, v ...any)
	Infof(format string, v ...any)
	Errorf(format string, v ...any)
}

type defaultLogger struct {
	stdlog *log.Logger
	errlog *log.Logger
}

func (l *defaultLogger) Debugf(format string, v ...any) {
	_ = l.stdlog.Output(2, fmt.Sprintf(format, v...))
}

func (l *defaultLogger) Infof(format string, v ...any) {
	_ = l.stdlog.Output(2, fmt.Sprintf(format, v...))
}

func (l *defaultLogger) Errorf(format string, v ...any) {
	_ = l.errlog.Output(2, fmt.Sprintf(format, v...))
}

var (
	mux    sync.RWMutex
	logger Logger = &defaultLogger{
		stdlog: log.New(os.Stdout, "", log.LstdFlags),
		errlog: log.New(os.Stderr, "ERROR: ", log.LstdFlags),
	}
)

func SetLogger(l Logger) {
	mux.Lock()
	defer mux.Unlock()
	logger = l
}

func Debugf(format string, v ...any) {
	mux.RLock()
	defer mux.RUnlock()
	logger.Debugf(format, v...)
}

func Infof(format string, v ...any) {
	mux.RLock()
	defer mux.RUnlock()
	logger.Infof(format, v...)
}

func Errorf(format string, v ...any) {
	mux.RLock()
	defer mux.RUnlock()
	logger.Errorf(format, v...)
}
