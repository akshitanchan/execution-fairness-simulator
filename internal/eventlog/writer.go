// Package eventlog provides JSON-lines event log I/O
package eventlog

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/akshitanchan/execution-fairness-simulator/internal/domain"
)

type Writer struct {
	file   *os.File
	writer *bufio.Writer
	count  uint64
}

func NewWriter(path string) (*Writer, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("create event log: %w", err)
	}
	return &Writer{
		file:   f,
		writer: bufio.NewWriterSize(f, 64*1024),
	}, nil
}

func (w *Writer) Write(event *domain.Event) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	_, err = w.writer.Write(data)
	if err != nil {
		return err
	}
	err = w.writer.WriteByte('\n')
	if err != nil {
		return err
	}
	w.count++
	return nil
}

func (w *Writer) Close() error {
	if err := w.writer.Flush(); err != nil {
		w.file.Close()
		return err
	}
	return w.file.Close()
}

func (w *Writer) Count() uint64 {
	return w.count
}

type Reader struct {
	file    *os.File
	scanner *bufio.Scanner
}

func NewReader(path string) (*Reader, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open event log: %w", err)
	}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 256*1024), 1024*1024)
	return &Reader{
		file:    f,
		scanner: scanner,
	}, nil
}

func (r *Reader) Next() (*domain.Event, error) {
	if !r.scanner.Scan() {
		if err := r.scanner.Err(); err != nil {
			return nil, err
		}
		return nil, io.EOF
	}
	var event domain.Event
	if err := json.Unmarshal(r.scanner.Bytes(), &event); err != nil {
		return nil, fmt.Errorf("unmarshal event: %w", err)
	}
	return &event, nil
}

func (r *Reader) ReadAll() ([]*domain.Event, error) {
	var events []*domain.Event
	for {
		e, err := r.Next()
		if err == io.EOF {
			return events, nil
		}
		if err != nil {
			return events, err
		}
		events = append(events, e)
	}
}

func (r *Reader) Close() error {
	return r.file.Close()
}
