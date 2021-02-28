package logwriter

import (
	"bytes"
)

type Writer struct {
	fn         func([]string) error
	lineBuffer []byte
}

func New(fn func([]string) error) *Writer {
	return &Writer{
		fn: fn,
	}
}

func (w *Writer) Write(data []byte) (int, error) {

	w.lineBuffer = append(w.lineBuffer, data...)

	lines := []string{}

	from := 0

	i := bytes.IndexRune(w.lineBuffer[from:], '\n')

	for i != -1 {
		line := string(w.lineBuffer[from : from+i])
		lines = append(lines, line)
		from += i + 1
		i = bytes.IndexRune(w.lineBuffer[from:], '\n')
	}

	if from < len(w.lineBuffer) {
		copy(w.lineBuffer, w.lineBuffer[from:])
	}
	w.lineBuffer = w.lineBuffer[:len(w.lineBuffer)-from]

	err := w.fn(lines)

	if err != nil {
		return 0, err
	}

	return len(data), nil
}

func (w *Writer) Close() error {
	if len(w.lineBuffer) > 0 {
		return w.fn([]string{string(w.lineBuffer)})
	}
	return nil
}
