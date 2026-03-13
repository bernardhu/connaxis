# Configuration

`connaxis` loads JSON config into `EvConfig`.

## Example

```json
{
  "ncpu": -1,
  "lbStrategy": "rr",
  "sslPem": "",
  "sslKey": "",
  "sslMode": "",
  "tlsEngine": "atls",
  "ktlsPolicy": "tls12-tx",
  "bufSize": 1048576,
  "chanSize": 8192,
  "pktSizeLimit": 67108864,
  "cliSbufLimit": 49152,
  "maxAcceptFD": -1,
  "pollWait": -1,
  "idleCheckInt": 1,
  "idleLimit": 0,
  "printStat": false,
  "listenAddrs": ["tcp://:5000?reuseport=false"]
}
```

## Fields

- `ncpu`: worker loop count. `-1` means `runtime.NumCPU()`.
- `lbStrategy`: `rr` / `rand` / `lru` / `hash`.
- `sslPem`: path to PEM certificate.
- `sslKey`: path to PEM key.
- `sslMode`: `tls` (other modes are not supported).
- `tlsEngine`: `atls` (default) or `ktls`.
- `ktlsPolicy`: used only when effective `tlsEngine` is `ktls`.
  - `tls12-tx` (default): force TLS1.2 and enable kTLS TX only.
  - `tls13-tx`: force TLS1.3 and enable kTLS TX only.
  - `tls12-rxtx`: force TLS1.2 and enable kTLS RX/TX.
  - `tls13-rxtx`: force TLS1.3 and enable kTLS RX/TX.
- `listenAddrs`: array of endpoint strings in form `tcp://:5000?reuseport=false`.
- `bufSize`: shared read buffer size per loop.
- `chanSize`: loop channel size.
- `pktSizeLimit`: maximum packet size in bytes.
- `cliSbufLimit`: per-connection pending write limit (server-side connections).
- `maxAcceptFD`: max accepted connections before overload handling.
- `pollWait`: poll wait in milliseconds (`-1` means block).
- `idleCheckInt`: idle check interval in seconds.
- `idleLimit`: idle timeout in seconds (`0` disables).
- `printStat`: enable periodic stats logging.

## Notes

- `listenAddrs` is parsed into internal endpoints. If you customize loading, ensure entries are converted into `EVEndpoint` values.
