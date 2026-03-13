package tuning

// WebSocket knobs (0 = unlimited).
var (
	// WSRequireClientMask enforces RFC6455: client->server frames must be masked.
	WSRequireClientMask = true

	// WSMaxFramePayloadBytes limits a single frame payload size.
	WSMaxFramePayloadBytes = 16 * 1024 * 1024

	// WSMaxMessageBytes limits a reassembled fragmented message size.
	WSMaxMessageBytes = 16 * 1024 * 1024
)
