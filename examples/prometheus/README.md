# Prometheus Example

Starts an echo server and exports metrics on `/metrics`.

```sh
go run ./examples/prometheus -p conf/connaxis.conf -metrics :2112
```

Open:

```sh
curl http://127.0.0.1:2112/metrics
```
