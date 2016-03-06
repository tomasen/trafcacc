package trafcacc

// pool of upstreams
type poolu struct {
	pl  []*upstream
	id  int
	end int
}

var (
	upool = poolu{}
)

func (p *poolu) append(u *upstream) {
	p.pl = append(p.pl, u)
	p.end++
}

func (p *poolu) next() *upstream {
	p.id++
	return p.pl[p.id%p.end]
}
