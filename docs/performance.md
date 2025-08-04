# Performance

> 本文档提供了 packetd 性能压测报告以及调参建议。

benchmark 代码位于 [packetd-benchmark](https://github.com/packetd/packetd-benchmark)。

压测环境

* CPU: 4C
* Memory: 4GB
* Disk: 20GB
* OS: fedora (6.15.4-200.fc42.aarch64)

Header 字段含义：

| Field           | Desc                   |
|-----------------|------------------------|
| PROTO           | 协议类型                   |
| REQUEST         | client 发起的总请求次数        |
| WORKERS         | client 并发数             |
| BODYSIZE        | client 请求 body 大小      |
| ELAPSED         | 单轮压测耗时                 |
| QPS             | 单轮压测请求速率               |
| BPS             | 单轮压测流量速率 bit/s         |
| PROTO (REQUEST) | 捕获的请求总数                | 
| PROTO (PERCENT) | 捕获的请求总数与实际总请求数的比例（达成率） |
| CPU (CORE)      | 压测期间使用的平均 CPU 核心数      | 
| MEMORY (MB)     | 压测结束时 RSS 内存 |    

***压测网络为 localhost，避免网络延迟以及网卡规格等因素影响压测数据。***

packetd 受限于程序代码以及网络设备性能等综合因素影响，**无法保证 100% 请求均被成功捕获并解析**，压测结果会尽量客观体现其瓶颈值。

## Tips

packetd 目前仅提供了 `sniffer.blockNum` 参数作为性能调优的方式。

blockNum 定义了`每块设备`的 buffer 区的大小，但这个值并不是越大越好，当消费端性能不足时扩大 buffer，只会造成大量的内存开销。

可以通过 packetd 提供的 metrics 来判断其是否过载。

```shell
$ curl localhost:9091/metrics | grep packets | grep drop
packetd_sniffer_dropped_packets_total{iface="lo"} 0
```

如果观察到 dropped 指标在不断增加，则证明缓存区在持续丢弃数据，此时可以适当调整 blockNum，最大值为 1024。

```yaml
# from packetd.reference.yaml

# Default: 16
# blockNum 缓冲区 block 数量（仅 Linux 生效）
# 实际代表着监听的`每个设备`生成的 buffer 区域空间为 (1/2 * blockNum) MB 即默认 bufferSize 为 8MB
# 该数值仅能设置为 16 的倍数 非法数值将重置为默认值
sniffer.blockNum: 16
```

## Optimization

packetd 的高性能主要得益于以下细节的优化：

### 1）减少内核态数据拷贝

packetd 使用了 `libpcap` 作为其默认监听引擎，配置 `protocol.rules` 时 sniffer 会将其翻译成 `bpf-filter`，如：

```yaml
protocol.rules:
  - name: http
    protocol: http
    port: 80
  - name: grpc
    protocol: grpc
    port: 8080
    
# 转换为 bpf-filer `(tcp and port 80) or (tcp and port 8080)`
```

这样可以在内核态过滤不需要的数据，减少数据从内核态拷贝到用户态的开销。同时使用 `NoCopy/Lazy` 减少解析开销。

```golang
packetSource.Lazy = true
packetSource.NoCopy = true
```

### 2）减少用户态内存拷贝

packetd 对于用户态的数据使用了**零拷贝**进行处理，重新实现了一套 IO 接口，避免在 Heap 上进行内存分配，降低 GC 压力。

```golang
// Reader ZeroCopy-API
//
// Reader Read 零拷贝方式读取 n 字节数据
type Reader interface {
	Read(n int) ([]byte, error)
}

// Writer ZeroCopy-API
//
// Writer Write 零拷贝方式写入数据 写入不会失败
type Writer interface {
	Write(p []byte)
}

// Closer ZeroCopy-API
//
// Close 将 Reader 置为 io.EOF 状态
type Closer interface {
	Close()
}

// Buffer ZeroCopy-API
//
// 支持 Write/Read/Close 方法 次接口的所有操作均为零拷贝
type Buffer interface {
	Writer
	Reader
	Closer
}
```

benchmark 性能对比：

```docs
goos: darwin
goarch: arm64
pkg: github.com/packetd/packetd/internal/zerocopy
cpu: Apple M3 Pro
BenchmarkZeroCopyBuffer
BenchmarkZeroCopyBuffer-12    	  200308	      6301 ns/op	   65569 B/op	       2 allocs/op
BenchmarkBuffer
BenchmarkBuffer-12            	   34767	     34702 ns/op	  344119 B/op	       6 allocs/op
```

### 3）减少数据切割的开销

部分文本协议使用了 CRLF 作为分隔符，使用标准库 `*bufio.Scanner` / `*bufio.Reader` 在进行切割和读取时可能会产生内存拷贝。

同样地这里也使用了一套**零拷贝**接口，性能对比可参考 _benchmark 单测。

```docs
// Reader Benchmark
goos: darwin
goarch: arm64
pkg: github.com/packetd/packetd/internal/splitio
cpu: Apple M3 Pro
BenchmarkBufioReader
BenchmarkBufioReader-12       	 1006678	      1320 ns/op	77625.18 MB/s	    4144 B/op	       2 allocs/op
BenchmarkZeroCopyReader
BenchmarkZeroCopyReader-12    	 4560764	       280.8 ns/op	364971.35 MB/s	       0 B/op	       0 allocs/op

// Scanner Benchmark
goos: darwin
goarch: arm64
pkg: github.com/packetd/packetd/internal/splitio
cpu: Apple M3 Pro
BenchmarkZeroCopyScanner
BenchmarkZeroCopyScanner-12    	 3747357	       303.5 ns/op	337740.02 MB/s	      48 B/op	       1 allocs/op
BenchmarkBufioScanner
BenchmarkBufioScanner-12       	 1000000	      1446 ns/op	70877.90 MB/s	    4272 B/op	       3 allocs/op
```

### 4）减少非重要字段解析

在各种应用层协议的解析过程中，并非所有字段都需要被 decoder 用到。因此所有的协议都尽可能地实现了一套 `LazyDecoder` 的方案。

以 Kafka Produce API 为例：

```docs
Produce Request (Version: 4) => transactional_id acks timeout_ms [topic_data]
    transactional_id => NULLABLE_STRING
    acks => INT16
    timeout_ms => INT32
    topic_data => name [partition_data]
        name => STRING
        partition_data => index records
        index => INT32
        records => RECORDS
```

在 Produce 请求解析中，重点关注 topicName，所以对于 `transactional_id`、`acks`、`timeout_ms` 选择了 skip 方式。待到 topic.name 解析完成后，对后续 payload 均只做字节计数处理，不做额外解析。

MongoDB 也是同理，重点关注 `database`、`collection`、`commnad` 等核心字段，没有必要对整个 payload 进行 Bson.Unmarshal。

## Protocols

以下是不同协议的压测结果，参数均在表格中说明。压测过程关闭 traces/metrics/roundtrips 输出，仅保留 metrics server 用于观测指标。

下述所有测试均把内存控制在 100MB 以内，理论上调大会 blockNum 会提升其达成率，但本测试的目的是验证在**可控资源开销内** packetd 的性能表现。

***测试取相同参数下运行的最好结果，不代表每次均能有结果中的达成率。***

### AMQP

AMQP 压测选用了 `rabbitmq`，高频写入需要较大的磁盘空间，因此仅进行简单的 consume 压测。

设置 rabbitmq consumer 客户端为同步 ACK 模式。

```shell
# 1) 投递 50000 条 messages，每条 message 为 10KB，约占 500MB 磁盘空间。
> packetd-benchmark/amqp/producer# ./producer -total 50000 -message_size 10240
2025/07/11 11:42:47 produced 50000 messages

# 2）消费 50000 条 messages，并统计器耗时和 values size。
> packetd-benchmark/amqp/consumer# ./consumer -total 50000
2025/07/12 11:18:16 consumed 50000 messages in 1.49208895s, size=488.28MB

# 3）查看流量数据。
$ curl -s localhost:9091/protocol/metrics  | grep amqp_requests_total
amqp_requests_total{class="Basic",method="Deliver"} 50000.000000
amqp_requests_total{class="Connection",method="Close"} 1.000000
amqp_requests_total{class="Channel",method="Close"} 1.000000
amqp_requests_total{class="Channel",method="Open"} 1.000000
amqp_requests_total{class="Basic",method="Qos"} 1.000000
amqp_requests_total{class="Basic",method="Consume"} 1.000000
amqp_requests_total{class="Connection",method="Tune"} 1.000000
amqp_requests_total{class="Connection",method="Open"} 1.000000
amqp_requests_total{class="Connection",method="Start"} 1.000000
amqp_requests_total{class="Queue",method="Declare"} 1.000000
```

### HTTP

HTTP 请求解析的效率较高，`blockNum: 16` 就能获得比较高的 roundtrips 达成率。

| REQUEST | WORKERS | BODYSIZE | ELAPSED | QPS | BPS | PROTO (REQUEST) | PROTO (PERCENT) | CPU (CORE) | MEMORY (MB) |
| ------- | ------- | -------- | ------- | --- | --- |-----------------|-----------------| ---------- | ----------- |
|   10000 |       1 | 10KB     | 0.755s  | 13249.506 | 1035Mib | 10000           | 100.000%        | 0.093      | 38.348      |
|   10000 |       1 | 100KB    | 0.761s  | 13138.960 | 10.02Gib | 10000           | 100.000%        | 0.105      | 40.012      |
|   10000 |       1 | 1MB      | 0.779s  | 12838.487 | 100.3Gib | 10000           | 100.000%        | 0.103      | 37.754      |
|  100000 |      10 | 10KB     | 2.094s  | 47765.909 | 3732Mib | 100000          | 100.000%        | 0.367      | 41.258      |
|  100000 |      10 | 100KB    | 2.156s  | 46373.435 | 35.38Gib | 100000          | 100.000%        | 0.343      | 39.418      |
|  100000 |      10 | 1MB      | 2.086s  | 47949.323 | 374.6Gib | 100000          | 100.000%        | 0.359      | 39.699      |
|  100000 |     100 | 10KB     | 1.545s  | 64715.898 | 5056Mib | 100000          | 100.000%        | 0.478      | 41.906      |
|  100000 |     100 | 100KB    | 1.560s  | 64095.780 | 48.9Gib | 100000          | 100.000%        | 0.461      | 42.387      |
|  100000 |     100 | 1MB      | 1.542s  | 64863.419 | 506.7Gib | 100000          | 100.000%        | 0.467      | 39.383      |
| 1000000 |    1000 | 10KB     | 13.950s | 71684.121 | 5600Mib | 996969          | 99.697%         | 0.636      | 49.406      |

### Redis

Redis 请求在 workers 数较高时达成率会有比较明显的下降，在单请求大数据包的场景下可能会出现丢包情况。

达成率 大于 100% 比例是因为计入了握手请求。

Redis 的 get/set 压测是对单一 key 进行更新或者读取，测试主要关注网络视角，而不是 Redis 的整体处理性能。

| REQUEST | WORKERS | BODYSIZE | ELAPSED | QPS | BPS | COMMAND  | PROTO (REQUEST) | PROTO (PERCENT) | CPU (CORE) | MEMORY (MB) |
| ------- | ------- | -------- | ------- | --- | --- |----------|-----------------|-----------------| ---------- | ----------- |
|  100000 |       1 | 0B       | 9.111s  | 10976.215 | 0b  | ping     | 100001          | 100.001%        | 0.052      | 39.781      |
|  100000 |      10 | 0B       | 1.084s  | 92238.957 | 0b  | ping     | 100010          | 100.010%        | 0.422      | 39.336      |
|  100000 |     100 | 0B       | 0.726s  | 137788.845 | 0b  | ping     | 100100          | 100.100%        | 0.586      | 42.895      |
|   10000 |       1 | 10KB     | 1.121s  | 8922.716 | 697.1Mib | set      | 10001           | 100.010%        | 0.115      | 38.512      |
|  100000 |      10 | 10KB     | 1.966s  | 50864.116 | 3974Mib | set      | 96981           | 96.981%         | 0.527      | 48.359      |
|  100000 |      10 | 100KB    | 4.450s  | 22471.109 | 17.14Gib | set     | 95297           | 95.297%         | 0.413      | 47.820      |
|  100000 |     100 | 10KB     | 1.708s  | 58547.232 | 4574Mib | set     | 90872           | 90.872%         | 0.554      | 48.559      |
|  100000 |       1 | 10KB     | 9.791s  | 10213.632 | 797.9Mib | get     | 100001          | 100.001%        | 0.060      | 47.543      |
|  100000 |      10 | 10KB     | 1.443s  | 69281.762 | 5413Mib | get     | 99711           | 99.711%         | 0.421      | 46.625      |
|  100000 |      10 | 100KB    | 3.505s  | 28532.702 | 21.77Gib | get     | 97504           | 97.504%         | 0.410      | 46.996      |
|  100000 |     100 | 100KB    | 3.443s  | 29044.809 | 22.16Gib | get     | 92262           | 92.262%         | 0.397      | 46.711      |

### gRPC

gRPC 请求在 workers 数达大于 100 时 `blockNum: 16` buffer 区已经不够用，调高至 `blockNum: 64` 会有所缓解。

gRPC 依赖 HTTP2 decoder 实现，所以仅压测 gRPC 即可。

| REQUEST | WORKERS | BODYSIZE | ELAPSED | QPS | BPS | PROTO (REQUEST) | PROTO (PERCENT) | CPU (CORE) | MEMORY (MB) |
| ------- | ------- | -------- | ------- | --- | --- |-----------------|-----------------| ---------- | ----------- |
|   10000 |       1 | 10KB     | 0.992s  | 10078.365 | 787.4Mib | 10000           | 100.000%        | 0.111      | 38.387      |
|   10000 |       1 | 100KB    | 2.102s  | 4757.738 | 3717Mib | 10000           | 100.000%        | 0.157      | 38.516      |
|   10000 |       1 | 1MB      | 10.713s | 933.415 | 7467Mib | 9997            | 99.970%         | 0.133      | 39.289      |
|  100000 |      10 | 10KB     | 2.402s  | 41628.326 | 3252Mib | 99999           | 99.999%         | 0.320      | 39.180      |
|  100000 |      10 | 100KB    | 8.386s  | 11924.699 | 9.098Gib | 99853           | 99.853%         | 0.267      | 47.992      |
|   10000 |      10 | 1MB      | 6.085s  | 1643.455 | 12.84Gib | 9838            | 98.380%         | 0.214      | 63.422      |
|  100000 |     100 | 10KB     | 1.771s  | 56476.888 | 4412Mib | 99698           | 99.698%         | 0.372      | 46.543      |
|  100000 |      50 | 50KB     | 4.292s  | 23301.353 | 8.889Gib | 99644           | 99.644%         | 0.315      | 47.477      |
|  100000 |      20 | 100KB    | 7.898s  | 12661.172 | 9.66Gib | 99753           | 99.753%         | 0.280      | 64.070      |

### Kafka

Kafka 作为 MQ，压测需要不断投递数据，压测环境并无充足的磁盘条件，所以仅进行简单的 producer/consumer 压测。

Kafka 的 ack 机制往往是异步的，所以单次 Request 请求后，client 不一定会及时确认消费，因此也不好统计 `达成率`，这里仅评估其流量数据。

```shell
# 1) 投递 50000 条 messages，每条 message 为 10KB，约占 500MB 磁盘空间。
> packetd-benchmark/kafka/producer# ./producer -total 50000 -message_size 10240
2025/07/11 11:42:47 produced 50000 messages

# 2）消费 50000 条 messages，并统计器耗时和 values size。
> packetd-benchmark/kafka/consumer# ./consumer -total 50000
2025/07/11 11:50:01 consumed 50000 messages in 883.801631ms, size=489MB

# 3）查看流量数据。
$ curl -s localhost:9091/protocol/metrics  | grep bytes | grep sum
kafka_request_body_bytes_sum{api="OffsetFetch"} 55.000000
kafka_request_body_bytes_sum{api="OffsetCommit"} 118.000000
kafka_request_body_bytes_sum{api="LeaveGroup"} 77.000000
kafka_request_body_bytes_sum{api="FindCoordinator"} 66.000000
kafka_request_body_bytes_sum{api="Metadata"} 72.000000
kafka_request_body_bytes_sum{api="JoinGroup"} 88.000000
kafka_request_body_bytes_sum{api="Heartbeat"} 81.000000
kafka_request_body_bytes_sum{api="ListOffsets"} 120.000000
kafka_request_body_bytes_sum{api="Fetch"} 48096.000000
kafka_request_body_bytes_sum{api="SyncGroup"} 163.000000
kafka_response_body_bytes_sum{api="OffsetCommit"} 37.000000
kafka_response_body_bytes_sum{api="LeaveGroup"} 14.000000
kafka_response_body_bytes_sum{api="FindCoordinator"} 78.000000
kafka_response_body_bytes_sum{api="Metadata"} 242.000000
kafka_response_body_bytes_sum{api="OffsetFetch"} 53.000000
kafka_response_body_bytes_sum{api="ListOffsets"} 114.000000
kafka_response_body_bytes_sum{api="Fetch"} 524305496.000000 -> ~500MB
kafka_response_body_bytes_sum{api="JoinGroup"} 189.000000
kafka_response_body_bytes_sum{api="SyncGroup"} 47.000000
kafka_response_body_bytes_sum{api="Heartbeat"} 14.000000
```

### MySQL

MySQL 解析高度依赖网络包的连续性，丢包情况会对解析造成比较大影响，其上限要低于像 HTTP/Redis 类型的文本协议。

达成率大于 100% 比例是因为计入了握手请求。压测数据样例如下：

```sql
MySQL root@localhost:benchmark> select * from stress_test limit 1;
+----+--------------------------------------+---------+---------+---------------------+--------+
| id | uuid                                 | user_id | amount  | create_time         | status |
+----+--------------------------------------+---------+---------+---------------------+--------+
| 1  | addf2dc0-563e-46d7-b813-1f11f6b2ca3e | 83923   | 1884.54 | 2025-07-06 08:23:24 | 3      |
+----+--------------------------------------+---------+---------+---------------------+--------+
```

压测 MySQL 不统计流量明细，主要关注请求 QPS 以及达成率。

| REQUEST | WORKERS | ELAPSED | QPS | SQL | PROTO (REQUEST) | PROTO (PERCENT) | CPU (CORE) | MEMORY (MB) |
| ------- | ------- | ------- | --- |-----|-----------------|-----------------| ---------- |-------------|
|   10000 |       1 | 2.316s  | 4317.668 | select * from stress_test limit 100 | 10001           | 100.010%        | 0.026      | 38.562      |
|   10000 |       1 | 5.291s  | 1889.883 | select * from stress_test limit 1000 | 10001           | 100.010%        | 0.066      | 46.715      |
|    1000 |       1 | 3.030s  | 330.042 | select * from stress_test limit 10000 | 1001            | 100.100%        | 0.096      | 46.895      |
|   10000 |      10 | 0.544s  | 18368.043 | select * from stress_test limit 100 | 10010           | 100.100%        | 0.128      | 38.180      |
|   10000 |      10 | 1.949s  | 5130.091 | select * from stress_test limit 1000 | 10010           | 100.100%        | 0.154      | 63.730      |
|    1000 |      10 | 1.595s  | 627.029 | select * from stress_test limit 10000 | 994             | 99.400%         | 0.125      | 78.801      |
|    2000 |      20 | 3.182s  | 628.515 | select * from stress_test limit 10000 | 1977            | 98.850%         | 0.126      | 78.863      |
|    2000 |      30 | 3.066s  | 652.303 | select * from stress_test limit 10000 | 1944            | 97.200%         | 0.124      | 79.227      |

### MongoDB

MongoDB 使用 [bson](https://www.mongodb.com/zh-cn/docs/manual/reference/bson-types/) 作为其数据格式，可以进行非连续解析，解析效率高于 JSON。

压测数据样例如下：

```sql
benchmark> db.getCollection('stress_test').find().limit(1)
[
  {
    _id: ObjectId('686e957a61e718ff1d5c1dc8'),
    user_id: 1,
    username: 'user_1',
    email: 'user_1@example.com',
    created_at: ISODate('2025-07-09T16:14:50.943Z'),
    active: false,
    score: 894,
    balance: '8947.83',
    tags: [ 'tag2', 'tag2' ],
    last_login: null,
    metadata: { device: 'desktop', version: '1.1' }
  }
]
```

MongoDB 执行 `find` 命令会拆分成多次（测试为 2 次）请求，client 在第一次 `find` 请求后会紧接着再发一次 `getMore` 请求获取剩余数据，因此 200% 则代表全部请求都已捕获。大于 200% 是计入了握手请求。

MongoDB 在解析 `database` 和 `collection` 字段比较高效，不需要解析完整请求内容。而 `code` / `ok` 两个字段则需要遍历所有的 Body，效率较低，压测时设置 `enableResponseCode: false`。

| REQUEST | WORKERS | ELAPSED | QPS | LIMIT | PROTO (REQUEST) | PROTO (PERCENT) | CPU (CORE) | MEMORY (MB) |
| ------- | ------- | ------- | --- | ----- |-----------------|-----------------| ---------- | ----------- |
|    1000 |       1 | 0.592s  | 1688.761 |  1000 | 2001            | 200.100%        | 0.084      | 62.312      |
|    1000 |       1 | 2.907s  | 343.983 | 10000 | 2001            | 200.100%        | 0.052      | 62.754      |
|    1000 |      10 | 1.341s  | 745.614 | 10000 | 2004            | 200.400%        | 0.149      | 70.461      |
|    5000 |      20 | 6.075s  | 823.111 | 10000 | 9798            | 195.960%        | 0.161      | 79.094      |
|    5000 |      50 | 5.934s  | 842.535 | 10000 | 9616            | 192.320%        | 0.175      | 79.238      |
|    5000 |     100 | 5.972s  | 837.294 | 10000 | 9723            | 194.460%        | 0.191      | 80.898      |
|   50000 |      10 | 8.595s  | 5817.087 |  1000 | 99932           | 199.864%        | 0.168      | 78.738      |
|   50000 |      20 | 8.100s  | 6173.102 |  1000 | 99724           | 199.448%        | 0.178      | 79.852      |

### PostgreSQL

PostgreSQL 解析同样高度依赖网络包的连续性，丢包情况会对解析造成比较大影响，其解析上限要低于 MySQL。

压测样例数据如下：

```sql
postgres@localhost:benchmark> select * from stress_test limit 1;
+--------+--------------------------------------+---------+---------+-------------------------------+--------+
| id     | uuid                                 | user_id | amount  | create_time                   | status |
|--------+--------------------------------------+---------+---------+-------------------------------+--------|
| 342993 | 05b41151-9cd2-40a6-bbc5-1e278b543fd9 | 93813   | 6002.12 | 2025-03-10 14:37:31.753746+00 | 1      |
+--------+--------------------------------------+---------+---------+-------------------------------+--------+
```

相比与 MySQL，PostgreSQL 更倾向于`小步快跑`，使用更小的数据包，但更高的发送频率。

| REQUEST | WORKERS | ELAPSED | QPS | SQL | PROTO (REQUEST) | PROTO (PERCENT) | CPU (CORE) | MEMORY (MB) |
| ------- | ------- | ------- | --- |-----|-----------------|-----------------| ---------- |-------------|
|   10000 |       1 | 1.324s  | 7552.303 | select * from stress_test limit 100; | 9999            | 99.990%         | 0.053      | 62.902      |
|   10000 |       1 | 3.185s  | 3139.549 | select * from stress_test limit 1000; | 9999            | 99.990%         | 0.119      | 63.070      |
|    1000 |       1 | 2.224s  | 449.689 | select * from stress_test limit 10000; | 999             | 99.900%         | 0.144      | 63.137      |
|   10000 |      10 | 0.287s  | 34853.278 | select * from stress_test limit 100; | 9990            | 99.900%         | 0.243      | 62.805      |
|   10000 |       5 | 1.289s  | 7760.628 | select * from stress_test limit 1000; | 9995            | 99.950%         | 0.225      | 94.527      |
|    2000 |       5 | 2.000s  | 1000.244 | select * from stress_test limit 10000; | 1995            | 99.750%         | 0.235      | 95.547      |
|   10000 |       5 | 5.299s  | 1886.992 | select * from stress_test limit 5000; | 9449            | 94.490%         | 0.223      | 94.430      |
