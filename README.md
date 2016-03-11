# trafcacc
[![Build Status](https://travis-ci.org/tomasen/trafcacc.svg?branch=master)](https://travis-ci.org/tomasen/trafcacc)
[![Coverage Status](https://coveralls.io/repos/tomasen/trafcacc/badge.svg?branch=master&service=github)](https://coveralls.io/github/tomasen/trafcacc?branch=master)

traffic accelerate proxy for tcp/udp

frontend duplicate and distribute data packets to multiple upstream backend.
backend picket the packet that arrived first and build connection to target network.


TODO:

- send 2 packet at a time
- support udp
- send udp and tcp at same time

- fix goroutine leak
