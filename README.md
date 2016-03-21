### traffic accelerate proxy for tcp/udp
[![Build Status](https://travis-ci.org/tomasen/trafcacc.svg?branch=master)](https://travis-ci.org/tomasen/trafcacc)
[![Coverage Status](https://coveralls.io/repos/tomasen/trafcacc/badge.svg?branch=master&service=github)](https://coveralls.io/github/tomasen/trafcacc?branch=master)
[![GoDoc](https://godoc.org/github.com/tomasen/trafcacc/v2?status.svg)](https://godoc.org/github.com/tomasen/trafcacc/v2)

frontend duplicate and distribute data packets to multiple upstream backend.
backend pick the packet that arrived first and send to the target address and
vice versa.

### Build

`docker build` or `GOOS=linux go build`

### Run

front-end:
`/home/ubuntu/trafcacc -listen=tcp://:5201 -upstream=tcp://backend-addresss:51501-51524 -v`

back-end:
`trafcacc -backend=true -listen=tcp://:51501-51524 -upstream=tcp://remote-address:5201 -v`


### Benchmark

`go test -bench .` 如果使用 iperf3 `IPERF=1 go test`

#### TODO

- support udp 和 send udp and tcp at same time， 后者和上一个问题 send 2 packets at the same time 也有关
