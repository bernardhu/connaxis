# 示例

## Echo

```sh
go run ./examples/echo -p conf/connaxis.conf
```

## TLS Echo

```sh
go run ./examples/tls_echo -p conf/connaxisssl.conf
```

## HTTP

```sh
go run ./examples/http -p conf/connaxis.conf
```

可选参数：

```sh
  -workers N   http worker count (default: GOMAXPROCS)
  -queue   N   http job queue size
```

## HTTP + WebSocket

```sh
go run ./examples/httpws -p conf/connaxis.conf
```

可选参数：

```sh
  -workers N   http worker count (default: GOMAXPROCS)
  -queue   N   http job queue size
```

测试 HTTP：

```sh
curl -v http://127.0.0.1:5000/
```

测试 WebSocket（二选一）：

```sh
websocat ws://127.0.0.1:5000/
wscat -c ws://127.0.0.1:5000/
```

## TCP + HTTP + WebSocket

```sh
go run ./examples/tcphttpws -p conf/connaxis.conf
```

可选参数：

```sh
  -workers N   http worker count (default: GOMAXPROCS)
  -queue   N   http job queue size
```

测试 HTTP：

```sh
curl -v http://127.0.0.1:5000/
```

测试 WebSocket：

```sh
websocat ws://127.0.0.1:5000/
```

测试 TCP：

```sh
nc 127.0.0.1 5000
```

## FastHTTP

```sh
go run ./examples/fasthttp -p conf/connaxis.conf
```

可选参数：

```sh
  -workers N   http worker count (default: GOMAXPROCS)
  -queue   N   http job queue size
```

## WebSocket

```sh
go run ./examples/websocket -p conf/connaxis.conf
```

## Prometheus

```sh
go run ./examples/prometheus -p conf/connaxis.conf -metrics :2112
```
