// SPDX-License-Identifier: MIT

package roaring

import (
	"context"
	"errors"
	"fmt"
	stdlog "log"
	"sync"
	"time"

	"github.com/RoaringBitmap/roaring"
	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/luigi"

	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/internal/persist"
	"go.cryptoscope.co/margaret/internal/seqobsv"
	"go.cryptoscope.co/margaret/multilog"
)

// NewStore returns a new multilog that is only good to store sequences
// It uses files to store roaring bitmaps directly.
// for this it turns the librarian.Addrs into a hex string.
func NewStore(store persist.Saver) *MultiLog {
	ctx, cancel := context.WithCancel(context.TODO())
	ml := &MultiLog{
		store:   store,
		l:       &sync.Mutex{},
		sublogs: make(map[librarian.Addr]*sublog),
		curSeq:  margaret.BaseSeq(-2),

		processing:   ctx,
		done:         cancel,
		writerClosed: make(chan struct{}),
		tickPersist:  time.NewTicker(13 * time.Second),
	}
	go ml.writeBatches()
	return ml
}

func (log *MultiLog) writeBatches() {
	for {
		select {
		case <-log.tickPersist.C:
		case <-log.processing.Done():
			close(log.writerClosed)
			return
		}
		err := log.Flush()
		if err != nil {
			stdlog.Println("flush trigger failed", err)
		}
	}
}

func (log *MultiLog) Flush() error {
	log.l.Lock()
	defer log.l.Unlock()
	return log.flushAllSublogs()
}

func (log *MultiLog) flushAllSublogs() error {
	for addr, sublog := range log.sublogs {
		if sublog.dirty {
			err := sublog.store()
			if err != nil {
				return fmt.Errorf("roaringfiles: sublog(%x) store failed: %w", addr, err)
			}
			sublog.dirty = false
		}
	}
	return nil
}

type MultiLog struct {
	store persist.Saver

	curSeq margaret.Seq

	l       *sync.Mutex
	sublogs map[librarian.Addr]*sublog

	processing   context.Context
	done         context.CancelFunc
	writerClosed chan struct{}
	tickPersist  *time.Ticker
}

func (log *MultiLog) Get(addr librarian.Addr) (margaret.Log, error) {
	log.l.Lock()
	defer log.l.Unlock()
	return log.openSublog(addr)
}

// openSublog alters the sublogs map, take the lock first!
func (log *MultiLog) openSublog(addr librarian.Addr) (*sublog, error) {
	slog := log.sublogs[addr]
	if slog != nil {
		return slog, nil
	}

	pk := persist.Key(addr)

	var seq margaret.BaseSeq

	r, err := log.loadBitmap(pk)
	if errors.Is(err, persist.ErrNotFound) {
		seq = margaret.SeqEmpty
		r = roaring.New()
	} else if err != nil {
		return nil, err
	} else {
		seq = margaret.BaseSeq(r.GetCardinality() - 1)
	}

	var obsV uint64
	if seq > 0 {
		obsV = uint64(seq)
	}

	slog = &sublog{
		mlog:      log,
		key:       pk,
		seq:       seqobsv.New(obsV),
		luigiObsv: luigi.NewObservable(seq),
		bmap:      r,
	}
	// the better idea is to have a store that can collece puts
	log.sublogs[addr] = slog
	return slog, nil
}

// LoadInternalBitmap loads the raw roaringbitmap for key
func (log *MultiLog) LoadInternalBitmap(key librarian.Addr) (*roaring.Bitmap, error) {
	if err := log.Flush(); err != nil {
		return nil, err
	}
	return log.loadBitmap([]byte(key))
}

func (log *MultiLog) loadBitmap(key []byte) (*roaring.Bitmap, error) {
	var r *roaring.Bitmap

	data, err := log.store.Get(key)
	if err != nil {
		return nil, fmt.Errorf("roaringfiles: invalid stored bitfield %x: %w", key, err)
	}

	r = roaring.New()
	err = r.UnmarshalBinary(data)
	if err != nil {
		return nil, fmt.Errorf("roaringfiles: unpack of %x failed: %w", key, err)
	}

	return r, nil
}

func (log *MultiLog) compress(key persist.Key, r *roaring.Bitmap) (bool, error) {
	n := r.GetSizeInBytes()
	if n < 4*1024 {
		return false, nil
	}

	currSize := r.GetSerializedSizeInBytes()
	r.RunOptimize()
	newSize := r.GetSerializedSizeInBytes()

	if currSize < newSize {
		return false, nil
	}

	compressed, err := r.MarshalBinary()
	if err != nil {
		return false, fmt.Errorf("roaringfiles: compress marshal failed: %w", err)
	}
	err = log.store.Put(key, compressed)
	if err != nil {
		return false, fmt.Errorf("roaringfiles: write compressed failed: %w", err)
	}

	return true, nil
}

func (log *MultiLog) CompressAll() error {
	log.l.Lock()
	defer log.l.Unlock()

	// save open ones
	for addr, sublog := range log.sublogs {
		err := sublog.store()
		if err != nil {
			return fmt.Errorf("failed to update open sublog %x: %w", addr, err)
		}
	}
	// load idle ones
	err := log.loadAll()
	if err != nil {
		return fmt.Errorf("failed to load all sublogs: %w", err)
	}

	// compress all
	for addr, sublog := range log.sublogs {
		_, err := log.compress(persist.Key(addr), sublog.bmap)
		if err != nil {
			return fmt.Errorf("failed to update open sublog %x: %w", addr, err)
		}
	}
	return nil
}

func (log *MultiLog) Delete(addr librarian.Addr) error {
	log.l.Lock()
	defer log.l.Unlock()

	if sl, ok := log.sublogs[addr]; ok {
		sl.deleted = true
		sl.luigiObsv.Set(multilog.ErrSublogDeleted)
		delete(log.sublogs, addr)
	}

	return log.store.Delete(persist.Key(addr))
}

// List returns a list of all stored sublogs
func (log *MultiLog) List() ([]librarian.Addr, error) {
	log.l.Lock()
	defer log.l.Unlock()

	err := log.loadAll()
	if err != nil {
		return nil, err
	}

	list := make([]librarian.Addr, len(log.sublogs))
	i := 0
	for addr, sublog := range log.sublogs {
		if sublog.bmap.GetCardinality() == 0 {
			continue
		}
		list[i] = addr
		i++
	}
	list = list[:i] // cut off the skipped ones

	return list, nil
}

func (log *MultiLog) loadAll() error {
	keys, err := log.store.List()
	if err != nil {
		return fmt.Errorf("roaringfiles: store iteration failed: %w", err)
	}
	for _, bk := range keys {
		_, err := log.openSublog(librarian.Addr(bk))
		if err != nil {
			return fmt.Errorf("roaringfiles: broken bitmap file (%s): %w", bk, err)
		}
	}
	return nil
}

func (log *MultiLog) Close() error {
	log.done()
	log.tickPersist.Stop()
	<-log.writerClosed

	if err := log.Flush(); err != nil {
		return fmt.Errorf("roaringfiles: close failed to flush: %w", err)
	}

	return log.store.Close()
}
