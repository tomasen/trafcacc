# trafcacc
traffic accelerate proxy for tcp/udp

frontend duplicate and distribute data packets to multiple upstream backend.
backend picket the packet that arrived first and build connection to target network.


TODO:

- send 2 packet at a time
- support udp
- send udp and tcp at same time

- fix goroutine leak
