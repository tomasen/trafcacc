### traffic accelerate proxy for tcp/udp
[![Build Status](https://travis-ci.org/tomasen/trafcacc.svg?branch=master)](https://travis-ci.org/tomasen/trafcacc)
[![Coverage Status](https://coveralls.io/repos/tomasen/trafcacc/badge.svg?branch=master&service=github)](https://coveralls.io/github/tomasen/trafcacc?branch=master)

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

`go test -bench .`

#### TODO

- 修正 backend 和 frontend 必须同时重启来保证connid一致并才能正常工作的问题。
这个问题产生的原因是connid由frontend生成，同时backend有需要保留一个已经关闭的connid的列表
来drop哪些已经关闭的连接的数据包，而不是为已经关闭的连接建立新的到remote-addr的连接。
所以backend会假设connid一定是递增的。解决方案或许是数据包加入serverid，而serverid是每次frontend重启时随机生成
- send 2 packet at a time。问题是是否应该在这里使用goroutine。使用goroutine会让问题复杂化，例如更多的内存复制或锁。
- support udp 和 send udp and tcp at same time ， 后者和上一个问题 send 2 packet at a time 也有关
- fix goroutine leak if any
