# Benchmark

## Overview

benchmark 代码位于 [packetd-benchmark](https://github.com/packetd/packetd)。压测环境为 Linux 6.11.9-100.fc39.aarch64 (4C4G)。

Header 字段含义:
- Proto: 协议类型
- Request: client 发起的总请求次数
- Workers: client 并发数
- BodySize: client 请求 body 大小（部分情况可能不携带）
- Interval: client 每次请求间隔周期
- Elapsed: 单轮压测耗时
- Qps: 单轮压测速率
- Bps: 压测流量速率 bit/s
- Proto/Metrics: packetd 进程记录的请求总数
- Proto/Percent: packetd 进程记录的请求总数与实际总请求数的比例（达成率）

压测均为 localhost 网络环境，避免因网络延迟已经网卡规格导致理论性能相差太大。packetd 受限于代码程序以及网络设备性能等综合因素的影响，无法保证 100% 请求均被成功捕获并解析，这里压测结果会尽量客观体现其瓶颈数值。

## Protocols

以下为各种协议在测试机上的压测结果，参数均以在表格中说明。

## HTTP

| PROTO | REQUEST | WORKERS | BODYSIZE | INTERVAL | ELAPSED | QPS | BPS | PROTO/METRICS | PROTO/PERCENT |
| ----- | ------- | ------- | -------- | -------- | ------- | --- | --- | ------------- | ------------- |
| HTTP  |  100000 |      10 | 10KB     | 0s       | 4.007305975s | 24954.42 | 1950Mib |        100000 | 100.00%       |
| HTTP  |  100000 |      10 | 10KB     | 0s       | 3.468937171s | 28827.27 | 2252Mib |        100000 | 100.00%       |
| HTTP  |  100000 |      10 | 100KB    | 0s       | 3.349872189s | 29851.89 | 22.78Gib |        100000 | 100.00%       |
| HTTP  |  100000 |      20 | 1000KB   | 0s       | 4.369766142s | 22884.52 | 174.6Gib |        100000 | 100.00%       |
| HTTP  |  500000 |      50 | 10KB     | 0s       | 13.165300587s | 37978.62 | 2967Mib |        500000 | 100.00%       |
| HTTP  |  500000 |      50 | 1KB      | 0s       | 12.485740201s | 40045.68 | 312.9Mib |        500000 | 100.00%       |
| HTTP  |  100000 |      30 | 1KB      | 0s       | 4.221497549s | 23688.28 | 185.1Mib |        100000 | 100.00%       |
| HTTP  |  100000 |      80 | 1KB      | 0s       | 3.558770104s | 28099.60 | 219.5Mib |        100000 | 100.00%       |
| HTTP  |  100000 |     200 | 10KB     | 0s       | 4.819255075s | 20750.09 | 1621Mib |         96904 | 96.90%        |
