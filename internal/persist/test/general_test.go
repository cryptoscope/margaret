// SPDX-License-Identifier: MIT

package test

import (
	"crypto/rand"
	"crypto/sha256"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.cryptoscope.co/margaret/internal/persist"
	"go.cryptoscope.co/margaret/internal/persist/badger"
	"go.cryptoscope.co/margaret/internal/persist/fs"
	"go.cryptoscope.co/margaret/internal/persist/mkv"
	"go.cryptoscope.co/margaret/internal/persist/sqlite"

	_ "github.com/mattn/go-sqlite3"
)

func SimpleSaver(mk func(*testing.T) persist.Saver) func(*testing.T) {

	return func(t *testing.T) {
		p := mk(t)

		r := require.New(t)

		l, err := p.List()
		r.NoError(err)
		r.Len(l, 0, "%v", l)

		k := persist.Key{0, 0, 0, 1}
		d, err := p.Get(k)
		r.EqualError(err, persist.ErrNotFound.Error())
		r.Nil(d)

		testData := []byte("fooo")

		err = p.Put(k, testData)
		r.NoError(err)

		l, err = p.List()
		r.NoError(err)
		r.Len(l, 1)
		r.Equal(k, l[0])

		d, err = p.Get(k)
		r.NoError(err)
		r.Equal(d, testData)

		// mkvs limit is 64k
		n := 160 * 1024
		bigKey, big := makeRandData(r, n)
		err = p.Put(bigKey, big)
		r.NoError(err)

		l, err = p.List()
		r.NoError(err)
		r.Len(l, 2)
		r.Equal(k, l[0])
		r.Equal(bigKey, l[1])

		bigdata, err := p.Get(bigKey)
		r.NoError(err)
		r.Equal(n, len(bigdata))
		r.Equal(big, bigdata)

		//make something smaller to check dealloc of pages
		_, smaller := makeRandData(r, 75*1024)
		err = p.Put(bigKey, smaller)
		r.NoError(err)

		// test listing
		l, err = p.List()
		r.NoError(err)
		r.Len(l, 2)
		r.Equal(k, l[0])
		r.Equal(bigKey, l[1])

		getSmaller, err := p.Get(bigKey)
		r.NoError(err)
		r.Equal(len(smaller), len(getSmaller))
		r.Equal(smaller, getSmaller)

		err = p.Delete(k)
		r.NoError(err)

		l, err = p.List()
		r.NoError(err)
		r.Len(l, 1)

		r.NoError(p.Close())
	}
}

func makeRandData(r *require.Assertions, n int) (persist.Key, []byte) {
	big := make([]byte, n)
	h := sha256.New()
	tr := io.TeeReader(rand.Reader, h)
	got, err := tr.Read(big)
	r.NoError(err)
	r.Equal(n, got)
	return persist.Key(h.Sum(nil)), big
}

func TestSaver(t *testing.T) {
	t.Run("fs", SimpleSaver(makeFS))
	t.Run("sqlite", SimpleSaver(makeSqlite))
	t.Run("badger", SimpleSaver(makeBadger))
	t.Run("kv", SimpleSaver(makeMKV))
}

func makeFS(t *testing.T) persist.Saver {
	base := filepath.Join("testrun", t.Name())
	os.RemoveAll(base)
	return fs.New(base)
}

func makeBadger(t *testing.T) persist.Saver {
	base := filepath.Join("testrun", t.Name())
	//os.RemoveAll(base)
	t.Log(base)
	s, err := badger.New(base)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func makeSqlite(t *testing.T) persist.Saver {
	base := filepath.Join("testrun", t.Name())
	os.RemoveAll(base)
	s, err := sqlite.New(base)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func makeMKV(t *testing.T) persist.Saver {
	base := filepath.Join("testrun", t.Name())
	os.RemoveAll(base)
	s, err := mkv.New(base)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
