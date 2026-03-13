# websocket 协议介绍
[websocket.pdf](https://www.rfc-editor.org/rfc/pdfrfc/rfc6455.txt.pdf)
websocket是工作再http协议上面的一层应用协议，普通http请求通过握手将协议update成为websocket协议以后，采用frame的形式和后端交互

## 握手协议
客户端通过普通http请求里面设置upgrade来通知后端要采用websocket协议，例子如下

    GET /chat HTTP/1.1
    Host: server.example.com
    Upgrade: websocket
    Connection: Upgrade
    Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==
    Origin: http://example.com
    Sec-WebSocket-Protocol: chat, superchat
    Sec-WebSocket-Version: 13
服务器响应客户端一个协议包以完成握手协议，例子如下

    HTTP/1.1 101 Switching Protocols
    Upgrade: websocket
    Connection: Upgrade
    Sec-WebSocket-Accept: s3pPLMBiTxaQ9kYGzzhZRbK+xOo=
    Sec-WebSocket-Protocol: chat
	
## 传输协议
完成握手以后，采用二进制协议传输数据，格式如下：

    |0              |1              |2              |3              |
    |0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1|
    +-+-+-+-+-------+-+-------------+-------------------------------+
    |F|R|R|R| opcode|M| Payload len |    Extended payload length    |
    |I|S|S|S|  (4)  |A|     (7)     |         (16/64)               |
    |N|V|V|V|       |S|             |   (if payload len==126/127)   |
    | |1|2|3|       |K|             |                               |
    +-+-+-+-+-------+-+-------------+ - - - - - - - - - - - - - - - +
    |   Extended payload length continued, if payload len == 127    |
    + - - - - - - - - - - - - - - - +-------------------------------+
    |                               | Masking-key, if MASK set to 1 |
    +-------------------------------+-------------------------------+
    | Masking-key (continued)       |          Payload Data         |
    +-------------------------------- - - - - - - - - - - - - - - - |
    :                   Payload Data continued ...                  :
    | - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - - |
    |                   Payload Data continued ...                  |
    +---------------------------------------------------------------+
> FIN: 1 bit

    表明是否是最后一个分片（一个数据包可以分片为多个）
> RSV1, RSV2, RSV3: 1 bit each

    MUST be 0 unless an extension is negotiated that defines meanings
    for non-zero values. If a nonzero value is received and none of
    the negotiated extensions defines the meaning of such a nonzero
    value, the receiving endpoint MUST _Fail the WebSocket
    Connection_.
> Opcode: 4 bits

    定义payload内容，如果非以下字段，断开连接
 * %x0 denotes a continuation frame
 * %x1 denotes a text frame
 * %x2 denotes a binary frame
 * %x3-7 are reserved for further non-control frames
 * %x8 denotes a connection close
 * %x9 denotes a ping
 * %xA denotes a pong
 * %xB-F are reserved for further control frames

> Mask: 1 bit

    Defines whether the "Payload data" is masked. If set to 1, a
    masking key is present in masking-key, and this is used to unmask
    the "Payload data" as per Section 5.3. All frames sent from
    client to server have this bit set to 1.
    Payload length: 7 bits, 7+16 bits, or 7+64 bits
    The length of the "Payload data", in bytes: if 0-125, that is the
    payload length. If 126, the following 2 bytes interpreted as a
    16-bit unsigned integer are the payload length. If 127, the
    following 8 bytes interpreted as a 64-bit unsigned integer (the
    most significant bit MUST be 0) are the payload length. Multibyte
    length quantities are expressed in network byte order. Note that
    in all cases, the minimal number of bytes MUST be used to encode
    the length, for example, the length of a 124-byte-long string
    can’t be encoded as the sequence 126, 0, 124. The payload length
    is the length of the "Extension data" + the length of the
    "Application data". The length of the "Extension data" may be
    zero, in which case the payload length is the length of the
    "Application data".
 
> Masking-key: 0 or 4 bytes

    All frames sent from the client to the server are masked by a
    32-bit value that is contained within the frame. This field is
    present if the mask bit is set to 1 and is absent if the mask bit
    is set to 0. See Section 5.3 for further information on client-
    to-server masking.
 
> Payload data: (x+y) bytes

    The "Payload data" is defined as "Extension data" concatenated
    with "Application data".
 
> Extension data: x bytes

    The "Extension data" is 0 bytes unless an extension has been
    negotiated. Any extension MUST specify the length of the
    "Extension data", or how that length may be calculated, and how
    the extension use MUST be negotiated during the opening handshake.
    If present, the "Extension data" is included in the total payload
    length.
 
> Application data: y bytes

    Arbitrary "Application data", taking up the remainder of the frame
    after any "Extension data". The length of the "Application data"
    is equal to the payload length minus the length of the "Extension
    data".