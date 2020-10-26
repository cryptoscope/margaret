package badger

import (
	"go.cryptoscope.co/margaret/internal/persist/badger"
	"go.cryptoscope.co/margaret/multilog/roaring"
)

func NewMultiLog(base string) (*roaring.MultiLog, error) {
	s, err := badger.New(base)
	if err != nil {
		return nil, err
	}
	return roaring.NewStore(s), nil
}
