package wrapper

import (
	"fmt"
	"os"
	"time"
)

var gMetrics IMetrics
var glogger ILogger

type IMetrics interface {
	Gauge(k string, v int64)
	Increment(k string)
	Count(k string, v int64)
	Timing(k string, v time.Duration)
}

func Gauge(k string, v int64) {
	if gMetrics != nil {
		gMetrics.Gauge(k, v)
	}
}

func Increment(k string) {
	if gMetrics != nil {
		gMetrics.Increment(k)
	}
}

func Count(k string, v int64) {
	if gMetrics != nil {
		gMetrics.Count(k, v)
	}
}

func Timing(k string, v time.Duration) {
	if gMetrics != nil {
		gMetrics.Timing(k, v)
	}
}

type ILogger interface {
	Debug(v ...interface{})
	Info(v ...interface{})
	Warn(v ...interface{})
	Error(v ...interface{})
	Fatal(v ...interface{})
	Panic(v ...interface{})

	Debugf(format string, v ...interface{})
	Infof(format string, v ...interface{})
	Warnf(format string, v ...interface{})
	Errorf(format string, v ...interface{})
	Panicf(format string, v ...interface{})
	Fatalf(format string, v ...interface{})

	Flush()
}

func Debugf(format string, v ...interface{}) {
	if glogger == nil {
		return
	}
	glogger.Debugf(format, v...)
}

func Infof(format string, v ...interface{}) {
	if glogger == nil {
		return
	}
	glogger.Infof(format, v...)
}

func Warnf(format string, v ...interface{}) {
	if glogger == nil {
		return
	}
	glogger.Warnf(format, v...)
}

func Errorf(format string, v ...interface{}) {
	if glogger == nil {
		return
	}
	glogger.Errorf(format, v...)
}

func Panicf(format string, v ...interface{}) {
	if glogger == nil {
		fmt.Printf(format, v...)
		os.Exit(1)
	}
	glogger.Panicf(format, v...)
	os.Exit(1)
}

func Fatalf(format string, v ...interface{}) {
	if glogger == nil {
		fmt.Printf(format, v...)
		os.Exit(1)
	}
	glogger.Fatalf(format, v...)
	os.Exit(1)
}

func Debug(v ...interface{}) {
	if glogger == nil {
		return
	}
	glogger.Debug(v...)
}

func Info(v ...interface{}) {
	if glogger == nil {
		return
	}
	glogger.Info(v...)
}

func Warn(v ...interface{}) {
	if glogger == nil {
		return
	}
	glogger.Warn(v...)
}

func Error(v ...interface{}) {
	if glogger == nil {
		return
	}
	glogger.Error(v...)
}

func Panic(v ...interface{}) {
	if glogger == nil {
		return
	}
	glogger.Panic(v...)
	os.Exit(1)
}

func Fatal(v ...interface{}) {
	if glogger == nil {
		return
	}
	glogger.Fatal(v...)
	os.Exit(1)
}

func SetLogger(logger ILogger) {
	glogger = logger
}

func SetMetrics(metrics IMetrics) {
	gMetrics = metrics
}

func Flush() {
	if glogger != nil {
		glogger.Flush()
	}
}
