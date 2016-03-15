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

// Accelerate traffic start by flag strings
func (t *trafcacc) accelerate(l, u string) {

	switch t.role {
	case BACKEND:
		// TODO: listen to l-s
		// connect to upstream
	case FRONTEND:
		// TODO: listen to l
		// use Dialer to init connection
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
