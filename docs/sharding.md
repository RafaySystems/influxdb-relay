# sharding

It's  possible to  add another  layer on  top of  this kind  of setup  to shard
data. Depending  on your  needs you could  shard on the  measurement name  or a
specific tag like `customer_id`. The sharding  layer would have to service both
queries and writes.

As  this relay  does not  handle queries,  it will  not implement  any sharding
logic. Any sharding would have to be done externally to the relay.
