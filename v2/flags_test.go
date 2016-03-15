package trafcacc

import "testing"

func TestFlags(t *testing.T) {

	if !testEq(parse("tcp://:5000"),
		[]endpoint{endpoint{proto: "tcp", portBegin: 5000, portEnd: 5000}}) {
		t.Fail()
	}

	if !testEq(parse("udp://127.0.0.1:5000-6000"),
		[]endpoint{endpoint{proto: "udp", host: "127.0.0.1", portBegin: 5000, portEnd: 6000}}) {
		t.Fail()
	}

	if !testEq(parse("udp://127.0.0.1:2000-2100,tcp://192.168.1.1:2000-2050"),
		[]endpoint{endpoint{proto: "udp", host: "127.0.0.1", portBegin: 2000, portEnd: 2100},
			endpoint{proto: "tcp", host: "192.168.1.1", portBegin: 2000, portEnd: 2050}}) {
		t.Fail()
	}
}

func testEq(a, b []endpoint) bool {

	if a == nil && b == nil {
		return true
	}

	if a == nil || b == nil {
		return false
	}

	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}
