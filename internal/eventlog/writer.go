// Package eventlog provides an append-only JSON-lines event log writer and reader.
package eventlog

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/akshitanchan/execution-fairness-simulator/internal/domain"
)

// Writer writes events as JSON lines to a file.
type Writer struct {
	file   *os.File
	writer *bufio.Writer
	count  uint64
}

// NewWriter creates a new event log writer at the given path.
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

// Write appends an event to the log.
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

// Close flushes and closes the log file.
func (w *Writer) Close() error {
	if err := w.writer.Flush(); err != nil {
		w.file.Close()
		return err
	}
	return w.file.Close()
}

// Count returns the number of events written.
func (w *Writer) Count() uint64 {
	return w.count
}

// Reader reads events from a JSON-lines event log.
type Reader struct {
	file    *os.File
	scanner *bufio.Scanner
}

// NewReader opens an event log for reading.
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

// Next reads the next event. Returns nil, io.EOF at end of log.
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

// ReadAll reads all events from the log.
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

// Close closes the log file.
func (r *Reader) Close() error {
	return r.file.Close()
}
