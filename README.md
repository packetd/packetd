# packetd

packetd 是一个基于 `libpcap` 的**应用层协议**网络数据无侵观测工具。

packetd 支持从数据流中解析出应用协议，使用请求的来回，即 ***RoundTrip* 作为其核心概念，进而衍生出 **Traces/Metrics** 数据。但由于缺乏上下文关联，Traces 仅能代表当次网络情况的情况，无法关联应用层的 Span，更像是一种 Event/Log 类型的数据，只不过以 Traces 的形式组织起来。

packetd 提供了更加现代化的可观测手段，如：
- 支持 Prometheus RemoteWrite 协议上报 Metrics 数据。
- 支持 OpenTelemetry 协议上报 Traces 数据。
- 支持 VictoriaMetrics VmRange Histogram，无需提前定义 bucket。

## Installation

```shell
$ go install github.com/packetd/packetd@latest
```

## Quickstart

packetd 提供了两种运行模式，agent 和 log，前者使用 agent 模式持续监听网络包并工作，后者作为一种 cli 工具临时 debug 网络请求。

```shell
# packetd is a eBPF-powered network traffic capture and analysis tool

Usage:
  packetd [command]

Available Commands:
  agent       Run packetd as a network monitoring agent
  config      Prints the reference configuration
  help        Help about any command
  log         Capture and record network traffic in roundtrips mode

Flags:
  -h, --help   help for packetd

Use "packetd [command] --help" for more information about a command.
```

packetd 项目启动需要指定配置文件，log 模式本质上是内置以一份配置模版，详见 [#logConfig](cmd/log.go)。可通过 `packetd config` 子命令查看所有配置项内容。

### 1) agent-mode

agent 模式以守护进程模式运行。

```shell
$ packetd agent --config packetd.yaml
2025-06-22 15:58:25.278 INFO    logger/logger.go:136    sniffer add device (any), address=[]
...
```

### 2) log-mode

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
      "Accept": ["*/*"],
      "User-Agent": ["curl/7.61.1"]
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
      "Accept-Ranges": ["bytes"],
      "Cache-Control": ["max-age=86400"],
      "Connection": ["Keep-Alive"],
      "Content-Length": ["81"],
      "Content-Type": ["text/html"],
      "Date": ["Sun, 22 Jun 2025 08:01:41 GMT"],
      "Etag": ["\"51-47cf7e6ee8400\""],
      "Expires": ["Mon, 23 Jun 2025 08:01:41 GMT"],
      "Last-Modified": ["Tue, 12 Jan 2010 13:48:00 GMT"],
      "Server": ["Apache"]
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

packetd 除了支持从网卡直接捕获网络数据，还支持加载 pcap 文件，如：

```shell
$ packetd log --pcap.file /tmp/app.pcap
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

压测程序位于 [packetd-benchmark](https://github.com/packetd/packetd-benchmark)，压测环境...

### HTTP

| Proto | Requests | Workers | BodySize | Interval | QPS | bps |
| ----- | -------- | ------- |----------|----------| --- | --- |
| HTTP | 100000 | 10 | 0s     | 10KB     | 33504.349798  | 2618Mib |
| HTTP | 100000 | 10 | 0s     | 100KB    | 33504.349798 | 28.68Gib |

### Redis

| Proto | Requests | Workers | BodySize | Cmd   | Interval          | QPS          | bps |
|-------| -------- | ------- |----------|-------|-------------------|--------------| --- |
| Redis | 100000 | 10 | 0s     | 0KB   | Ping | 94553.401549 | 738.7Mib |


// TODO: 待补充

## License

Apache [©packetd](https://github.com/packetd/packetd/blob/master/LICENSE)
