package trafcacc

import (
	"sync"

	"github.com/Sirupsen/logrus"
)

type connCache struct {
	sync.RWMutex
	seqence map[uint32]*packet
	lastack uint32
}

type writeCache struct {
	sync.RWMutex
	conns map[uint64]*connCache
}

func newWriteCache() *writeCache {
	return &writeCache{conns: make(map[uint64]*connCache)}
}

func (c *writeCache) add(p *packet) {
	if p.Buf == nil {
		return
	}

	key := packetKey(p.Senderid, p.Connid)

	c.Lock()
	cn, exist := c.conns[key]
	if !exist {
		// TODO: only create when connect or conected
		cn = &connCache{
			seqence: make(map[uint32]*packet),
		}
		c.conns[key] = cn
	}
	c.Unlock()

	cn.Lock()
	if p.Seqid > cn.lastack {
		if _, ok := cn.seqence[p.Seqid]; !ok {
			cn.seqence[p.Seqid] = p
		}
	}
	cn.Unlock()

	return
}

func (c *writeCache) get(senderid, connid, seqid uint32) *packet {
	key := packetKey(senderid, connid)

	c.RLock()
	cn, exist := c.conns[key]
	c.RUnlock()
	if !exist {
		return nil
	}

	cn.Lock()
	defer cn.Unlock()
	return cn.seqence[seqid]
}

func (c *writeCache) ack(senderid, connid, seqid uint32) {
	key := packetKey(senderid, connid)

	c.RLock()
	cn, exist := c.conns[key]
	c.RUnlock()
	if !exist {
		return
	}

	cn.Lock()
	logrus.WithFields(logrus.Fields{
		"senderid": senderid,
		"connid":   connid,
		"seqid":    seqid,
	}).Debugln("clean cached packet")
	for k := range cn.seqence {
		if k <= seqid {
			delete(cn.seqence, k)
		} else {
			// maybe performance effictive
			break
		}
	}
	if seqid > cn.lastack {
		cn.lastack = seqid
	}
	cn.Unlock()
}

func (c *writeCache) close(senderid, connid uint32) {
	key := packetKey(senderid, connid)

	c.Lock()
	delete(c.conns, key)
	c.Unlock()
}
