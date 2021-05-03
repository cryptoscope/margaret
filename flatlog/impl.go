package flatlog

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"go.cryptoscope.co/luigi"
	"go.cryptoscope.co/margaret"
)

type FlatLog struct {
	writer *os.File

	currSeq int64
}

func Open(path string) (*FlatLog, error) {
	fl := FlatLog{}

	err := os.MkdirAll(filepath.Dir(path), 0700)
	if err != nil {
		return nil, err
	}

	fl.writer, err = os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0700)
	if err != nil {
		return nil, err
	}

	fl.currSeq, err = countLines(fl.writer)
	fmt.Println("flatLog opened with ", fl.currSeq)
	return &fl, err
}

func (fl FlatLog) FileName() string {
	return fl.writer.Name()
}

func countLines(f *os.File) (int64, error) {
	fullFile, err := ioutil.ReadAll(f)
	if err != nil {
		return -1, err
	}

	_, err = f.Seek(0, io.SeekStart)
	if err != nil {
		return -1, err
	}

	newLines := strings.Count(string(fullFile), "\n")

	return int64(newLines - 1), err
}

// Seq returns an observable that holds the current sequence number
func (fl FlatLog) Seq() luigi.Observable {
	panic("not implemented") // TODO: Implement
}

// Get returns the entry with sequence number seq
func (fl FlatLog) Get(seq margaret.Seq) (interface{}, error) {
	// rememer for jumping back later
	whereAmI, err := fl.writer.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, err
	}

	// now reset to start
	_, err = fl.writer.Seek(0, io.SeekStart)
	if err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(fl.writer)

	var i = seq.Seq()
	var ourLine string
	for scanner.Scan() {

		if i == 0 {
			ourLine = scanner.Text()
			break
		}

		i--
	}

	if i < 0 {
		return nil, margaret.OOB
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	// jump back to where we were
	_, err = fl.writer.Seek(whereAmI, io.SeekStart)
	if err != nil {
		return nil, err
	}

	// find replacment thingy
	lineLength := len(ourLine)
	replacer := ourLine[lineLength-1:]

	intendedLine := strings.ReplaceAll(ourLine[:lineLength-1], replacer, "\n")

	return intendedLine, nil
}

// Query returns a stream that is constrained by the passed query specification
func (fl FlatLog) Query(_ ...margaret.QuerySpec) (luigi.Source, error) {
	panic("not implemented") // TODO: Implement
}

// Append appends a new entry to the log
func (fl *FlatLog) Append(val interface{}) (margaret.Seq, error) {
	// totally random replacement byte for newlines
	newLineReplace := "\x07"

	var out string
	switch tv := val.(type) {
	case int64:
		out = strconv.FormatInt(tv, 10)
	case int:
		out = strconv.Itoa(tv)
	case string:
		if strings.Index(tv, newLineReplace) != -1 {
			return nil, fmt.Errorf("need a different replacement thingy")
		}

		out = strings.ReplaceAll(tv, "\n", newLineReplace)
	}

	_, err := fmt.Fprintln(fl.writer, out+newLineReplace)
	if err != nil {
		return nil, err
	}

	fl.currSeq += 1

	return margaret.BaseSeq(fl.currSeq), nil
}
