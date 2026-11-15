# Venom - Executor RabbitMQ

Three types of execution are supported:
- **publisher**: publish a message to a queue or to an exchange.
- **subscriber**: bind to a queue or an exchange (using routing key) and wait for message(s) to be consumed.
- **client**: publish a message to a queue or to an exchange and wait for the response message to be received on the [reply-to](https://www.rabbitmq.com/docs/direct-reply-to) queue.

Steps to use publish / subscribe on a RabbitMQ:

## Input
In your yaml file, you can use:

```yaml

  # RabbitMQ connection
  - addrs optional        (default amqp:/localhost:5672)
  - user optional         (default guest)
  - password optional     (default guest)

  - clientType mandatory (publisher, subscriber or client)

  # RabbitMQ Q configuration
  - qName mandatory

  # Exchange configuration
  - routingKey optional   (default qName)
  - exchangeType optional  (default "fanout")
  - exchange optional     (default "")

  # For subscriber and client only
  - messageLimit optional (default 1)

  # For publisher and client only
  - messages
    - durable optional      (true or false) (default false)
    - contentType optional  
    - contentEncoding optional
    - persistent optional (default true)
    - headers optional
      - name: value

```

## Examples:

### Publisher (workQ)
```yaml
name: TestSuite RabbitMQ
vars:
  addrs: 'amqp://localhost:5672'
  user: 
  password: 
testcases:
  - name: RabbitMQ publish (work Q)
    steps:
      - type: rabbitmq
        addrs: "{{.addrs}}"
        user: "{{.user}}"
        password: "{{.password}}"
        clientType: publisher
        qName: TEST
        messages: 
          - value: '{"a": "b"}'
            contentType: application/json
            contentEncoding: utf8
            persistent: false
            headers: 
              myCustomHeader: value
              myCustomHeader2: value2
```

### Subscriber (workQ)
```yaml
name: TestSuite RabbitMQ
vars:
  addrs: 'amqp://localhost:5672'
  user: 
  password: 
  - name: RabbitMQ subscribe testcase
    steps:
      - type: rabbitmq
        addrs: "{{.addrs}}"
        user: "{{.user}}"
        password: "{{.password}}"
        clientType: subscriber
        qName: "{{.qName}}"
        durable: true
        messageLimit: 1
        assertions: 
          - result.bodyjson.bodyjson0.a ShouldEqual b   
          - result.headers.headers0.mycustomheader ShouldEqual value
          - result.headers.headers0.mycustomheader2 ShouldEqual value2
          - result.messages.messages0.contentencoding ShouldEqual utf8
          - result.messages.messages0.contenttype ShouldEqual application/json
```

### Publisher (pubsub)
```yaml
name: TestSuite RabbitMQ
vars:
  addrs: 'amqp://localhost:5672'
  user: 
  password: 
testcases:
  - name: RabbitMQ publish (work Q)
    steps:
      - type: rabbitmq
        addrs: "{{.addrs}}"
        user: "{{.user}}"
        password: "{{.password}}"
        clientType: publisher
        exchange: exchange_test
        routingKey: pubsub_test
        messages: 
          - value: '{"a": "b"}'
            contentType: application/json
            contentEncoding: utf8
            persistent: false
            headers: 
              myCustomHeader: value
              myCustomHeader2: value2
```

### Subscriber (pubsub)
```yaml
name: TestSuite RabbitMQ
vars:
  addrs: 'amqp://localhost:5672'
  user: 
  password: 
  - name: RabbitMQ subscribe testcase
    steps:
      - type: rabbitmq
        addrs: "{{.addrs}}"
        user: "{{.user}}"
        password: "{{.password}}"
        clientType: subscriber
        exchange: exchange_test
        routingKey: pubsub_test
        messageLimit: 1
        assertions: 
          - result.bodyjson.bodyjson0.a ShouldEqual b   
          - result.headers.headers0.mycustomheader ShouldEqual value
          - result.headers.headers0.mycustomheader2 ShouldEqual value2
          - result.messages.messages0.contentencoding ShouldEqual utf8
          - result.messages.messages0.contenttype ShouldEqual application/json
```

### Client (pubsub RPC)
```yaml
name: TestSuite RabbitMQ
vars:
  addrs: 'amqp://localhost:5672'
  user: 
  password: 
testcases:
  - name: RabbitMQ request/reply
    steps:
      - type: rabbitmq
        addrs: "{{.addrs}}"
        user: "{{.user}}"
        password: "{{.password}}"
        clientType: client
        exchange: exchange_test
        routingKey: pubsub_test
        messages: 
          - value: '{"a": "b"}'
            contentType: application/json
            contentEncoding: utf8
            persistent: false
            headers: 
              myCustomHeader: value
              myCustomHeader2: value2
        messageLimit: 1
        assertions: 
          - result.bodyjson.bodyjson0 ShouldContainKey Status
          - result.bodyjson.bodyjson0.Status ShouldEqual Succeeded
```

## Protobuf bodies (subscriber)

A subscriber step decodes protobuf message bodies to JSON when it names the descriptors, as the grpc executor does:

```yaml
- type: rabbitmq
  addrs: amqp://rabbitmq:5672
  user: guest
  password: guest
  clientType: subscriber
  qName: qa-capture
  queue_passive: true          # read a queue the stand declared and bound; nothing is declared or bound
  messageLimit: 50             # an empty queue ends the read early when bodies are decoded
  proto_files: [cgw/crawler/v1/bus.proto]
  import_paths: [/proto]
  message_types:               # routing key → message type; message_type is the type of any other key
    order.outcome: cgw.crawler.v1.CrawlerOrderOutcome
  message_type: cgw.crawler.v1.FactEnvelope
  assertions:
    - result.count ShouldBeGreaterThan 0
    - result.routing_keys ShouldContain order.outcome
    - result.bodyjson.bodyjson0.kind ShouldNotBeEmpty
```

Result per message, in read order: `body` (JSON text of a decoded body), `bodyjson`, `routing_keys`, `exchanges`,
`types` (the message type used, empty for a key without one), `headers`; `count` is the number of messages read.

A step-own capture queue: bind it before the action with `messageLimit: -1` (declare and bind, read nothing), read it after
the action with `queue_delete: true` (the queue is deleted after the read):

```yaml
- type: rabbitmq
  clientType: subscriber
  qName: qa-B8-93-1201
  exchange: cgw-crawler.facts
  exchangeType: fanout
  messageLimit: -1
# … the action …
- type: rabbitmq
  clientType: subscriber
  qName: qa-B8-93-1201
  queue_passive: true
  queue_delete: true
  messageLimit: 200
```
