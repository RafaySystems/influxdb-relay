# recovery

InfluxDB  organizes  its data  on  disk  into  logical  blocks of  time  called
shards. We can use this to create a hot recovery process with zero downtime.

The length of  time that shards represent  in InfluxDB are typically  1 hour, 1
day, or 7 days, depending on the  retention duration, but can be explicitly set
when creating the  retention policy. For the sake of  our example, let's assume
shard durations of 1 day.

Let's say one of the InfluxDB servers goes down for an hour on 2016-03-10. Once
midnight UTC  rolls over, all  InfluxDB processes are  now writing data  to the
shard  for  2016-03-11 and  the  file(s)  for  2016-03-10  have gone  cold  for
writes. We can then restore things using these steps:

1. Tell the load balancer to stop  sending query traffic to the server that was
   down  (this should  be done  as soon  as an  outage is  detected to  prevent
   partial or inconsistent query returns.)
2. Create backup of 2016-03-10 shard from a server that was up the entire day
3. Restore the backup of the shard from  the good server to the server that had
   downtime
4. Tell  the load balancer to  resume sending queries to  the previously downed
   server

During this entire  process the Relays should be sending  current writes to all
servers, including the one with downtime.
