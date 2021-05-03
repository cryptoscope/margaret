package flatlog

import (
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
)

type FlatLog struct{}

func Open(path string) (FlatLog, error) {

	return FlatLog{}, nil
}

// Seq returns an observable that holds the current sequence number
func (fl FlatLog) Seq() luigi.Observable {
	panic("not implemented") // TODO: Implement
}

// Get returns the entry with sequence number seq
func (fl FlatLog) Get(seq margaret.Seq) (interface{}, error) {
	panic("not implemented") // TODO: Implement
}

// Query returns a stream that is constrained by the passed query specification
func (fl FlatLog) Query(_ ...margaret.QuerySpec) (luigi.Source, error) {
	panic("not implemented") // TODO: Implement
}

// Append appends a new entry to the log
func (fl FlatLog) Append(_ interface{}) (margaret.Seq, error) {
	panic("not implemented") // TODO: Implement
}
