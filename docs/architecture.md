# architecture

The architecture is fairly simple and consists  of a load balancer, two or more
`influxdb-relay`   processes   and  two   or   more   InfluxDB  or   Prometheus
processes. The  load balancer should point  UDP traffic and HTTP  POST requests
with the path `/write`  to the two relays while pointing  GET requests with the
path `/query` to the two InfluxDB servers.

The setup should look like this:

```
        ┌─────────────────┐
        │writes & queries │
        └─────────────────┘
                 │
                 ▼
         ┌───────────────┐
         │               │
┌────────│ Load Balancer │─────────┐
│        │               │         │
│        └──────┬─┬──────┘         │
│               │ │                │
│               │ │                │
│        ┌──────┘ └────────┐       │
│        │ ┌─────────────┐ │       │┌──────┐
│        │ │/write or UDP│ │       ││/query│
│        ▼ └─────────────┘ ▼       │└──────┘
│  ┌──────────┐      ┌──────────┐  │
│  │ InfluxDB │      │ InfluxDB │  │
│  │ Relay    │      │ Relay    │  │
│  └──┬────┬──┘      └────┬──┬──┘  │
│     │    |              |  │     │
│     |  ┌─┼──────────────┘  |     │
│     │  │ └──────────────┐  │     │
│     ▼  ▼                ▼  ▼     │
│  ┌──────────┐      ┌──────────┐  │
│  │          │      │          │  │
└─▶│ InfluxDB │      │ InfluxDB │◀─┘
   │          │      │          │
   └──────────┘      └──────────┘
 ```

The  relay will  listen for  HTTP or  UDP  writes and  write the  data to  each
InfluxDB server  via the  HTTP write  or UDP endpoint,  as appropriate.  If the
write is sent via HTTP, the relay will return a success response as soon as one
of the InfluxDB servers returns a success. If any InfluxDB server returns a 4xx
response,  that will  be returned  to the  client immediately.  If all  servers
return a 5xx, a 5xx will be returned to the client. If some but not all servers
return a 5xx that  will not be returned to the client.  You should monitor each
instance's logs for 5xx errors.

With this setup a  failure of one Relay or one InfluxDB  can be sustained while
still taking  writes and serving  queries. However, the recovery  process might
require operator intervention.
