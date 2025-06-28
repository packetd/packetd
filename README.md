# packetd

packetd 是一个基于 `libpcap` 的**应用层协议**网络数据无侵观测项目。

packetd 支持从数据流中解析出多种应用协议（HTTP/Grpc/MySQL/Redis/...），使用请求的来回 **RoundTrip** 作为其核心概念，进而衍生出 **Traces/Metrics** 数据。

但由于缺乏上下文关联，Traces 仅能代表当次网络情况的情况，无法关联应用层的 Span，更像是一种 Event/Log 类型的数据，只不过以 Traces 的形式组织起来。

packetd 提供了更加现代化的可观测手段，可以无缝地对接现有的观测体系：
- 支持 Prometheus RemoteWrite 协议上报 Metrics 数据。
- 支持 VictoriaMetrics VmRange Histogram，无需提前定义 bucket。
- 支持 OpenTelemetry 协议上报 Traces 数据。

## 🔰 Installation

```shell
$ go install github.com/packetd/packetd@latest
```

## 🚀 Quickstart

packetd 提供了 agent 和 log 两种运行模式，前者使用 agent 模式持续监听网络包并工作，后者作为一种 cli 工具可以临时 debug 网络请求。

```shell
$ packetd
# packetd is a eBPF-powered network traffic capture and analysis tool

Usage:
  packetd [command]

Available Commands:
  agent       Run in network monitoring agent mode
  config      Prints the reference configuration
  help        Help about any command
  ifaces      List all available interfaces
  log         Capture and log network traffic roundtrips

Flags:
  -h, --help   help for packetd

Use "packetd [command] --help" for more information about a command.
```

packetd 项目启动需要指定配置文件，log 模式本质上是内置以一份配置模版，详见 [#logConfig](cmd/log.go)。可通过 `packetd config` 子命令查看所有配置项内容。

可以先通过 ifaces 子命令查看支持监听的网卡设备，如：

```shell
$ packetd ifaces
- lo: [127.0.0.1/8 ::1/128]
- ens160: [172.16.22.128/24 fe80::20c:29ff:fe11:428c/64]
- docker0: [172.17.0.1/16]
- br-05d5cdd6d4c9: [172.18.0.1/16]
```

### log mode

这里以 log 模式作为示例，展示 packetd 的工作模式及输出内容（报错可尝试使用 root 权限执行）：

```shell
# start packetd

$ packetd log --ifaces any  --proto 'http;80'
2025-06-22 15:58:25.278 INFO    logger/logger.go:136    sniffer add device (any), address=[]
```

接着在新的 terminal 中访问任意 80 端口的 HTTP 服务，这里以 `baidu.com` 为例：
```shell
2025-06-28 16:26:32.749	INFO	logger/logger.go:136	sniffer add device (any), address=[]
{"Request":{"Host":"172.16.22.128","Port":55172,"Method":"GET","Header":{"Accept":["*/*"],"User-Agent":["curl/8.2.1"]},"Proto":"HTTP/1.1","Path":"/","URL":"/","Scheme":"","RemoteHost":"baidu.com","Close":false,"Size":0,"Chunked":false,"Time":"2025-06-28T16:26:39.64617881+08:00"},"Response":{"Host":"182.61.244.181","Port":80,"Header":{"Accept-Ranges":["bytes"],"Cache-Control":["max-age=86400"],"Connection":["Keep-Alive"],"Content-Length":["81"],"Content-Type":["text/html"],"Date":["Sat, 28 Jun 2025 08:26:39 GMT"],"Etag":["\"51-47cf7e6ee8400\""],"Expires":["Sun, 29 Jun 2025 08:26:39 GMT"],"Last-Modified":["Tue, 12 Jan 2010 13:48:00 GMT"],"Server":["Apache"]},"Status":"200 OK","StatusCode":200,"Proto":"HTTP/1.1","Close":false,"Size":81,"Chunked":false,"Time":"2025-06-28T16:26:39.757873402+08:00"},"Duration":"111.694592ms"}
```

`jq` 格式化查看：
```json
{
    "Request": {
        "Host": "172.16.22.128",
        "Port": 55172,
        "Method": "GET",
        "Header": {
            "Accept": [
                "*/*"
            ],
            "User-Agent": [
                "curl/8.2.1"
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
        "Time": "2025-06-28T16:26:39.64617881+08:00"
    },
    "Response": {
        "Host": "182.61.244.181",
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
                "Sat, 28 Jun 2025 08:26:39 GMT"
            ],
            "Etag": [
                "\"51-47cf7e6ee8400\""
            ],
            "Expires": [
                "Sun, 29 Jun 2025 08:26:39 GMT"
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
        "Time": "2025-06-28T16:26:39.757873402+08:00"
    },
    "Duration": "111.694592ms"
}
```

packetd 捕获了一个完整的 HTTP 请求，并结构化地输出请求明细，考虑到请求体和响应体的内容可能会比较多，这里仅记录了 BodySize，除了输出到 console，还可以输出到指定文件。

```shell
$ packetd log -h
Capture and log network traffic roundtrips

Usage:
  packetd log [flags]

Examples:
# packetd log --proto 'http;80,8080' --proto 'dns;53' --ifaces any --console

Flags:
      --console            Enable console logging
  -h, --help               help for log
      --ifaces string      Network interfaces to monitor (supports regex), 'any' for all interfaces (default "any")
      --ipv4               Capture IPv4 traffic only
      --log.backups int    Maximum number of old log files to retain (default 10)
      --log.file string    Path to log file (default "roundtrips.log")
      --log.size int       Maximum size of log file in MB (default 100)
      --no-promiscuous     Don't put the interface into promiscuous mode
      --pcap.file string   Path to pcap file to read from
      --proto strings      Protocols to capture in 'protocol;ports[;host]' format, multiple protocols supported
```

packetd 除了支持从网卡直接捕获网络数据，还支持加载 pcap 文件，如：

```shell
$ packetd log --pcap.file /tmp/app.pcp --console
```

### agent-mode

agent 模式则需要显式指定配置文件，默认为 `packetd.yaml`，启动命令 `packetd --config packetd.yaml`

```yaml
# packetd.yaml
server:
  enabled: true
  address: ":9091"

logger:
  stdout: true

controller:
  layer4Metrics:
    enabled: true

sniffer.ifaces: 'any'
sniffer.engine: pcap
sniffer.protocols:
  rules:
    - name: "http"
      protocol: "http"
      ports: [80]

processor:
  - name: roundtripstometrics
    config:
      http:
        requireLabels:
          - "server.host"
          - "server.port"
          - "request.method"
          - "request.path"
          - "response.status_code"

  - name: roundtripstotraces
    config:

pipeline:
  - name: "metrics/common"
    processors:
      - roundtripstometrics

metricsStorage:
  enabled: true
  vmHistogram: true

# 这里仅做指标暴露 不输出其他任何内容 详细配置参见 packetd.reference.yaml
exporter:
```

同样在新终端中访问任意 80 端口的 HTTP 服务，如：
```shell
$ curl baidu.com
<html>
<meta http-equiv="refresh" content="0;url=http://www.baidu.com/">
</html>

$ curl baidu.com/hello/world
<!DOCTYPE HTML PUBLIC "-//IETF//DTD HTML 2.0//EN">
<html><head>
<title>302 Found</title>
</head><body>
<h1>Found</h1>
<p>The document has moved <a href="http://www.baidu.com/search/error.html">here</a>.</p>
</body></html>
```

访问 9091 端口查看 `/protocol/metrics` API 的打点统计。
```shell
$ curl localhost:9091/protocol/metrics
http_requests_total{server_port="80",method="GET",path="/",status_code="200"} 1.000000
http_requests_total{server_port="80",method="GET",path="/hello/world",status_code="302"} 1.000000
layer4_packets_total{} 24.000000
layer4_bytes_total{} 1048.000000
http_request_duration_seconds_bucket{server_port="80",method="GET",path="/",status_code="200",vmrange="1.136e-01...1.292e-01"} 1.000000
http_request_duration_seconds_sum{server_port="80",method="GET",path="/",status_code="200"} 0.116916
http_request_duration_seconds_count{server_port="80",method="GET",path="/",status_code="200"} 1.000000
http_request_duration_seconds_bucket{server_port="80",method="GET",path="/hello/world",status_code="302",vmrange="1.000e-01...1.136e-01"} 1.000000
http_request_duration_seconds_sum{server_port="80",method="GET",path="/hello/world",status_code="302"} 0.111345
http_request_duration_seconds_count{server_port="80",method="GET",path="/hello/world",status_code="302"} 1.000000
http_request_body_bytes_bucket{server_port="80",method="GET",path="/",status_code="200",vmrange="0...1.000e-09"} 1.000000
http_request_body_bytes_sum{server_port="80",method="GET",path="/",status_code="200"} 0.000000
http_request_body_bytes_count{server_port="80",method="GET",path="/",status_code="200"} 1.000000
http_request_body_bytes_bucket{server_port="80",method="GET",path="/hello/world",status_code="302",vmrange="0...1.000e-09"} 1.000000
http_request_body_bytes_sum{server_port="80",method="GET",path="/hello/world",status_code="302"} 0.000000
http_request_body_bytes_count{server_port="80",method="GET",path="/hello/world",status_code="302"} 1.000000
http_response_body_bytes_bucket{server_port="80",method="GET",path="/hello/world",status_code="302",vmrange="2.154e+02...2.448e+02"} 1.000000
http_response_body_bytes_sum{server_port="80",method="GET",path="/hello/world",status_code="302"} 222.000000
http_response_body_bytes_count{server_port="80",method="GET",path="/hello/world",status_code="302"} 1.000000
http_response_body_bytes_bucket{server_port="80",method="GET",path="/",status_code="200",vmrange="7.743e+01...8.799e+01"} 1.000000
http_response_body_bytes_sum{server_port="80",method="GET",path="/",status_code="200"} 81.000000
http_response_body_bytes_count{server_port="80",method="GET",path="/",status_code="200"} 1.000000
```

## 📝 Configuration

建议使用 `packetd config > packetd.yaml` 命令生成一个样例文件并按需进行调整，样例文件已对各项配置进行了详细说明。

详细配置参见 [#Config Reference](./cmd/static/packetd.reference.yaml)

## 💡 Protocol

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

## 🔍 Observation

packetd 遵循了 Prometheus 以及 OpenTelemetry 的 Metrics / Traces 使用指南，可通过配置文件的开关选择是否打开数据的上报功能，对于指标提供了 /metrics 接口以及 remotewrite 两种形式。

详细内容参见 [#Obveration](./docs/observation.md)

## 🏅 Benchmark

pakcetd 支持的每种协议都进行了压测，并输出了相应的压测报告。

详细内容参见 [#Benchamark](./docs/benchmark.md)

## 🤔 Limitation

## 🗂 Roadmap

- 支持 stats 模式
- 内置 web 可视化方案
- kubernetes 部署支持
- 更多的协议支持

## 🔖 License

Apache [©packetd](https://github.com/packetd/packetd/blob/master/LICENSE)
