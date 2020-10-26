// SPDX-License-Identifier: MIT

package test // import "go.cryptoscope.co/margaret/multilog/test"

import (
	"context"
	"encoding/binary"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
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

func SinkTestSimple(f NewLogFunc) func(*testing.T) {
	type testcase struct {
		tipe    interface{}
		values  []interface{}
		specs   []margaret.QuerySpec
		f       func(t *testing.T) multilog.Func
		result  map[librarian.Addr][]interface{}
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
				- make multilog and sink
				- query entire log and pump stream into multilog-sink
				- append values to log
				- check if multilog entries match
			*/

			// make multilog
			mlog, dir, err := f(t.Name(), tc.tipe, "")
			r.NoError(err, "error creating multilog")
			defer func() {
				err := mlog.Close()
				if err != nil {
					t.Error("mlog close", err)
				}
				if t.Failed() {
					t.Log("db location:", dir)
				} else {
					os.RemoveAll(dir)
				}
			}()

			// make file that tracks current sequence number
			prefix := "curSeq-" + strings.Replace(t.Name(), "/", "_", -1) + "-"
			file, err := ioutil.TempFile(".", prefix)
			r.NoError(err, "error creating curseq file")
			defer func() {
				err := os.Remove(file.Name())
				if err != nil {
					t.Error("seq file rm", err)
				}
			}()

			sink := multilog.NewSink(file, mlog, tc.f(t))

			// append values
			for i, v := range tc.values {
				//err := sink.Pour(ctx, multilog.WithValue(margaret.BaseSeq(i), v))
				err := sink.Pour(ctx, margaret.WrapWithSeq(v, margaret.BaseSeq(i)))
				a.NoError(err, "error pouring into sink")
			}

			// check if multilog entries match
			for addr, results := range tc.result {
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

				for _, v := range results {
					go func() {
						select {
						case <-time.After(50 * time.Millisecond):
							t.Log("canceling context")
							cancel()
						case <-waiter:
						}
					}()
					func() {
						ctx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
						defer cancel()

						v_, err = src.Next(ctx)
						if tc.errStr == "" {
							if tc.seqWrap {
								sw := v.(margaret.SeqWrapper)
								sw_ := v_.(margaret.SeqWrapper)

								a.Equal(sw.Seq(), sw_.Seq(), "sequence number doesn't match")
								a.Equal(sw.Value(), sw_.Value(), "value doesn't match")
							} else {
								a.EqualValues(v, v_, "values don't match")
							}
						}
					}()
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
		}
	}

	tcs := []testcase{
		{
			tipe:   margaret.BaseSeq(0),
			values: count(0, 20),
			f: func(t *testing.T) multilog.Func {
				return func(ctx context.Context, seq margaret.Seq, v interface{}, mlog multilog.MultiLog) (err error) {
					facs := uniq(factorize(int(v.(margaret.Seq).Seq())))
					for _, fac := range facs {
						prefixBs := make([]byte, 4)
						binary.BigEndian.PutUint32(prefixBs, uint32(fac))
						prefix := librarian.Addr(prefixBs)

						var slog margaret.Log
						slog, err = mlog.Get(prefix)
						if err != nil {
							err = errors.Wrapf(err, "error getting sublog for prefix %d", fac)
							return err
						}

						_, err = slog.Append(seq.Seq())
						if err != nil {
							err = errors.Wrapf(err, "error appending to sublog for prefix %d", fac)
							return err
						}
					}

					err = nil
					return nil
				}
			},
			specs: []margaret.QuerySpec{margaret.Live(true)},
			live:  true,
			result: map[librarian.Addr][]interface{}{
				librarian.Addr([]byte{0, 0, 0, 2}):  []interface{}{2, 4, 6, 8, 10, 12, 14, 16, 18},
				librarian.Addr([]byte{0, 0, 0, 3}):  []interface{}{3, 6, 9, 12, 15, 18},
				librarian.Addr([]byte{0, 0, 0, 5}):  []interface{}{5, 10, 15},
				librarian.Addr([]byte{0, 0, 0, 7}):  []interface{}{7, 14},
				librarian.Addr([]byte{0, 0, 0, 11}): []interface{}{11},
				librarian.Addr([]byte{0, 0, 0, 13}): []interface{}{13},
				librarian.Addr([]byte{0, 0, 0, 17}): []interface{}{17},
				librarian.Addr([]byte{0, 0, 0, 19}): []interface{}{19},
			},
		},
	}

	return func(t *testing.T) {
		for i, tc := range tcs {
			t.Run(fmt.Sprint(i), mkTest(tc))
		}
	}
}

func count(from, to int) []interface{} {
	out := make([]interface{}, to-from)
	for i := from; i < to; i++ {
		out[i-from] = margaret.BaseSeq(i)
	}
	return out
}

func factorize(n int) []int {
	if n == 0 {
		return nil
	}
	var out []int

	for i := 2; n != 1; i++ {
		for n != 0 && n%i == 0 {
			out = append(out, i)
			n = n / i
		}
	}

	return out
}

func uniq(ints []int) []int {
	var out []int

	for _, n := range ints {
		if len(out) == 0 {
			out = append(out, n)
			continue
		}

		if n != out[len(out)-1] {
			out = append(out, n)
		}
	}

	return out
}
