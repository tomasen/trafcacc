# trafcacc
traffic accelerate proxy for tcp/udp

frontend duplicate and distribute data packets to multiple upstream backend.
backend picket the packet that arrived first and build connection to target network.


TODO:

- fix goroutine leak
- send udp and tcp at same time
- support udp
