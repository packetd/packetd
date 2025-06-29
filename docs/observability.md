# Observation

## RoundTrips

Roundtrip 是项目定义的各种协议的结构体，以 HTTP 协议为例：

```go
// Request HTTP 请求
//
// 裁剪了 http.Request 部分字段 大部分字段语义保持一致
type Request struct {
	Host       string
	Port       uint16
	Method     string
	Header     http.Header
	Proto      string
	Path       string
	URL        string
	Scheme     string
	RemoteHost string
	Close      bool
	Size       int
	Chunked    bool
	Time       time.Time
}

// Response HTTP 响应
//
// 裁剪了 http.Response 部分字段 大部分字段语义保持一致
type Response struct {
	Host       string
	Port       uint16
	Header     http.Header
	Status     string
	StatusCode int
	Proto      string
	Close      bool
	Size       int
	Chunked    bool
	Time       time.Time
}
```

所有的协议的定义均可在 [./protocol](../protocol) 目录中找到，下面是所有协议 **JSON 序列化**后的样例展示：

* AMQP: [amqp.json](./roundtrips/amqp.json)
* DNS: [dns.json](./roundtrips/dns.json)
* GRPC: [grpc.json](./roundtrips/grpc.json)
* HTTP: [http.json](./roundtrips/http.json)
* HTTP2: [http2.json](./roundtrips/http2.json)
* Kafka: [kafka.json](./roundtrips/kafka.json)
* MongoDB: [mongodb.json](./roundtrips/mongodb.json)
* MySQL: [mysql.json](./roundtrips/mysql.json)
* PostgreSQL: [postgresql.json](./roundtrips/postgresql.json)
* Redis: [redis.json](./roundtrips/redis.json)

## Metrics

指标使用 Prometheus 命名风格，指标名称均以协议名称作为前缀，同时所有指标都有以下公共维度，下文不再赘述：

- client_address
- client_port
- server_address
- server_port

### AMQP

Metrics:
- amqp_requests_total
- amqp_request_duration_seconds
- amqp_request_body_bytes
- amqp_response_body_bytes

Labels:
- queue_name
- class
- method

### DNS

Metrics:
- amqp_requests_total
- amqp_request_duration_seconds
- amqp_request_body_bytes
- amqp_response_body_bytes

Labels:
- question

### GRPC

Metrics:
- grpc_requests_total
- grpc_request_duration_seconds
- grpc_request_body_bytes
- grpc_response_body_bytes

Labels:
- service
- status_code

### HTTP

Metrics:
- http_requests_total
- http_request_duration_seconds
- http_request_body_bytes
- http_response_body_bytes

Labels:
- method
- path
- status_code

### HTTP2

Metrics:
- http2_requests_total
- http2_request_duration_seconds
- http2_request_body_bytes
- http2_response_body_bytes

Labels:
- method
- path
- status_code

### Kafka

Metrics:
- kafka_requests_total
- kafka_request_duration_seconds
- kafka_request_body_bytes
- kafka_response_body_bytes

Labels:
- api
- version

### MongoDB

Metrics:
- mongodb_requests_total
- mongodb_request_duration_seconds
- mongodb_request_body_bytes
- mongodb_response_body_bytes

Labels:
- service
- source
- ok

### MySQL

Metrics:
- mysql_requests_total
- mysql_request_duration_seconds
- mysql_request_body_bytes
- mysql_response_body_bytes
- mysql_response_affected_rows
- mysql_response_resultset_rows

Labels:
- command
- packet_type

### PostgreSQL

Metrics:
- postgresql_requests_total
- postgresql_request_duration_seconds
- postgresql_request_body_bytes
- postgresql_response_body_bytes
- postgresql_response_affected_rows

Labels:
- command
- packet_type

### Redis

Metrics:
- redis_requests_total
- redis_request_duration_seconds
- redis_request_body_bytes
- redis_response_body_bytes

Labels:
- command

## Traces
