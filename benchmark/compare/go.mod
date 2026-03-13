module github.com/bernardhu/connaxis/benchmark/compare

go 1.24.0

toolchain go1.24.2

require (
	github.com/bernardhu/connaxis v0.0.0
	github.com/cloudwego/netpoll v0.6.5
	github.com/panjf2000/gnet/v2 v2.9.0
	github.com/tidwall/evio v1.0.4
)

require (
	github.com/bytedance/gopkg v0.1.0 // indirect
	github.com/kavu/go_reuseport v1.5.0 // indirect
	github.com/panjf2000/ants/v2 v2.11.3 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.27.0 // indirect
	golang.org/x/sync v0.11.0 // indirect
	golang.org/x/sys v0.39.0 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.2.1 // indirect
)

replace github.com/bernardhu/connaxis => ../../
