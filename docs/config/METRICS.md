# Metrics

`connaxis` emits metrics through the `wrapper.IMetrics` interface. You can plug in Prometheus or any custom collector.

## Emitted Keys

Counters (Count):
- `qps.connaxis.accept`
- `qps.connaxis.loop.write`
- `qps.connaxis.loop.read`
- `qps.connaxis.loop.recvcmd`
- `qps.connaxis.loop.cmdconsume`
- `qps.connaxis.loop.cmddrop`
- `qps.connaxis.loop.cmdfail`
- `qps.connaxis.http.reject.header_too_large`
- `qps.connaxis.http.reject.body_too_large`
- `qps.connaxis.http.reject.chunked`
- `qps.connaxis.http.reject.expect_continue`
- `qps.connaxis.http.reject.parse_error`
- `qps.connaxis.fasthttp.reject.header_too_large`
- `qps.connaxis.fasthttp.reject.body_too_large`
- `qps.connaxis.fasthttp.reject.chunked`
- `qps.connaxis.fasthttp.reject.expect_continue`
- `qps.connaxis.fasthttp.reject.parse_error`

Gauges:
- `qps.connaxis.online`
- `qps.connaxis.dials`

## Prometheus Example

```go
// Example adapter (sketch):
// - replace dots with underscores for Prometheus names.

package main

import (
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/bernardhu/connaxis/wrapper"
)

type promMetrics struct {
	counters map[string]prometheus.Counter
	gauges   map[string]prometheus.Gauge
}

func (p *promMetrics) getCounter(k string) prometheus.Counter {
	name := strings.ReplaceAll(k, ".", "_")
	if c, ok := p.counters[name]; ok {
		return c
	}
	c := prometheus.NewCounter(prometheus.CounterOpts{Name: name})
	prometheus.MustRegister(c)
	p.counters[name] = c
	return c
}

func (p *promMetrics) getGauge(k string) prometheus.Gauge {
	name := strings.ReplaceAll(k, ".", "_")
	if g, ok := p.gauges[name]; ok {
		return g
	}
	g := prometheus.NewGauge(prometheus.GaugeOpts{Name: name})
	prometheus.MustRegister(g)
	p.gauges[name] = g
	return g
}

func (p *promMetrics) Gauge(k string, v int64)    { p.getGauge(k).Set(float64(v)) }
func (p *promMetrics) Increment(k string)          { p.getCounter(k).Inc() }
func (p *promMetrics) Count(k string, v int64)      { p.getCounter(k).Add(float64(v)) }
func (p *promMetrics) Timing(k string, v time.Duration) {}

func main() {
	wrapper.SetMetrics(&promMetrics{
		counters: map[string]prometheus.Counter{},
		gauges:   map[string]prometheus.Gauge{},
	})
}
```

For a runnable example, see `examples/prometheus`.
