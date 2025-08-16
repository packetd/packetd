# API 文档

> 本文档描述了 packetd server API。

## 监控路由

* GET /metrics: prometheus 自监控指标上报
* GET /protocol/metrics: 协议监控指标上报
* GET /watch?max_message=5&timeout=10s: 实时观测 roundtrips
   - max_message: 最大消息数
   - timeout: 超时时间

### 管理路由

* POST /-/logger: 运行时动态调整 logger level

    ```shell
    $ curl -XPOST -d 'level=debug' http://locahost:9091/-/logger 
    ```

* POST /-/reload: 运行时重载 packetd

### 性能分析

* GET /debug/pprof/cmdline: 返回 cmdline 执行命令
* GET /debug/pprof/profile: profile 采集
* GET /debug/pprof/symbol: symbol 采集
* GET /debug/pprof/trace: trace 采集
* GET /debug/pprof/{other}: 其他 profile 项采集
