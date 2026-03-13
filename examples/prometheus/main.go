package main

import (
	"flag"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/bernardhu/connaxis"
	"github.com/bernardhu/connaxis/connection"
	"github.com/bernardhu/connaxis/eventloop"
	"github.com/bernardhu/connaxis/wrapper"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
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

func (p *promMetrics) Gauge(k string, v int64)          { p.getGauge(k).Set(float64(v)) }
func (p *promMetrics) Increment(k string)               { p.getCounter(k).Inc() }
func (p *promMetrics) Count(k string, v int64)          { p.getCounter(k).Add(float64(v)) }
func (p *promMetrics) Timing(k string, v time.Duration) {}

type handler struct{}

func (h *handler) OnReady(s eventloop.IServer) {
	log.Printf("ready: listen on %v (loops: %d)", s.GetListenAddrs(), s.GetWorkerNum())
}

func (h *handler) OnClosed(c connection.AppConn, err error) { _ = err }

func (h *handler) OnConnected(c connection.ProtoConn) {
	c.SetPktHandler(h)
}

func (h *handler) ParsePacket(c connection.ProtoConn, in *[]byte) (int, int) {
	_ = c
	return len(*in), len(*in)
}

func (h *handler) OnData(c connection.ProtoConn, in *[]byte) ([]byte, bool) {
	_ = c
	return *in, false
}

func (h *handler) Stat(bool) {}

func main() {
	var path string
	var metricsAddr string
	flag.StringVar(&path, "p", "conf/connaxis.conf", "config file path")
	flag.StringVar(&metricsAddr, "metrics", ":2112", "prometheus metrics address")
	flag.Parse()

	wrapper.SetMetrics(&promMetrics{
		counters: map[string]prometheus.Counter{},
		gauges:   map[string]prometheus.Gauge{},
	})

	go func() {
		http.Handle("/metrics", promhttp.Handler())
		if err := http.ListenAndServe(metricsAddr, nil); err != nil {
			log.Printf("metrics server error: %v", err)
		}
	}()

	var h handler
	if err, _ := connaxis.Serve(&h, path); err != nil {
		log.Fatal(err)
	}
}
