# Observability

> 本文档描述了 packetd 程序输出的数据明细。

## RoundTrips

Roundtrip 是 packetd 定义的各种协议的结构体，以 HTTP 协议为例：

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

所有的协议定义均可在 [packetd/protocol](../protocol) 目录中找到，下面是所有协议 **JSON 序列化**后的样例展示：

* AMQP: [amqp.json](./roundtrips/amqp.json)
* DNS: [dns.json](./roundtrips/dns.json)
* gGRC: [grpc.json](./roundtrips/grpc.json)
* HTTP: [http.json](./roundtrips/http.json)
* HTTP2: [http2.json](./roundtrips/http2.json)
* Kafka: [kafka.json](./roundtrips/kafka.json)
* MongoDB: [mongodb.json](./roundtrips/mongodb.json)
* MySQL: [mysql.json](./roundtrips/mysql.json)
* PostgreSQL: [postgresql.json](./roundtrips/postgresql.json)
* Redis: [redis.json](./roundtrips/redis.json)

## Metrics

Metrics 使用 Prometheus 命名风格，指标名称均以协议名称作为前缀，同时所有指标都有以下**公共维度**，下文不再赘述：

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

Labels: `queue_name` `class` `method`

### DNS

Metrics:
- amqp_requests_total
- amqp_request_duration_seconds
- amqp_request_body_bytes
- amqp_response_body_bytes

Labels: `question`

### gRPC

Metrics:
- grpc_requests_total
- grpc_request_duration_seconds
- grpc_request_body_bytes
- grpc_response_body_bytes

Labels: `service` `status_code`

### HTTP

Metrics:
- http_requests_total
- http_request_duration_seconds
- http_request_body_bytes
- http_response_body_bytes

Labels: `method` `path` `status_code`

### HTTP2

Metrics:
- http2_requests_total
- http2_request_duration_seconds
- http2_request_body_bytes
- http2_response_body_bytes

Labels: `method` `path` `status_code`

### Kafka

Metrics:
- kafka_requests_total
- kafka_request_duration_seconds
- kafka_request_body_bytes
- kafka_response_body_bytes

Labels: `api` `version`

### MongoDB

Metrics:
- mongodb_requests_total
- mongodb_request_duration_seconds
- mongodb_request_body_bytes
- mongodb_response_body_bytes

Labels: `service` `source` `ok`

### MySQL

Metrics:
- mysql_requests_total
- mysql_request_duration_seconds
- mysql_request_body_bytes
- mysql_response_body_bytes
- mysql_response_affected_rows
- mysql_response_resultset_rows

Labels: `command`

### PostgreSQL

Metrics:
- postgresql_requests_total
- postgresql_request_duration_seconds
- postgresql_request_body_bytes
- postgresql_response_body_bytes
- postgresql_response_affected_rows

Labels: `command`
- command

### Redis

Metrics:
- redis_requests_total
- redis_request_duration_seconds
- redis_request_body_bytes
- redis_response_body_bytes

Labels: `command`

## Traces

Traces 遵守 OpenTelemetry 定义规范，其 SpanID/TraceID 为随机值。每种协议均给出了 Spec 参考链接。

### AMQP

> https://opentelemetry.io/docs/specs/semconv/messaging/rabbitmq/

Span Name: <class>.<method>

Span Attributes:
- messaging.name
- messaging.operation.name
- messaging.message.body.size
- messaging.amqp.destination.routing_key
- messaging.amqp.destination.exchange_name
- messaging.amqp.destination.queue_name
- server.address
- server.port
- network.peer.address
- network.peer.port

### DNS

> https://opentelemetry.io/docs/specs/semconv/dns/dns-metrics/

Span Name: <question.name>

Span Attributes:
- dns.request.size
- dns.response.size
- dns.question.type
- server.address
- server.port
- network.peer.address
- network.peer.port

### gRPC

> https://opentelemetry.io/docs/specs/semconv/rpc/grpc/

Span Name: <rpc.service>

Span Attributes:
- rpc.system
- rpc.method
- rpc.service
- rpc.grpc.status_code
- rpc.request.size
- rpc.response.size
- server.address
- server.port
- network.peer.address
- network.peer.port
- rpc.grpc.request.metadata.<key>
- rpc.grpc.response.metadata.<key>

### HTTP/HTTP2

> https://opentelemetry.io/docs/specs/semconv/http/http-spans/

Span Name <http.request.method>

Span Attributes:
- http.request.size
- http.response.size
- http.request.method
- http.response.status_code
- url.full
- url.scheme
- server.address
- server.port
- client.remote.host
- network.peer.address
- network.peer.port
- network.transport
- network.protocol.name
- network.protocol.version
- http.request.header.<key>
- http.response.header.<key>

### Kafka

> https://opentelemetry.io/docs/specs/semconv/messaging/kafka/

Span Name <http.request.method>

Span Attributes:
- messaging.name
- messaging.operation.name
- messaging.operation.version
- messaging.client.id
- messaging.consumer.group.name
- messaging.message.body.size
- error.type
- server.address
- server.port
- network.peer.address
- network.peer.port

### MongoDB

> https://opentelemetry.io/docs/specs/semconv/database/mongodb/

Span Name <db.operation.name>

Span Attributes:
- db.system.name
- db.query.text
- db.operation.name
- db.request.size
- db.response.size
- db.namespace
- db.response.status_code
- db.response.ok
- error.type
- server.address
- server.port
- network.peer.address
- network.peer.port

### MySQL

> https://opentelemetry.io/docs/specs/semconv/database/mysql/

Span Name <db.operation.name>

Span Attributes:
- db.system.name
- db.query.text
- db.operation.name
- db.request.size
- db.response.size
- server.address
- server.port
- network.peer.address
- network.peer.port
- db.response.returned_rows
- error.type
- error.code
- error.sql_state
- db.response.affected_rows
- db.response.last_insert_id
- db.response.warnings
- db.response.info

### PostgreSQL

> https://opentelemetry.io/docs/specs/semconv/database/postgresql/

Span Name <db.operation.name>

Span Attributes:
- db.system.name
- db.operation.name
- db.request.size
- db.response.size
- server.address
- server.port
- network.peer.address
- network.peer.port
- db.query.text
- db.query.text
- db.response.returned_rows
- error.type
- error.code
- error.sql_state
- db.packet.flag

### Redis

>  https://opentelemetry.io/docs/specs/semconv/database/redis/

Span Name <db.operation.name>

Span Attributes:
- db.system.name
- db.operation.name
- db.request.size
- db.response.size
- server.address
- server.port
- network.peer.address
- network.peer.port
- response.data_type
