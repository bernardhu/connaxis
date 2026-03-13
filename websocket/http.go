package websocket

var headerHost = []byte("Host")
var headerUpgrade = []byte("Upgrade")
var headerConnection = []byte("Connection")
var headerSecVersion = []byte("Sec-Websocket-Version")
var headerSecKey = []byte("Sec-Websocket-Key")

var headerSecProtocol = []byte("Sec-Websocket-Protocol")
var headerSecExtensions = []byte("Sec-Websocket-Extensions")

const (
	headerSeenHost = 1 << iota
	headerSeenUpgrade
	headerSeenConnection
	headerSeenSecVersion
	headerSeenSecKey

	// headerSeenAll is the value that we expect to receive at the end of
	// headers read/parse loop.
	headerSeenAll = 0 |
		headerSeenHost |
		headerSeenUpgrade |
		headerSeenConnection |
		headerSeenSecVersion |
		headerSeenSecKey
)

const WSHeaderMinSize = 2

var (
	specHeaderValueWsLower      = []byte("websocket")
	specHeaderValueUpgradeLower = []byte("upgrade")
	specHeaderValueSecVersion   = []byte("13")
)

type httpRequestLine struct {
	method, uri  []byte
	major, minor int
}
