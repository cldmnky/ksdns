package changelog

import "github.com/miekg/dns"

const (
	TypeInsert = iota
	TypeRemove
	TypeRemoveName
	TypeRemoveRRSet
)

type Record struct {
	Type   int
	Record string
	Offset uint64
	Zone   string
}

func NewRecord(zone string, rr dns.RR, rType int) *Record {
	return &Record{
		Type:   rType,
		Record: rr.String(),
		Zone:   zone,
	}
}
