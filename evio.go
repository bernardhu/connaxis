package connaxis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/bernardhu/connaxis/eventloop"
	"github.com/bernardhu/connaxis/pool"
	"github.com/bernardhu/connaxis/wrapper"
)

// LoadBalance sets the load balancing method.
type LoadBalance int

const (
	// RoundRobin requests that connections are distributed to a loop in a
	// round-robin fashion.
	RoundRobin LoadBalance = iota

	// Random requests that connections are randomly distributed.
	Random

	// LeastConnections assigns the next accepted connection to the loop with
	// the least number of active connections.
	LeastConnections

	Hash
)

// Serve starts handling events for the specified addresses.
//
// Addresses should use a scheme prefix and be formatted
// like `tcp://192.168.0.10:9851`.
//
// Only the "tcp" network scheme is supported.
func Serve(h eventloop.IHandler, fname string) (error, *Server) {
	config, err := load(fname)
	if err != nil {
		return err, nil
	}
	return ServeByConfig(h, config, true)
}

func ServeByConfig(h eventloop.IHandler, config *EvConfig, standalone bool) (error, *Server) {
	if config == nil {
		return errors.New("nil config"), nil
	}
	pool.Setup(17) //max 64M
	return serve(h, config, standalone)
}

type EvConfig struct {
	Ncpu                      int                     `json:"ncpu"`
	LbStrategy                string                  `json:"lbStrategy"`
	SslPem                    string                  `json:"sslPem"`
	SslKey                    string                  `json:"sslKey"`
	SslOcspStaple             string                  `json:"sslOcspStaple"`
	SslMode                   string                  `json:"sslMode"`
	TlsEngine                 string                  `json:"tlsEngine"`
	KTLSPolicy                string                  `json:"ktlsPolicy"`
	TlsMinVersion             uint16                  `json:"tlsMinVersion"`
	TlsMaxVersion             uint16                  `json:"tlsMaxVersion"`
	TlsNextProtos             []string                `json:"tlsNextProtos"`
	TlsSessionTicketsDisabled bool                    `json:"tlsSessionTicketsDisabled"`
	TlsSessionTicketKeyFile   string                  `json:"tlsSessionTicketKeyFile"`
	ListenAddrs               []eventloop.IEVEndpoint `json:"listenAddrs"`
	PollWait                  int                     `json:"pollWait"`
	TwInterval                int                     `json:"twInterval"`
	DialKeepAlive             bool                    `json:"dialKeepAlive"`
	DialPolling               bool                    `json:"dialPolling"`

	PktSizeLimit    int  `json:"pktSizeLimit"`
	BufSize         int  `json:"bufSize"`
	ChanSize        int  `json:"chanSize"`
	MaxAcceptFD     int  `json:"maxAcceptFD"`
	IdleCheckInt    int  `json:"idleCheckInt"`
	IdleLimit       int  `json:"idleLimit"`
	CliSendBufLimit int  `json:"cliSbufLimit"`
	PrintStat       bool `json:"printStat"`

	IDGen eventloop.IDGenerator
}

func GetDefaultConfig() *EvConfig {
	var cfg EvConfig
	cfg.Ncpu = -1
	cfg.LbStrategy = "hash"
	cfg.TlsEngine = "atls"
	cfg.KTLSPolicy = "tls12-tx"
	cfg.BufSize = 1048576
	cfg.ChanSize = 8192
	cfg.PktSizeLimit = 64 * 1024 * 1024
	cfg.CliSendBufLimit = 48 * 1024
	cfg.MaxAcceptFD = -1
	cfg.ListenAddrs = []eventloop.IEVEndpoint{}
	cfg.PollWait = -1    //100 比较好
	cfg.IdleCheckInt = 1 //sec
	cfg.IdleLimit = 0
	return &cfg
}

func load(name string) (*EvConfig, error) {
	config := EvConfig{}
	bytes, err := os.ReadFile(name)
	if err != nil || len(bytes) == 0 {
		return nil, fmt.Errorf("can't load config: name:%s err:%v", name, err)
	}
	if err := json.Unmarshal(bytes, &config); err != nil {
		return nil, fmt.Errorf("fail to parse config: name:%s err:%v", name, err)
	}
	wrapper.Infof("load EvConfig from file:%s", name)
	return &config, nil
}

func parseAddr(addr string) (network, address string, err error) {
	network = "tcp"
	address = addr

	res := strings.Split(addr, "://")
	if len(res) != 2 {
		return
	}

	network = strings.ToLower(strings.TrimSpace(res[0]))
	if network == "" {
		network = "tcp"
	}
	if network != "tcp" {
		err = fmt.Errorf("unsupported network %q: only tcp is supported", network)
		return
	}

	left := strings.SplitN(res[1], "?", 2)
	address = left[0]

	return
}

type EVEndpoint struct {
	Address string
	Net     string
	Reuse   bool
	Ctx     context.Context
}

func (ep *EVEndpoint) Network() string {
	return ep.Net
}

func (ep *EVEndpoint) String() string {
	return ep.Address
}

func (ep *EVEndpoint) IsReuse() bool {
	return ep.Reuse
}

func (ep *EVEndpoint) GetContext() interface{} {
	return ep.Ctx
}
