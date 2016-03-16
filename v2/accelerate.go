package trafcacc

// tag is type of role: BACKEND or FRONTEND
type tag bool

// BACKEND FRONTEND role tag
const (
	BACKEND  tag = true
	FRONTEND tag = false
)

type trafcacc struct {
	role tag
}

// Trafcacc give a interface to query running status
type Trafcacc interface {
	Status()
}

// Accelerate traffic by listen to l, and connect to u
func (t *trafcacc) accelerate(l, u string) {

	switch t.role {
	case BACKEND:
		// TODO: setup trafcacc.Server to listen to and HandleFunc from l
		// connect to upstream
	case FRONTEND:
		// TODO: listen to l
		// use trafcacc.Dialer to init connection to u
	}

}

func (t *trafcacc) roleString() string {
	switch t.role {
	case BACKEND:
		return "backend"
	case FRONTEND:
		return "frontend"
	}
	return "unknown"
}
