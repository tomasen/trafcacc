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

- 记录每个 upstream 的 latency 并合理安排优先使用那些upstream
- 更合理的控制要求重发的频率
- 如果backend没有重新启动 frontend就连接上去，会收到不存在的packetQueue 导致退出的问题
- performance improvement 提高性能、速度和响应时间。目前问题：写时需要加锁，否则就要大量memcopy，需要找折中方案； buffersize 为了避免udp message too long的问题必须设置的比较小，可能因此导致性能下降；其他性能瓶颈
