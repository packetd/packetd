# packetd

packetd 是一个基于 `libpcap` 的聚焦于应用层协议网络数据包观测工具，可以从数据流中解析出应用协议，以 ***RoundTrip* 作为其核心概念，即请求的来回。

packetd 提供了更加现代化的可观测手段，如 Prometheus/VictoriaMetrics/OpenTelemetry，可通过 HTTP 请求将 Metrics/Traces 数据发送至 Prometheus/OT-Collector 进行持久化存储。 

## Installation

```shell
$ go install github.com/packetd/packetd@latest
```

## Quickstart

packetd 目前提供了两种运行模式，agent 和 log，前者使用 agent 模式持续监听网络包并工作，后者作为一种 cli 工具临时 debug 网络数据包。

```shell
packetd -h
# packetd is a eBPF-powered network traffic capture and analysis tool

Usage:
  packetd [command]

Available Commands:
  agent       Run packetd as a network monitoring agent
  help        Help about any command
  log         Capture and log network traffic based on protocol configurations

Flags:
  -h, --help   help for packetd

Use "packetd [command] --help" for more information about a command.
```

### agent mode

## log mode

```shell
$ packetd log --ifaces any  --proto 'http;80'
2025-06-22 15:58:25.278 INFO    logger/logger.go:136    sniffer add device (any), address=[]

# 在另外的 terminal 执行 `curl baidu.com`
# cat roundtrip.log
cat roundtrips.log | jq .
{
  "Request": {
    "Port": 36418,
    "Method": "GET",
    "Header": {
      "Accept": [
        "*/*"
      ],
      "User-Agent": [
        "curl/7.61.1"
      ]
    },
    "Proto": "HTTP/1.1",
    "Path": "/",
    "URL": "/",
    "Scheme": "",
    "RemoteHost": "baidu.com",
    "Close": false,
    "Size": 0,
    "Chunked": false,
    "Time": "2025-06-22T16:01:41.044276916+08:00"
  },
  "Response": {
    "Port": 80,
    "Header": {
      "Accept-Ranges": [
        "bytes"
      ],
      "Cache-Control": [
        "max-age=86400"
      ],
      "Connection": [
        "Keep-Alive"
      ],
      "Content-Length": [
        "81"
      ],
      "Content-Type": [
        "text/html"
      ],
      "Date": [
        "Sun, 22 Jun 2025 08:01:41 GMT"
      ],
      "Etag": [
        "\"51-47cf7e6ee8400\""
      ],
      "Expires": [
        "Mon, 23 Jun 2025 08:01:41 GMT"
      ],
      "Last-Modified": [
        "Tue, 12 Jan 2010 13:48:00 GMT"
      ],
      "Server": [
        "Apache"
      ]
    },
    "Status": "200 OK",
    "StatusCode": 200,
    "Proto": "HTTP/1.1",
    "Close": false,
    "Size": 81,
    "Chunked": false,
    "Time": "2025-06-22T16:01:41.135450645+08:00"
  },
  "Duration": "91.173729ms"
}
```

## Protocol

支持的协议列表，参见 [./protocol](./protocol)

- amqp
- dns
- grpc
- http
- http2
- kafka
- mongodb
- mysql
- postgresql
- redis

## Benchmark

压测程序位于 [packetd-benchmark](https://github.com/packetd/packetd-benchmark)

### HTTP

```shell
$ ./packetd agent --config packetd.yaml
2025-06-22 16:21:33.197	INFO	logger/logger.go:136	sniffer add device (any), address=[]
2025-06-22 16:21:33.199	INFO	logger/logger.go:136	server listening on :9091

# 启动 HTTP Server
root@localhost:~/projects/golang/packetd-benchmark/http/server# go run .
2025/06/22 16:22:24 server listening on localhost:8083

# --- ROUND1

# 启动 HTTP Client
./client -body_size 10KB -total 100000 -workers 10 -duration 0s
...
2025/06/22 16:21:38 Total 100000 requests take 2.984687081s, qps=33504.349798, bps=2618Mib

# 访问 packetd /protocol/metrics 接口
curl -s  localhost:9091/protocol/metrics | grep total
http_requests_total{server_port="8083",method="GET",path="/benchmark",status_code="200"} 100000.000000

# --- ROUND2
./client -body_size 100KB -total 100000 -workers 20 -duration 0s
...
2025/06/22 16:24:32 Total 100000 requests take 2.660366126s, qps=37588.811188, bps=28.68Gib

# 访问 packetd /protocol/metrics 接口
curl -s  localhost:9091/protocol/metrics | grep total
http_requests_total{server_port="8083",method="GET",path="/benchmark",status_code="200"} 100000.000000
```

### Redis

```shell
root@localhost:~/projects/golang/packetd-benchmark/redis/client# ./client -cmd ping -total 100000 -workers 10
...
2025/06/22 17:54:26 Total 100000 requests take 1.057603411s, qps=94553.401549, bps=738.7Mib

curl -s  localhost:9091/protocol/metrics | grep total
redis_request_total{} 100010.000000
```

## License

Apache [©packetd](https://github.com/packetd/packetd/blob/master/LICENSE)
