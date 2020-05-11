// SPDX-License-Identifier: MIT

package test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.cryptoscope.co/librarian"
	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
	"go.cryptoscope.co/margaret/multilog"
)

func MultilogTestAddLogAndListed(f NewLogFunc) func(*testing.T) {
	return func(t *testing.T) {
		a := assert.New(t)
		r := require.New(t)

		mlog, dir, err := f(t.Name(), margaret.BaseSeq(0), "")
		r.NoError(err)

		// empty yet
		addrs, err := mlog.List()
		r.NoError(err, "error listing mlog")
		r.Len(addrs, 0)

		var addr librarian.Addr = "f23"
		sublog, err := mlog.Get(addr)
		r.NoError(err)
		r.NotNil(sublog)

		// add some vals
		var vals = []margaret.BaseSeq{1, 2, 3}
		for i, v := range vals {
			_, err := sublog.Append(v)
			r.NoError(err, "failed to append testVal %d", i)
		}
		curr, err := sublog.Seq().Value()
		r.NoError(err, "failed to get sublog sequence")
		a.NotEqual(margaret.SeqEmpty, curr)

		// sublog was added
		ok, err := multilog.Has(mlog, addr)
		a.NoError(err)
		a.True(ok)

		addrs, err = mlog.List()
		r.NoError(err, "error listing mlog")
		r.Len(addrs, 1)
		r.Equal(addrs[0], addr)

		// reopen
		r.NoError(mlog.Close())
		mlog, dir, err = f(t.Name(), margaret.BaseSeq(0), dir)
		r.NoError(err)

		ok, err = multilog.Has(mlog, addr)
		a.NoError(err)
		a.True(ok)

		addrs, err = mlog.List()
		r.NoError(err, "error listing mlog")
		r.Len(addrs, 1)

		// empty sublogs do nothing

		for i := 0; i < 10; i++ {
			_, err := mlog.Get(librarian.Addr(fmt.Sprintf("empty%02d", i)))
			r.NoError(err)
		}

		// reopen
		r.NoError(mlog.Close())
		mlog, dir, err = f(t.Name(), margaret.BaseSeq(0), dir)
		r.NoError(err)

		addrs, err = mlog.List()
		r.NoError(err, "error listing mlog")
		r.Len(addrs, 1)

		// add a log and then delete it
		var delAddr librarian.Addr = "deleteme"
		sublog, err = mlog.Get(delAddr)
		r.NoError(err)
		r.NotNil(sublog)
		vals = []margaret.BaseSeq{99, 101, 101, 102}
		for i, v := range vals {
			_, err := sublog.Append(v)
			r.NoError(err, "failed to append testVal %d for deletion", i)
		}

		r.NoError(mlog.Flush())

		//  should have the new one
		addrs, err = mlog.List()
		r.NoError(err, "error listing mlog")
		r.Len(addrs, 2)

		// remove it
		err = mlog.Delete(delAddr)
		r.NoError(err, "delete of %s", delAddr)

		// cant use previous handle
		sv, err := sublog.Seq().Value()
		r.NoError(err, "should get a value")
		r.NotNil(sv, "should not return value for deleted sequence")
		r.EqualValues(multilog.ErrSublogDeleted, sv.(error))

		v, err := sublog.Get(margaret.BaseSeq(0))
		r.Error(err, "get shouldn't work")
		r.Nil(v, "should not return value for deleted sequence")

		seq, err := sublog.Append(666)
		r.Error(err, "append shouldn't work")
		r.Nil(seq, "should not return new sequence")

		src, err := sublog.Query()
		r.Error(err, "query shouldn't work")
		r.Nil(src, "should not return a source")

		// getting fresh, check that it is empty
		sublog, err = mlog.Get(delAddr)
		r.NoError(err)
		r.NotNil(sublog)
		curr, err = sublog.Seq().Value()
		r.NoError(err, "failed to get sublog sequence of deleted sublog")
		a.Equal(margaret.SeqEmpty, curr)

		// one left
		addrs, err = mlog.List()
		r.NoError(err, "error listing mlog")
		r.Len(addrs, 1)

		r.NoError(mlog.Close())
	}
}

func MultiLogTestSimple(f NewLogFunc) func(*testing.T) {
	type testcase struct {
		name    string
		tipe    interface{}
		specs   []margaret.QuerySpec
		values  map[librarian.Addr][]interface{}
		results map[librarian.Addr][]interface{}
		errStr  string
		live    bool
		seqWrap bool
	}

	mkTest := func(tc testcase) func(*testing.T) {
		return func(t *testing.T) {
			a := assert.New(t)
			r := require.New(t)
			ctx := context.Background()

			/*
				- make multilog
				- append values to sublogs
				- query all sublogs
				- check if entries match
			*/

			// make multilog
			mlog, dir, err := f(t.Name(), tc.tipe, "")
			r.NoError(err, "error creating multilog")

			// append values
			for addr, values := range tc.values {
				slog, err := mlog.Get(addr)
				r.NoError(err, "error getting sublog")
				for i, v := range values {
					seq, err := slog.Append(v)
					r.NoError(err, "error appending to log")
					r.Equal(margaret.BaseSeq(i), seq, "sequence missmatch")
				}
			}

			// check Has and List
			has, err := multilog.Has(mlog, librarian.Addr([]byte{0, 0, 0, 19}))
			r.NoError(err)
			r.True(has, "did not find assumed sublog")
			has, err = multilog.Has(mlog, librarian.Addr([]byte{0, 0, 0, 20}))
			r.NoError(err)
			r.False(has, "did find unassumed sublog")

			knownLogs, err := mlog.List()
			r.NoError(err, "error calling List")
			r.Len(knownLogs, len(tc.values))

			hasAddr := func(addr librarian.Addr) bool {
				for _, a := range knownLogs {
					if a == addr {
						return true
					}
				}
				return false
			}

			for addr := range tc.values {
				a.True(hasAddr(addr), "failed to find %s in List()", addr)
			}

			// check if multilog entries match
			for addr, results := range tc.results {
				slog, err := mlog.Get(addr)
				r.NoError(err, "error getting sublog")
				r.NotNil(slog, "retrieved sublog is nil")

				src, err := slog.Query(tc.specs...)
				r.NoError(err, "error querying log")

				ctx, cancel := context.WithCancel(ctx)
				defer cancel()

				waiter := make(chan struct{})
				var v_ interface{}
				err = nil

				for i, v := range results {
					go func() {
						select {
						case <-time.After(20 * time.Millisecond):
							t.Log("canceling context")
							cancel()
						case <-waiter:
						}
					}()

					v_, err = src.Next(ctx)
					// t.Logf("for prefix %x got value %v - expected %v. detailed error: %+v", addr, v_, v, err)
					if tc.errStr == "" {
						if tc.seqWrap {
							sw := v.(margaret.SeqWrapper)
							sw_ := v_.(margaret.SeqWrapper)

							a.Equal(sw.Seq(), sw_.Seq(), "sequence number doesn't match - result %d", i)
							a.Equal(sw.Value(), sw_.Value(), "value doesn't match - result %d", i)
						} else {
							a.EqualValues(v, v_, "values don't match - result %d", i)
						}
					}
					if err != nil {
						break
					}
					waiter <- struct{}{}
				}

				if err != nil && tc.errStr == "" {
					t.Errorf("unexpected error: %+v", err)
				} else if err == nil && tc.errStr != "" {
					t.Errorf("expected error %q but got nil", tc.errStr)
				} else if tc.errStr != "" && err.Error() != tc.errStr {
					t.Errorf("expected error %q but got %q", tc.errStr, err)
				}

				go func() {
					select {
					case <-time.After(time.Millisecond):
						cancel()
					case <-waiter:
					}
				}()

				v, err := src.Next(ctx)
				if !tc.live && !luigi.IsEOS(err) {
					t.Errorf("expected end-of-stream but got %+v", err)
				} else if tc.live && errors.Cause(err) != context.Canceled {
					t.Errorf("expected context canceled but got %v, %+v", v, err)
				}

				select {
				case <-time.After(time.Millisecond):
					cancel()
				case waiter <- struct{}{}:
				}
			}

			r.NoError(mlog.Close(), "failed to close testlog")

			if t.Failed() {
				t.Log("db location:", dir)
			} else {
				os.RemoveAll(dir)
			}
		}
	}

	tcs := []testcase{
		{
			name:  "simple all",
			tipe:  margaret.BaseSeq(0),
			specs: []margaret.QuerySpec{margaret.Live(true)},
			live:  true,
			values: map[librarian.Addr][]interface{}{
				librarian.Addr([]byte{0, 0, 0, 2}):  []interface{}{2, 4, 6, 8, 10, 12, 14, 16, 18},
				librarian.Addr([]byte{0, 0, 0, 3}):  []interface{}{3, 6, 9, 12, 15, 18},
				librarian.Addr([]byte{0, 0, 0, 4}):  []interface{}{4, 8, 12, 16},
				librarian.Addr([]byte{0, 0, 0, 5}):  []interface{}{5, 10, 15},
				librarian.Addr([]byte{0, 0, 0, 6}):  []interface{}{6, 12, 18},
				librarian.Addr([]byte{0, 0, 0, 7}):  []interface{}{7, 14},
				librarian.Addr([]byte{0, 0, 0, 8}):  []interface{}{8, 16},
				librarian.Addr([]byte{0, 0, 0, 9}):  []interface{}{9, 18},
				librarian.Addr([]byte{0, 0, 0, 10}): []interface{}{10},
				librarian.Addr([]byte{0, 0, 0, 11}): []interface{}{11},
				librarian.Addr([]byte{0, 0, 0, 12}): []interface{}{12},
				librarian.Addr([]byte{0, 0, 0, 12}): []interface{}{12},
				librarian.Addr([]byte{0, 0, 0, 13}): []interface{}{13},
				librarian.Addr([]byte{0, 0, 0, 14}): []interface{}{14},
				librarian.Addr([]byte{0, 0, 0, 15}): []interface{}{15},
				librarian.Addr([]byte{0, 0, 0, 16}): []interface{}{16},
				librarian.Addr([]byte{0, 0, 0, 17}): []interface{}{17},
				librarian.Addr([]byte{0, 0, 0, 18}): []interface{}{18},
				librarian.Addr([]byte{0, 0, 0, 19}): []interface{}{19},
			},
			results: map[librarian.Addr][]interface{}{
				librarian.Addr([]byte{0, 0, 0, 2}):  []interface{}{2, 4, 6, 8, 10, 12, 14, 16, 18},
				librarian.Addr([]byte{0, 0, 0, 3}):  []interface{}{3, 6, 9, 12, 15, 18},
				librarian.Addr([]byte{0, 0, 0, 4}):  []interface{}{4, 8, 12, 16},
				librarian.Addr([]byte{0, 0, 0, 5}):  []interface{}{5, 10, 15},
				librarian.Addr([]byte{0, 0, 0, 6}):  []interface{}{6, 12, 18},
				librarian.Addr([]byte{0, 0, 0, 7}):  []interface{}{7, 14},
				librarian.Addr([]byte{0, 0, 0, 8}):  []interface{}{8, 16},
				librarian.Addr([]byte{0, 0, 0, 9}):  []interface{}{9, 18},
				librarian.Addr([]byte{0, 0, 0, 10}): []interface{}{10},
				librarian.Addr([]byte{0, 0, 0, 11}): []interface{}{11},
				librarian.Addr([]byte{0, 0, 0, 12}): []interface{}{12},
				librarian.Addr([]byte{0, 0, 0, 12}): []interface{}{12},
				librarian.Addr([]byte{0, 0, 0, 13}): []interface{}{13},
				librarian.Addr([]byte{0, 0, 0, 14}): []interface{}{14},
				librarian.Addr([]byte{0, 0, 0, 15}): []interface{}{15},
				librarian.Addr([]byte{0, 0, 0, 16}): []interface{}{16},
				librarian.Addr([]byte{0, 0, 0, 17}): []interface{}{17},
				librarian.Addr([]byte{0, 0, 0, 18}): []interface{}{18},
				librarian.Addr([]byte{0, 0, 0, 19}): []interface{}{19},
			},
		},

		// BUG(cryptix): roaring does not implement reverse right now and just throws an error (https://github.com/cryptoscope/margaret/issues/7)
		{
			name:  "reverse",
			tipe:  margaret.BaseSeq(0),
			specs: []margaret.QuerySpec{margaret.Reverse(true)},
			values: map[librarian.Addr][]interface{}{
				librarian.Addr([]byte{0, 0, 0, 2}):  []interface{}{2, 4, 6, 8, 10, 12, 14, 16, 18},
				librarian.Addr([]byte{0, 0, 0, 3}):  []interface{}{3, 6, 9, 12, 15, 18},
				librarian.Addr([]byte{0, 0, 0, 4}):  []interface{}{4, 8, 12, 16},
				librarian.Addr([]byte{0, 0, 0, 5}):  []interface{}{5, 10, 15},
				librarian.Addr([]byte{0, 0, 0, 6}):  []interface{}{6, 12, 18},
				librarian.Addr([]byte{0, 0, 0, 7}):  []interface{}{7, 14},
				librarian.Addr([]byte{0, 0, 0, 8}):  []interface{}{8, 16},
				librarian.Addr([]byte{0, 0, 0, 9}):  []interface{}{9, 18},
				librarian.Addr([]byte{0, 0, 0, 10}): []interface{}{10},
				librarian.Addr([]byte{0, 0, 0, 11}): []interface{}{11},
				librarian.Addr([]byte{0, 0, 0, 12}): []interface{}{12},
				librarian.Addr([]byte{0, 0, 0, 12}): []interface{}{12},
				librarian.Addr([]byte{0, 0, 0, 13}): []interface{}{13},
				librarian.Addr([]byte{0, 0, 0, 14}): []interface{}{14},
				librarian.Addr([]byte{0, 0, 0, 15}): []interface{}{15},
				librarian.Addr([]byte{0, 0, 0, 16}): []interface{}{16},
				librarian.Addr([]byte{0, 0, 0, 17}): []interface{}{17},
				librarian.Addr([]byte{0, 0, 0, 18}): []interface{}{18},
				librarian.Addr([]byte{0, 0, 0, 19}): []interface{}{19},
			},
			results: map[librarian.Addr][]interface{}{
				librarian.Addr([]byte{0, 0, 0, 2}):  []interface{}{18, 16, 14, 12, 10, 8, 6, 4, 2},
				librarian.Addr([]byte{0, 0, 0, 3}):  []interface{}{18, 15, 12, 9, 6, 3},
				librarian.Addr([]byte{0, 0, 0, 4}):  []interface{}{16, 12, 8, 4},
				librarian.Addr([]byte{0, 0, 0, 5}):  []interface{}{15, 10, 5},
				librarian.Addr([]byte{0, 0, 0, 6}):  []interface{}{18, 12, 6},
				librarian.Addr([]byte{0, 0, 0, 7}):  []interface{}{14, 7},
				librarian.Addr([]byte{0, 0, 0, 8}):  []interface{}{16, 8},
				librarian.Addr([]byte{0, 0, 0, 9}):  []interface{}{18, 9},
				librarian.Addr([]byte{0, 0, 0, 10}): []interface{}{10},
				librarian.Addr([]byte{0, 0, 0, 11}): []interface{}{11},
				librarian.Addr([]byte{0, 0, 0, 12}): []interface{}{12},
				librarian.Addr([]byte{0, 0, 0, 12}): []interface{}{12},
				librarian.Addr([]byte{0, 0, 0, 13}): []interface{}{13},
				librarian.Addr([]byte{0, 0, 0, 14}): []interface{}{14},
				librarian.Addr([]byte{0, 0, 0, 15}): []interface{}{15},
				librarian.Addr([]byte{0, 0, 0, 16}): []interface{}{16},
				librarian.Addr([]byte{0, 0, 0, 17}): []interface{}{17},
				librarian.Addr([]byte{0, 0, 0, 18}): []interface{}{18},
				librarian.Addr([]byte{0, 0, 0, 19}): []interface{}{19},
			},
		},

		{
			name:  "limit1",
			tipe:  margaret.BaseSeq(0),
			specs: []margaret.QuerySpec{margaret.Limit(1)},
			values: map[librarian.Addr][]interface{}{
				librarian.Addr([]byte{0, 0, 0, 2}):  []interface{}{2, 4, 6, 8, 10, 12, 14, 16, 18},
				librarian.Addr([]byte{0, 0, 0, 3}):  []interface{}{3, 6, 9, 12, 15, 18},
				librarian.Addr([]byte{0, 0, 0, 4}):  []interface{}{4, 8, 12, 16},
				librarian.Addr([]byte{0, 0, 0, 5}):  []interface{}{5, 10, 15},
				librarian.Addr([]byte{0, 0, 0, 6}):  []interface{}{6, 12, 18},
				librarian.Addr([]byte{0, 0, 0, 7}):  []interface{}{7, 14},
				librarian.Addr([]byte{0, 0, 0, 8}):  []interface{}{8, 16},
				librarian.Addr([]byte{0, 0, 0, 9}):  []interface{}{9, 18},
				librarian.Addr([]byte{0, 0, 0, 10}): []interface{}{10},
				librarian.Addr([]byte{0, 0, 0, 11}): []interface{}{11},
				librarian.Addr([]byte{0, 0, 0, 12}): []interface{}{12},
				librarian.Addr([]byte{0, 0, 0, 12}): []interface{}{12},
				librarian.Addr([]byte{0, 0, 0, 13}): []interface{}{13},
				librarian.Addr([]byte{0, 0, 0, 14}): []interface{}{14},
				librarian.Addr([]byte{0, 0, 0, 15}): []interface{}{15},
				librarian.Addr([]byte{0, 0, 0, 16}): []interface{}{16},
				librarian.Addr([]byte{0, 0, 0, 17}): []interface{}{17},
				librarian.Addr([]byte{0, 0, 0, 18}): []interface{}{18},
				librarian.Addr([]byte{0, 0, 0, 19}): []interface{}{19},
			},
			results: map[librarian.Addr][]interface{}{
				librarian.Addr([]byte{0, 0, 0, 2}):  []interface{}{2},
				librarian.Addr([]byte{0, 0, 0, 3}):  []interface{}{3},
				librarian.Addr([]byte{0, 0, 0, 4}):  []interface{}{4},
				librarian.Addr([]byte{0, 0, 0, 5}):  []interface{}{5},
				librarian.Addr([]byte{0, 0, 0, 6}):  []interface{}{6},
				librarian.Addr([]byte{0, 0, 0, 7}):  []interface{}{7},
				librarian.Addr([]byte{0, 0, 0, 8}):  []interface{}{8},
				librarian.Addr([]byte{0, 0, 0, 9}):  []interface{}{9},
				librarian.Addr([]byte{0, 0, 0, 10}): []interface{}{10},
				librarian.Addr([]byte{0, 0, 0, 11}): []interface{}{11},
				librarian.Addr([]byte{0, 0, 0, 12}): []interface{}{12},
				librarian.Addr([]byte{0, 0, 0, 12}): []interface{}{12},
				librarian.Addr([]byte{0, 0, 0, 13}): []interface{}{13},
				librarian.Addr([]byte{0, 0, 0, 14}): []interface{}{14},
				librarian.Addr([]byte{0, 0, 0, 15}): []interface{}{15},
				librarian.Addr([]byte{0, 0, 0, 16}): []interface{}{16},
				librarian.Addr([]byte{0, 0, 0, 17}): []interface{}{17},
				librarian.Addr([]byte{0, 0, 0, 18}): []interface{}{18},
				librarian.Addr([]byte{0, 0, 0, 19}): []interface{}{19},
			},
		},

		{
			name:  "live and gte1",
			tipe:  margaret.BaseSeq(0),
			specs: []margaret.QuerySpec{margaret.Live(true), margaret.Gte(margaret.BaseSeq(1))},
			live:  true,
			values: map[librarian.Addr][]interface{}{
				librarian.Addr([]byte{0, 0, 0, 2}):  []interface{}{2, 4, 6, 8, 10, 12, 14, 16, 18},
				librarian.Addr([]byte{0, 0, 0, 3}):  []interface{}{3, 6, 9, 12, 15, 18},
				librarian.Addr([]byte{0, 0, 0, 4}):  []interface{}{4, 8, 12, 16},
				librarian.Addr([]byte{0, 0, 0, 5}):  []interface{}{5, 10, 15},
				librarian.Addr([]byte{0, 0, 0, 6}):  []interface{}{6, 12, 18},
				librarian.Addr([]byte{0, 0, 0, 7}):  []interface{}{7, 14},
				librarian.Addr([]byte{0, 0, 0, 8}):  []interface{}{8, 16},
				librarian.Addr([]byte{0, 0, 0, 9}):  []interface{}{9, 18},
				librarian.Addr([]byte{0, 0, 0, 10}): []interface{}{10},
				librarian.Addr([]byte{0, 0, 0, 11}): []interface{}{11},
				librarian.Addr([]byte{0, 0, 0, 12}): []interface{}{12},
				librarian.Addr([]byte{0, 0, 0, 12}): []interface{}{12},
				librarian.Addr([]byte{0, 0, 0, 13}): []interface{}{13},
				librarian.Addr([]byte{0, 0, 0, 14}): []interface{}{14},
				librarian.Addr([]byte{0, 0, 0, 15}): []interface{}{15},
				librarian.Addr([]byte{0, 0, 0, 16}): []interface{}{16},
				librarian.Addr([]byte{0, 0, 0, 17}): []interface{}{17},
				librarian.Addr([]byte{0, 0, 0, 18}): []interface{}{18},
				librarian.Addr([]byte{0, 0, 0, 19}): []interface{}{19},
			},
			results: map[librarian.Addr][]interface{}{
				librarian.Addr([]byte{0, 0, 0, 2}):  []interface{}{4, 6, 8, 10, 12, 14, 16, 18},
				librarian.Addr([]byte{0, 0, 0, 3}):  []interface{}{6, 9, 12, 15, 18},
				librarian.Addr([]byte{0, 0, 0, 4}):  []interface{}{8, 12, 16},
				librarian.Addr([]byte{0, 0, 0, 5}):  []interface{}{10, 15},
				librarian.Addr([]byte{0, 0, 0, 6}):  []interface{}{12, 18},
				librarian.Addr([]byte{0, 0, 0, 7}):  []interface{}{14},
				librarian.Addr([]byte{0, 0, 0, 8}):  []interface{}{16},
				librarian.Addr([]byte{0, 0, 0, 9}):  []interface{}{18},
				librarian.Addr([]byte{0, 0, 0, 10}): []interface{}{},
				librarian.Addr([]byte{0, 0, 0, 11}): []interface{}{},
				librarian.Addr([]byte{0, 0, 0, 12}): []interface{}{},
				librarian.Addr([]byte{0, 0, 0, 12}): []interface{}{},
				librarian.Addr([]byte{0, 0, 0, 13}): []interface{}{},
				librarian.Addr([]byte{0, 0, 0, 14}): []interface{}{},
				librarian.Addr([]byte{0, 0, 0, 15}): []interface{}{},
				librarian.Addr([]byte{0, 0, 0, 16}): []interface{}{},
				librarian.Addr([]byte{0, 0, 0, 17}): []interface{}{},
				librarian.Addr([]byte{0, 0, 0, 18}): []interface{}{},
				librarian.Addr([]byte{0, 0, 0, 19}): []interface{}{},
			},
		},

		{
			name:  "lte3",
			tipe:  margaret.BaseSeq(0),
			specs: []margaret.QuerySpec{margaret.Lte(margaret.BaseSeq(3))},
			values: map[librarian.Addr][]interface{}{
				librarian.Addr([]byte{0, 0, 0, 2}):  []interface{}{2, 4, 6, 8, 10, 12, 14, 16, 18},
				librarian.Addr([]byte{0, 0, 0, 3}):  []interface{}{3, 6, 9, 12, 15, 18},
				librarian.Addr([]byte{0, 0, 0, 4}):  []interface{}{4, 8, 12, 16},
				librarian.Addr([]byte{0, 0, 0, 5}):  []interface{}{5, 10, 15},
				librarian.Addr([]byte{0, 0, 0, 6}):  []interface{}{6, 12, 18},
				librarian.Addr([]byte{0, 0, 0, 7}):  []interface{}{7, 14},
				librarian.Addr([]byte{0, 0, 0, 8}):  []interface{}{8, 16},
				librarian.Addr([]byte{0, 0, 0, 9}):  []interface{}{9, 18},
				librarian.Addr([]byte{0, 0, 0, 10}): []interface{}{10},
				librarian.Addr([]byte{0, 0, 0, 11}): []interface{}{11},
				librarian.Addr([]byte{0, 0, 0, 12}): []interface{}{12},
				librarian.Addr([]byte{0, 0, 0, 12}): []interface{}{12},
				librarian.Addr([]byte{0, 0, 0, 13}): []interface{}{13},
				librarian.Addr([]byte{0, 0, 0, 14}): []interface{}{14},
				librarian.Addr([]byte{0, 0, 0, 15}): []interface{}{15},
				librarian.Addr([]byte{0, 0, 0, 16}): []interface{}{16},
				librarian.Addr([]byte{0, 0, 0, 17}): []interface{}{17},
				librarian.Addr([]byte{0, 0, 0, 18}): []interface{}{18},
				librarian.Addr([]byte{0, 0, 0, 19}): []interface{}{19},
			},
			results: map[librarian.Addr][]interface{}{
				librarian.Addr([]byte{0, 0, 0, 2}):  []interface{}{2, 4, 6, 8},
				librarian.Addr([]byte{0, 0, 0, 3}):  []interface{}{3, 6, 9, 12},
				librarian.Addr([]byte{0, 0, 0, 4}):  []interface{}{4, 8, 12, 16},
				librarian.Addr([]byte{0, 0, 0, 5}):  []interface{}{5, 10, 15},
				librarian.Addr([]byte{0, 0, 0, 6}):  []interface{}{6, 12, 18},
				librarian.Addr([]byte{0, 0, 0, 7}):  []interface{}{7, 14},
				librarian.Addr([]byte{0, 0, 0, 8}):  []interface{}{8, 16},
				librarian.Addr([]byte{0, 0, 0, 9}):  []interface{}{9, 18},
				librarian.Addr([]byte{0, 0, 0, 10}): []interface{}{10},
				librarian.Addr([]byte{0, 0, 0, 11}): []interface{}{11},
				librarian.Addr([]byte{0, 0, 0, 12}): []interface{}{12},
				librarian.Addr([]byte{0, 0, 0, 12}): []interface{}{12},
				librarian.Addr([]byte{0, 0, 0, 13}): []interface{}{13},
				librarian.Addr([]byte{0, 0, 0, 14}): []interface{}{14},
				librarian.Addr([]byte{0, 0, 0, 15}): []interface{}{15},
				librarian.Addr([]byte{0, 0, 0, 16}): []interface{}{16},
				librarian.Addr([]byte{0, 0, 0, 17}): []interface{}{17},
				librarian.Addr([]byte{0, 0, 0, 18}): []interface{}{18},
				librarian.Addr([]byte{0, 0, 0, 19}): []interface{}{19},
			},
		},

		{
			name:  "lt3",
			tipe:  margaret.BaseSeq(0),
			specs: []margaret.QuerySpec{margaret.Lt(margaret.BaseSeq(3))},
			values: map[librarian.Addr][]interface{}{
				librarian.Addr([]byte{0, 0, 0, 2}):  []interface{}{2, 4, 6, 8, 10, 12, 14, 16, 18},
				librarian.Addr([]byte{0, 0, 0, 3}):  []interface{}{3, 6, 9, 12, 15, 18},
				librarian.Addr([]byte{0, 0, 0, 4}):  []interface{}{4, 8, 12, 16},
				librarian.Addr([]byte{0, 0, 0, 5}):  []interface{}{5, 10, 15},
				librarian.Addr([]byte{0, 0, 0, 6}):  []interface{}{6, 12, 18},
				librarian.Addr([]byte{0, 0, 0, 7}):  []interface{}{7, 14},
				librarian.Addr([]byte{0, 0, 0, 8}):  []interface{}{8, 16},
				librarian.Addr([]byte{0, 0, 0, 9}):  []interface{}{9, 18},
				librarian.Addr([]byte{0, 0, 0, 10}): []interface{}{10},
				librarian.Addr([]byte{0, 0, 0, 11}): []interface{}{11},
				librarian.Addr([]byte{0, 0, 0, 12}): []interface{}{12},
				librarian.Addr([]byte{0, 0, 0, 12}): []interface{}{12},
				librarian.Addr([]byte{0, 0, 0, 13}): []interface{}{13},
				librarian.Addr([]byte{0, 0, 0, 14}): []interface{}{14},
				librarian.Addr([]byte{0, 0, 0, 15}): []interface{}{15},
				librarian.Addr([]byte{0, 0, 0, 16}): []interface{}{16},
				librarian.Addr([]byte{0, 0, 0, 17}): []interface{}{17},
				librarian.Addr([]byte{0, 0, 0, 18}): []interface{}{18},
				librarian.Addr([]byte{0, 0, 0, 19}): []interface{}{19},
			},
			results: map[librarian.Addr][]interface{}{
				librarian.Addr([]byte{0, 0, 0, 2}):  []interface{}{2, 4, 6},
				librarian.Addr([]byte{0, 0, 0, 3}):  []interface{}{3, 6, 9},
				librarian.Addr([]byte{0, 0, 0, 4}):  []interface{}{4, 8, 12},
				librarian.Addr([]byte{0, 0, 0, 5}):  []interface{}{5, 10, 15},
				librarian.Addr([]byte{0, 0, 0, 6}):  []interface{}{6, 12, 18},
				librarian.Addr([]byte{0, 0, 0, 7}):  []interface{}{7, 14},
				librarian.Addr([]byte{0, 0, 0, 8}):  []interface{}{8, 16},
				librarian.Addr([]byte{0, 0, 0, 9}):  []interface{}{9, 18},
				librarian.Addr([]byte{0, 0, 0, 10}): []interface{}{10},
				librarian.Addr([]byte{0, 0, 0, 11}): []interface{}{11},
				librarian.Addr([]byte{0, 0, 0, 12}): []interface{}{12},
				librarian.Addr([]byte{0, 0, 0, 12}): []interface{}{12},
				librarian.Addr([]byte{0, 0, 0, 13}): []interface{}{13},
				librarian.Addr([]byte{0, 0, 0, 14}): []interface{}{14},
				librarian.Addr([]byte{0, 0, 0, 15}): []interface{}{15},
				librarian.Addr([]byte{0, 0, 0, 16}): []interface{}{16},
				librarian.Addr([]byte{0, 0, 0, 17}): []interface{}{17},
				librarian.Addr([]byte{0, 0, 0, 18}): []interface{}{18},
				librarian.Addr([]byte{0, 0, 0, 19}): []interface{}{19},
			},
		},
	}

	return func(t *testing.T) {
		for _, tc := range tcs {
			t.Run(tc.name, mkTest(tc))
		}
	}
}
