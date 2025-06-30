# API 文档

### 监控指标

* GET /metrics: prometheus 自监控指标上报
* GET /protocol/metrics: 协议监控指标上报

### 管理路由

* POST /-/logger: 运行时动态调整 logger level

    ```shell
    $ curl -XPOST -d 'level=debug' http://locahost:9091/-/logger 
    ```

* POST /-/reload: 运行时重载 packetd
* POST /-/protocol/reset: 清空协议指标

### 性能分析

* GET /debug/pprof/cmdline: 返回 cmdline 执行命令
* GET /debug/pprof/profile: profile 采集
* GET /debug/pprof/symbol: symbol 采集
* GET /debug/pprof/trace: trace 采集
* GET /debug/pprof/{other}: 其他 profile 项采集
