# Quickstart

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

packetd 项目启动需要指定配置文件，log 模式本质上是以一份内置配置模版运行，详见 [#logConfig](../cmd/log.go)。可通过 `packetd config` 子命令查看所有配置项内容。

可以先通过 ifaces 子命令查看支持监听的网卡设备，如：

```shell
$ packetd ifaces
- lo: [127.0.0.1/8 ::1/128]
- ens160: [172.16.22.128/24 fe80::20c:29ff:fe11:428c/64]
- docker0: [172.17.0.1/16]
- br-05d5cdd6d4c9: [172.18.0.1/16]
```

## log mode

这里以 log 模式作为示例，展示 packetd 的工作模式及输出内容（如若报错可尝试使用 root 权限执行）

```shell
$ packetd log --ifaces any  --proto 'http;80' --console
2025-06-22 15:58:25.278 INFO    logger/logger.go:136    sniffer add device (any), address=[]
```

接着在任意 terminal 中访问任意 80 端口的 HTTP 服务，以 `google.com` 为例：

```shell
2025-06-28 16:26:32.749	INFO	logger/logger.go:136	sniffer add device (any), address=[]
{"Request":{"Host":"172.16.22.128","Port":45446,"Method":"GET","Header":{"Accept":["*/*"],"User-Agent":["curl/8.2.1"]},"Proto":"HTTP/1.1","Path":"/","URL":"/","Scheme":"","RemoteHost":"google.com","Close":false,"Size":0,"Chunked":false,"Time":"2025-06-29T01:51:32.341778618+08:00"},"Response":{"Host":"8.7.198.46","Port":80,"Header":{"Cache-Control":["public, max-age=2592000"],"Content-Length":["219"],"Content-Security-Policy-Report-Only":["object-src 'none';base-uri 'self';script-src 'nonce-94QUfpHFUYNEdrQmxRB42g' 'strict-dynamic' 'report-sample' 'unsafe-eval' 'unsafe-inline' https: http:;report-uri https://csp.withgoogle.com/csp/gws/other-hp"],"Content-Type":["text/html; charset=UTF-8"],"Date":["Sat, 28 Jun 2025 17:51:32 GMT"],"Expires":["Mon, 28 Jul 2025 17:51:32 GMT"],"Location":["http://www.google.com/"],"Server":["gws"],"X-Frame-Options":["SAMEORIGIN"],"X-Xss-Protection":["0"]},"Status":"301 Moved Permanently","StatusCode":301,"Proto":"HTTP/1.1","Close":false,"Size":219,"Chunked":false,"Time":"2025-06-29T01:51:32.50632449+08:00"},"Duration":"164.545872ms"}
```

`jq` 格式化查看：
```json
{
  "Request": {
    "Host": "172.16.22.128",
    "Port": 45446,
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
    "RemoteHost": "google.com",
    "Close": false,
    "Size": 0,
    "Chunked": false,
    "Time": "2025-06-29T01:51:32.341778618+08:00"
  },
  "Response": {
    "Host": "8.7.198.46",
    "Port": 80,
    "Header": {
      "Cache-Control": [
        "public, max-age=2592000"
      ],
      "Content-Length": [
        "219"
      ],
      "Content-Security-Policy-Report-Only": [
        "object-src 'none';base-uri 'self';script-src 'nonce-94QUfpHFUYNEdrQmxRB42g' 'strict-dynamic' 'report-sample' 'unsafe-eval' 'unsafe-inline' https: http:;report-uri https://csp.withgoogle.com/csp/gws/other-hp"
      ],
      "Content-Type": [
        "text/html; charset=UTF-8"
      ],
      "Date": [
        "Sat, 28 Jun 2025 17:51:32 GMT"
      ],
      "Expires": [
        "Mon, 28 Jul 2025 17:51:32 GMT"
      ],
      "Location": [
        "http://www.google.com/"
      ],
      "Server": [
        "gws"
      ],
      "X-Frame-Options": [
        "SAMEORIGIN"
      ],
      "X-Xss-Protection": [
        "0"
      ]
    },
    "Status": "301 Moved Permanently",
    "StatusCode": 301,
    "Proto": "HTTP/1.1",
    "Close": false,
    "Size": 219,
    "Chunked": false,
    "Time": "2025-06-29T01:51:32.50632449+08:00"
  },
  "Duration": "164.545872ms"
}
```

packetd 捕获了一个完整的 HTTP 请求，并结构化地输出请求明细，考虑到请求体和响应体的内容可能会比较多，这里仅记录了 BodySize。

除了输出到 console，还可以输出到指定文件，如 `--log.file roundtrips.log`。

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
$ packetd log --pcap.file /my/app.pcap --console
```

## agent-mode

agent 模式则需要显式指定配置文件，默认为 `packetd.yaml`，启动命令 `packetd agent --config packetd.yaml`。

```yaml
# packetd.yaml
server:
  enabled: true
  address: ":9091"

logger.stdout: true
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

pipeline:
  - name: "metrics/common"
    processors:
      - roundtripstometrics

metricsStorage:
  enabled: true

# 这里仅做指标暴露 不输出其他任何内容 详细配置参见 packetd.reference.yaml
exporter:
```

同样在新终端中访问任意 80 端口的 HTTP 服务，如：

```shell
$ curl httpbin.org
```

访问 9091 端口查看 `/protocol/metrics` API 的打点统计。每种协议都默认提供了包括请求耗时、请求体大小、响应体大小、请求总数等指标。

```shell
$ curl localhost:9091/protocol/metrics
http_requests_total{server_port="80",method="GET",path="/",status_code="200"} 1.000000
layer4_packets_total{} 24.000000
layer4_bytes_total{} 10425.000000
http_request_body_bytes_bucket{server_port="80",method="GET",path="/",status_code="200",le="10240"} 1.000000
http_request_body_bytes_bucket{server_port="80",method="GET",path="/",status_code="200",le="102400"} 1.000000
http_request_body_bytes_bucket{server_port="80",method="GET",path="/",status_code="200",le="256000"} 1.000000
http_request_body_bytes_bucket{server_port="80",method="GET",path="/",status_code="200",le="512000"} 1.000000
http_request_body_bytes_bucket{server_port="80",method="GET",path="/",status_code="200",le="1048576"} 1.000000
http_request_body_bytes_bucket{server_port="80",method="GET",path="/",status_code="200",le="5242880"} 1.000000
http_request_body_bytes_bucket{server_port="80",method="GET",path="/",status_code="200",le="10485760"} 1.000000
http_request_body_bytes_bucket{server_port="80",method="GET",path="/",status_code="200",le="20971520"} 1.000000
http_request_body_bytes_bucket{server_port="80",method="GET",path="/",status_code="200",le="31457280"} 1.000000
http_request_body_bytes_bucket{server_port="80",method="GET",path="/",status_code="200",le="52428800"} 1.000000
http_request_body_bytes_bucket{server_port="80",method="GET",path="/",status_code="200",le="83886080"} 1.000000
http_request_body_bytes_bucket{server_port="80",method="GET",path="/",status_code="200",le="104857600"} 1.000000
http_request_body_bytes_bucket{server_port="80",method="GET",path="/",status_code="200",le="157286400"} 1.000000
http_request_body_bytes_bucket{server_port="80",method="GET",path="/",status_code="200",le="209715200"} 1.000000
http_request_body_bytes_bucket{server_port="80",method="GET",path="/",status_code="200",le="524288000"} 1.000000
http_request_body_bytes_bucket{server_port="80",method="GET",path="/",status_code="200",le="+Inf"} 1.000000
http_request_body_bytes_sum{server_port="80",method="GET",path="/",status_code="200"} 0.000000
http_request_body_bytes_count{server_port="80",method="GET",path="/",status_code="200"} 1.000000
http_response_body_bytes_bucket{server_port="80",method="GET",path="/",status_code="200",le="10240"} 1.000000
http_response_body_bytes_bucket{server_port="80",method="GET",path="/",status_code="200",le="102400"} 1.000000
http_response_body_bytes_bucket{server_port="80",method="GET",path="/",status_code="200",le="256000"} 1.000000
http_response_body_bytes_bucket{server_port="80",method="GET",path="/",status_code="200",le="512000"} 1.000000
http_response_body_bytes_bucket{server_port="80",method="GET",path="/",status_code="200",le="1048576"} 1.000000
http_response_body_bytes_bucket{server_port="80",method="GET",path="/",status_code="200",le="5242880"} 1.000000
http_response_body_bytes_bucket{server_port="80",method="GET",path="/",status_code="200",le="10485760"} 1.000000
http_response_body_bytes_bucket{server_port="80",method="GET",path="/",status_code="200",le="20971520"} 1.000000
http_response_body_bytes_bucket{server_port="80",method="GET",path="/",status_code="200",le="31457280"} 1.000000
http_response_body_bytes_bucket{server_port="80",method="GET",path="/",status_code="200",le="52428800"} 1.000000
http_response_body_bytes_bucket{server_port="80",method="GET",path="/",status_code="200",le="83886080"} 1.000000
http_response_body_bytes_bucket{server_port="80",method="GET",path="/",status_code="200",le="104857600"} 1.000000
http_response_body_bytes_bucket{server_port="80",method="GET",path="/",status_code="200",le="157286400"} 1.000000
http_response_body_bytes_bucket{server_port="80",method="GET",path="/",status_code="200",le="209715200"} 1.000000
http_response_body_bytes_bucket{server_port="80",method="GET",path="/",status_code="200",le="524288000"} 1.000000
http_response_body_bytes_bucket{server_port="80",method="GET",path="/",status_code="200",le="+Inf"} 1.000000
http_response_body_bytes_sum{server_port="80",method="GET",path="/",status_code="200"} 9593.000000
http_response_body_bytes_count{server_port="80",method="GET",path="/",status_code="200"} 1.000000
http_request_duration_seconds_bucket{server_port="80",method="GET",path="/",status_code="200",le="0.001"} 0.000000
http_request_duration_seconds_bucket{server_port="80",method="GET",path="/",status_code="200",le="0.005"} 0.000000
http_request_duration_seconds_bucket{server_port="80",method="GET",path="/",status_code="200",le="0.01"} 0.000000
http_request_duration_seconds_bucket{server_port="80",method="GET",path="/",status_code="200",le="0.05"} 0.000000
http_request_duration_seconds_bucket{server_port="80",method="GET",path="/",status_code="200",le="0.1"} 0.000000
http_request_duration_seconds_bucket{server_port="80",method="GET",path="/",status_code="200",le="0.25"} 0.000000
http_request_duration_seconds_bucket{server_port="80",method="GET",path="/",status_code="200",le="0.5"} 1.000000
http_request_duration_seconds_bucket{server_port="80",method="GET",path="/",status_code="200",le="1"} 1.000000
http_request_duration_seconds_bucket{server_port="80",method="GET",path="/",status_code="200",le="2"} 1.000000
http_request_duration_seconds_bucket{server_port="80",method="GET",path="/",status_code="200",le="5"} 1.000000
http_request_duration_seconds_bucket{server_port="80",method="GET",path="/",status_code="200",le="10"} 1.000000
http_request_duration_seconds_bucket{server_port="80",method="GET",path="/",status_code="200",le="20"} 1.000000
http_request_duration_seconds_bucket{server_port="80",method="GET",path="/",status_code="200",le="30"} 1.000000
http_request_duration_seconds_bucket{server_port="80",method="GET",path="/",status_code="200",le="60"} 1.000000
http_request_duration_seconds_bucket{server_port="80",method="GET",path="/",status_code="200",le="120"} 1.000000
http_request_duration_seconds_bucket{server_port="80",method="GET",path="/",status_code="200",le="300"} 1.000000
http_request_duration_seconds_bucket{server_port="80",method="GET",path="/",status_code="200",le="600"} 1.000000
http_request_duration_seconds_bucket{server_port="80",method="GET",path="/",status_code="200",le="+Inf"} 1.000000
```

packetd 支持运行时热重载 Protocol Rules，有两种方式触发重载：
- kill -HUP $pid
- curl -XPOST $host:$port/-/reload
