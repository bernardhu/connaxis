# Prometheus 示例

启动一个 echo server，并在 `/metrics` 暴露指标。

```sh
go run ./examples/prometheus -p conf/connaxis.conf -metrics :2112
```

查看：

```sh
curl http://127.0.0.1:2112/metrics
```
