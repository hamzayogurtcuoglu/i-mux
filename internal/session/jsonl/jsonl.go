// Package jsonl reads line-oriented session files within bounded windows.
//
// Large agent transcripts are tens of MB. Read visits only a head window
// (the original request and metadata) and a tail window (recent history);
// the tail starts at the first full line inside the window. Lines longer than
// MaxLine are skipped and counted. Reverse scans a file backwards line by
// line (usage windows).
package jsonl

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
)

// DefaultMaxLine is the default per-line cap (16 MiB).
const DefaultMaxLine = 16 << 20

// Options bounds a Read.
type Options struct {
	HeadBytes int64 // lines that start before this offset form the head window
	TailBytes int64 // the last TailBytes of the file form the tail window
	MaxLine   int   // lines longer than this are skipped; 0 = DefaultMaxLine
}

// Stats describes what Read visited.
type Stats struct {
	Size        int64 // file size
	Lines       int   // lines delivered to the callback
	Gap         bool  // the middle of the file was not read
	GapBytes    int64 // size of the unread middle
	SkippedLong int   // lines skipped because they exceeded MaxLine
	TailStart   int64 // offset of the first tail line (when Gap)
}

// Warnings renders the stats' problems as human-readable warnings.
func (s Stats) Warnings() []string {
	var w []string
	if s.SkippedLong > 0 {
		w = append(w, fmt.Sprintf("%d oversized line(s) skipped", s.SkippedLong))
	}
	return w
}

// LineFunc receives one line without its trailing "\n" or "\r\n". The slice
// is only valid during the call. off is the line's byte offset. Returning
// ErrStop ends the scan without error.
type LineFunc func(line []byte, off int64) error

// ErrStop stops a scan early.
var ErrStop = errors.New("stop")

const bufSize = 256 << 10

// Read calls fn for every line of the head and tail windows, in file order.
func Read(path string, opt Options, fn LineFunc) (Stats, error) {
	f, err := os.Open(path)
	if err != nil {
		return Stats{}, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return Stats{}, err
	}
	return ReadAt(f, fi.Size(), opt, fn)
}

// ReadAt is Read over any io.ReaderAt of the given size.
func ReadAt(r io.ReaderAt, size int64, opt Options, fn LineFunc) (Stats, error) {
	if opt.MaxLine <= 0 {
		opt.MaxLine = DefaultMaxLine
	}
	st := Stats{Size: size}
	whole := opt.HeadBytes <= 0 || opt.TailBytes <= 0 || opt.HeadBytes+opt.TailBytes >= size
	if whole {
		_, err := scan(io.NewSectionReader(r, 0, size), 0, -1, opt.MaxLine, &st, fn)
		return st, ignoreStop(err)
	}
	headEnd, err := scan(io.NewSectionReader(r, 0, size), 0, opt.HeadBytes, opt.MaxLine, &st, fn)
	if err != nil {
		return st, ignoreStop(err)
	}
	tailStart := size - opt.TailBytes
	if tailStart <= headEnd {
		_, err = scan(io.NewSectionReader(r, headEnd, size-headEnd), headEnd, -1, opt.MaxLine, &st, fn)
		return st, ignoreStop(err)
	}
	// Find the first full line inside the tail window.
	start, err := nextLineStart(r, tailStart, size)
	if err != nil {
		return st, err
	}
	st.Gap = true
	st.GapBytes = start - headEnd
	st.TailStart = start
	if start >= size {
		return st, nil
	}
	_, err = scan(io.NewSectionReader(r, start, size-start), start, -1, opt.MaxLine, &st, fn)
	return st, ignoreStop(err)
}

func ignoreStop(err error) error {
	if errors.Is(err, ErrStop) {
		return nil
	}
	return err
}

// nextLineStart returns the offset of the first line that starts at or after
// off (off itself if the previous byte is a newline).
func nextLineStart(r io.ReaderAt, off, size int64) (int64, error) {
	if off <= 0 {
		return 0, nil
	}
	var b [1]byte
	if _, err := r.ReadAt(b[:], off-1); err != nil {
		return 0, err
	}
	if b[0] == '\n' {
		return off, nil
	}
	buf := make([]byte, 64<<10)
	for pos := off; pos < size; {
		n, err := r.ReadAt(buf, pos)
		if i := bytes.IndexByte(buf[:n], '\n'); i >= 0 {
			return pos + int64(i) + 1, nil
		}
		pos += int64(n)
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, err
		}
	}
	return size, nil
}

// scan reads lines from r (which starts at file offset base). It stops
// before a line that starts at or after stopAt (stopAt < 0: no limit) and
// returns the offset right after the last consumed line.
func scan(r io.Reader, base, stopAt int64, maxLine int, st *Stats, fn LineFunc) (int64, error) {
	br := bufio.NewReaderSize(r, bufSize)
	off := base
	var long []byte
	for {
		if stopAt >= 0 && off >= stopAt {
			return off, nil
		}
		lineStart := off
		chunk, err := br.ReadSlice('\n')
		off += int64(len(chunk))
		var line []byte
		skipped := false
		if errors.Is(err, bufio.ErrBufferFull) {
			long = append(long[:0], chunk...)
			for errors.Is(err, bufio.ErrBufferFull) {
				chunk, err = br.ReadSlice('\n')
				off += int64(len(chunk))
				if !skipped {
					if len(long)+len(chunk) > maxLine+2 {
						skipped = true
						long = long[:0]
					} else {
						long = append(long, chunk...)
					}
				}
			}
			line = long
		} else {
			line = chunk
		}
		if err != nil && !errors.Is(err, io.EOF) {
			return off, err
		}
		line = bytes.TrimSuffix(line, []byte{'\n'})
		line = bytes.TrimSuffix(line, []byte{'\r'})
		switch {
		case skipped || len(line) > maxLine:
			st.SkippedLong++
		case len(line) > 0:
			st.Lines++
			if ferr := fn(line, lineStart); ferr != nil {
				return off, ferr
			}
		}
		if errors.Is(err, io.EOF) {
			return off, nil
		}
	}
}

// Reverse yields the lines of a file from last to first.
type Reverse struct {
	r       io.ReaderAt
	pos     int64 // bytes before pos are not yet consumed
	buf     []byte
	pending []byte // partial line carried between chunks (in file order)
	maxLine int
	skipped int
	err     error
	line    []byte
}

// NewReverse returns a reverse scanner over r of the given size.
func NewReverse(r io.ReaderAt, size int64, maxLine int) *Reverse {
	if maxLine <= 0 {
		maxLine = DefaultMaxLine
	}
	return &Reverse{r: r, pos: size, buf: make([]byte, 64<<10), maxLine: maxLine}
}

// Scan advances to the previous line. It returns false at the start of the
// file or on error.
func (rv *Reverse) Scan() bool {
	for {
		if i := bytes.LastIndexByte(rv.pending, '\n'); i >= 0 {
			line := rv.pending[i+1:]
			rv.pending = rv.pending[:i]
			line = bytes.TrimSuffix(line, []byte{'\r'})
			if len(line) == 0 {
				continue
			}
			if len(line) > rv.maxLine {
				rv.skipped++
				continue
			}
			rv.line = line
			return true
		}
		if rv.pos == 0 {
			line := bytes.TrimSuffix(rv.pending, []byte{'\r'})
			rv.pending = nil
			if len(line) == 0 {
				return false
			}
			if len(line) > rv.maxLine {
				rv.skipped++
				return false
			}
			rv.line = line
			return true
		}
		n := int64(len(rv.buf))
		if n > rv.pos {
			n = rv.pos
		}
		rv.pos -= n
		chunk := make([]byte, n, n+int64(len(rv.pending)))
		if _, err := rv.r.ReadAt(chunk, rv.pos); err != nil && !errors.Is(err, io.EOF) {
			rv.err = err
			return false
		}
		// Bound memory for absurdly long lines: drop the partial line once it
		// exceeds the cap (it will be counted when its start is found).
		if len(rv.pending) > rv.maxLine+1 && bytes.IndexByte(chunk, '\n') < 0 {
			rv.pending = rv.pending[:rv.maxLine+1]
		}
		rv.pending = append(chunk, rv.pending...)
	}
}

// Bytes returns the current line (valid until the next Scan).
func (rv *Reverse) Bytes() []byte { return rv.line }

// Err returns the first I/O error.
func (rv *Reverse) Err() error { return rv.err }

// Skipped returns the number of oversized lines skipped.
func (rv *Reverse) Skipped() int { return rv.skipped }
