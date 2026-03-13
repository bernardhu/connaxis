package examplelog

import (
	"fmt"
	stdlog "log"
	"os"
	"runtime"
	"strings"
	"sync"

	"github.com/bernardhu/connaxis/wrapper"
)

// LogCfg keeps a minimal compatibility surface for benchmark examples that
// previously used an internal logger package.
type LogCfg struct {
	Underlying         string
	FileName           string
	LogLevel           string
	FormatTimeWithMs   bool
	FormatWithFileName bool
}

type level int

const (
	levelDebug level = iota
	levelInfo
	levelWarn
	levelError
)

type stdAdapter struct {
	mu   sync.RWMutex
	l    *stdlog.Logger
	lv   level
	flag int
}

var global = &stdAdapter{
	l:    stdlog.New(os.Stderr, "", stdlog.LstdFlags),
	lv:   levelInfo,
	flag: stdlog.LstdFlags,
}

func Init(cfg *LogCfg) {
	global.mu.Lock()
	defer global.mu.Unlock()
	if cfg != nil {
		global.lv = parseLevel(cfg.LogLevel)
		flags := stdlog.LstdFlags
		if cfg.FormatTimeWithMs {
			flags = stdlog.LstdFlags | stdlog.Lmicroseconds
		}
		if cfg.FormatWithFileName {
			flags |= stdlog.Lshortfile
		}
		global.flag = flags
	}
	global.l = stdlog.New(os.Stderr, "", global.flag)
}

func Flush() {}

func GetLogger() wrapper.ILogger { return global }

func Debug(v ...interface{})                 { global.Debug(v...) }
func Info(v ...interface{})                  { global.Info(v...) }
func Warn(v ...interface{})                  { global.Warn(v...) }
func Error(v ...interface{})                 { global.Error(v...) }
func Fatal(v ...interface{})                 { global.Fatal(v...) }
func Panic(v ...interface{})                 { global.Panic(v...) }
func Debugf(format string, v ...interface{}) { global.Debugf(format, v...) }
func Infof(format string, v ...interface{})  { global.Infof(format, v...) }
func Warnf(format string, v ...interface{})  { global.Warnf(format, v...) }
func Errorf(format string, v ...interface{}) { global.Errorf(format, v...) }
func Fatalf(format string, v ...interface{}) { global.Fatalf(format, v...) }
func Panicf(format string, v ...interface{}) { global.Panicf(format, v...) }

func (s *stdAdapter) Debug(v ...interface{}) { s.output(levelDebug, "DEBUG", fmt.Sprint(v...)) }
func (s *stdAdapter) Info(v ...interface{})  { s.output(levelInfo, "INFO", fmt.Sprint(v...)) }
func (s *stdAdapter) Warn(v ...interface{})  { s.output(levelWarn, "WARN", fmt.Sprint(v...)) }
func (s *stdAdapter) Error(v ...interface{}) { s.output(levelError, "ERROR", fmt.Sprint(v...)) }
func (s *stdAdapter) Fatal(v ...interface{}) {
	s.output(levelError, "FATAL", fmt.Sprint(v...))
	os.Exit(1)
}
func (s *stdAdapter) Panic(v ...interface{}) {
	msg := fmt.Sprint(v...)
	s.output(levelError, "PANIC", msg)
	panic(msg)
}

func (s *stdAdapter) Debugf(format string, v ...interface{}) {
	s.outputf(levelDebug, "DEBUG", format, v...)
}
func (s *stdAdapter) Infof(format string, v ...interface{}) {
	s.outputf(levelInfo, "INFO", format, v...)
}
func (s *stdAdapter) Warnf(format string, v ...interface{}) {
	s.outputf(levelWarn, "WARN", format, v...)
}
func (s *stdAdapter) Errorf(format string, v ...interface{}) {
	s.outputf(levelError, "ERROR", format, v...)
}
func (s *stdAdapter) Panicf(format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	s.output(levelError, "PANIC", msg)
	panic(msg)
}
func (s *stdAdapter) Fatalf(format string, v ...interface{}) {
	s.outputf(levelError, "FATAL", format, v...)
	os.Exit(1)
}

func (s *stdAdapter) Flush() {}

func (s *stdAdapter) outputf(min level, prefix string, format string, v ...interface{}) {
	s.output(min, prefix, fmt.Sprintf(format, v...))
}

func (s *stdAdapter) output(min level, prefix string, msg string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if min < s.lv {
		return
	}
	_ = s.l.Output(resolveCallDepth(), prefix+" "+msg)
}

func resolveCallDepth() int {
	// Called from stdAdapter.output -> log.Output.
	// Try to locate the first frame outside logger/wrapper internals.
	const (
		startDepth   = 2
		maxScanDepth = 20
	)
	for depth := startDepth; depth < maxScanDepth; depth++ {
		pc, _, _, ok := runtime.Caller(depth)
		if !ok {
			break
		}
		fn := runtime.FuncForPC(pc)
		if fn == nil {
			continue
		}
		name := fn.Name()
		if strings.Contains(name, "benchmark/internal/examplelog.") {
			continue
		}
		if strings.Contains(name, "/wrapper.") || strings.Contains(name, "github.com/bernardhu/connaxis/wrapper.") {
			continue
		}
		return depth
	}
	// Keep previous behavior as fallback.
	return 3
}

func parseLevel(in string) level {
	switch strings.ToLower(strings.TrimSpace(in)) {
	case "debug":
		return levelDebug
	case "warn", "warning":
		return levelWarn
	case "error":
		return levelError
	default:
		return levelInfo
	}
}
