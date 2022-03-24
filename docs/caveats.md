# caveats

While `influxdb-relay` does provide some  level of high availability, there are
a few scenarios that need to be accounted for:

- Continuous queries  will still only write their results  locally. If a server
  goes down, the continuous query will have to be backfilled after the data has
  been recovered for that instance.
- Overwriting points is potentially unpredictable. For example, given servers A
  and B, if  B is down, and point  X is written (we'll call the  value X1) just
  before B  comes back online,  that write is  queued behind every  other write
  that occurred while B was offline. Once  B is back online, the first buffered
  write succeeds, and  all new writes are now allowed  to pass-through. At this
  point (before  X1 is written to  B), X is  written again (with value  X2 this
  time) to both A and B. When the relay reaches the end of B's buffered writes,
  it will write X (with value X1) to B... At this point A now has X2, but B has
  X1.
  - It  is probably best to  avoid re-writing points (if  possible). Otherwise,
    please be aware that overwriting the same  field for a given point can lead
    to data differences.
  - This  could potentially  be mitigated  by waiting for  the buffer  to flush
    before opening writes back up to being passed-through.
- When a request is buffered, the client recieves a `202` HTTP response
  indicating that his request will be fullfilled later. So the client will
  never reveive the response of the actual request.
