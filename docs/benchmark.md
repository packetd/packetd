# Benchmark

benchmark 代码位于 [packetd-benchmark](https://github.com/packetd/packetd)。压测环境为 Linux 6.11.9-100.fc39.aarch64 (4C4G)。

Header 字段含义:
- Proto: 协议类型
- Request: client 发起的总请求次数
- Workers: client 并发数
- BodySize: client 请求 body 大小
- Interval: client 每次请求间隔周期
- Elapsed: 单轮压测耗时
- Qps: 单轮压测请求速率
- Bps: 单轮压测流量速率 bit/s
- Proto/Metrics: packetd 进程记录的请求总数
- Proto/Percent: packetd 进程记录的请求总数与实际总请求数的比例（达成率）

压测网络为 localhost，避免因网络延迟已经网卡规格导致理论性能相去甚远。packetd 受限于代码程序以及网络设备性能等综合因素的影响，无法保证 100% 请求均被成功捕获并解析，这里压测结果会尽量客观体现其瓶颈值。

## Performance

// TODO: 待补充

## Protocols

以下为各种协议在测试机上的压测结果，参数均在表格中说明。压测过程关闭 roundtrips 以及 traces 输出，只保留 metrics 用于做指标打点统计。

### HTTP

| PROTO | REQUEST | WORKERS | BODYSIZE  | ELAPSED | QPS | BPS | PROTO/METRICS | PROTO/PERCENT |
| ----- | ------- | ------- | --------  |---------| --- | --- | ------------- | ------------- |
| HTTP  |  100000 |      10 | 10KB     | 3.288s  | 30410.734 | 2376Mib |        100000 | 100.000%      |
| HTTP  |  100000 |      10 | 100KB    | 3.310s  | 30214.392 | 23.05Gib |        100000 | 100.000%      |
| HTTP  |  100000 |      10 | 1MB      | 3.339s  | 29951.802 | 234Gib |        100000 | 100.000%      |
| HTTP  |  500000 |      50 | 1KB      | 10.152s | 49250.733 | 384.8Mib |        500000 | 100.000%      |
| HTTP  |  500000 |      50 | 10KB     | 10.051s | 49745.785 | 3886Mib |        500000 | 100.000%      |
| HTTP  |  500000 |      50 | 1MB      | 10.680s | 46815.213 | 365.7Gib |        500000 | 100.000%      |
| HTTP  |  500000 |     500 | 10KB     | 7.728s  | 64700.562 | 5055Mib |        500000 | 100.000%      |
| HTTP  |  500000 |    5000 | 10KB     | 18.350s | 27248.214 | 2129Mib |        441169 | 88.234%       |


### Redis

| PROTO | REQUEST | WORKERS | COMMAND | BODYSIZE | ELAPSED  | QPS       | BPS     | PROTO/METRICS | PROTO/PERCENT |
| ----- | ------- | ------- |---------| -------- |----------| --- | ------------- | ------------- | --- |
| Redis |  100000 |      10 | ping    | 0B       | 0.562s  | 177993.854 | 0b  |        100010 | 100.010%      |
| Redis |  100000 |      50 | ping    | 0B       | 0.440s  | 227433.191 | 0b  |         81995 | 81.995%       |
| Redis |   10000 |       1 | get     | 10KB     | 0.887s  | 11274.481 | 880.8Mib |          9994 | 99.940%       |
| Redis |   50000 |       2 | get     | 10KB     | 2.618s  | 19101.756 | 1492Mib |         49998 | 99.996%       |
| Redis |   50000 |       5 | get     | 10KB     | 2.032s  | 24606.572 | 1922Mib |         49769 | 99.538%       |
| Redis |  100000 |      10 | get     | 10KB     | 3.643s  | 27448.310 | 2144Mib |         98852 | 98.852%       |
| Redis |  100000 |      10 | set     | 10KB     | 2.323s  | 43046.831 | 3363Mib |        100010 | 100.010%      |
| Redis |  100000 |      10 | set     | 100KB    | 5.309s  | 18834.716 | 14.37Gib |         99644 | 99.644%       |
| Redis |  100000 |      50 | set     | 100KB    | 5.812s  | 17204.495 | 13.13Gib |         96499 | 96.499%       |

### Grpc

| PROTO | REQUEST | WORKERS | BODYSIZE  | ELAPSED | QPS | BPS | PROTO/METRICS | PROTO/PERCENT |
| ----- | ------- | ------- | --------  |---------| --- | --- | ------------- | ------------- |
| Grpc  |   10000 |      10 | 10KB     | 1.407s  | 7105.671 | 555.1Mib |         10000 | 100.000%      |
| Grpc  |  100000 |      10 | 20KB     | 5.712s  | 17506.366 | 2735Mib |        100000 | 100.000%      |
| Grpc  |  100000 |      20 | 20KB     | 5.253s  | 19035.493 | 2974Mib |         99987 | 99.987%       |
| Grpc  |  100000 |      20 | 100KB    | 12.739s | 7849.908 | 6133Mib |         99353 | 99.353%       |


### Kafka

### MySQL

### MongoDB

### PostgreSQL
