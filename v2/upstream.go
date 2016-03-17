package trafcacc

import (
	"encoding/gob"
	"net"

	"github.com/Sirupsen/logrus"
)

type upstream struct {
	proto string
	addr  string

	conn    net.Conn
	encoder *gob.Encoder
	decoder *gob.Decoder
	// mux     sync.RWMutex
}

func (u *upstream) ping() error {
	err := u.encoder.Encode(&packet{Cmd: ping})
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"error": err,
		}).Warnln("Dialer ping upstream error")
	}
	return err
}
