package providerlog

import (
	"fmt"
	"io"
	"strings"
	"unicode"
)

type ProviderLogWriter struct {
	w    io.Writer
	line []byte
}

func NewProviderLogWriter(w io.Writer) *ProviderLogWriter {
	return &ProviderLogWriter{w: w}
}

func (w *ProviderLogWriter) Write(p []byte) (n int, err error) {
	start := 0
	for i, b := range p {
		// TODO: This probably does not hold up against encoded UTF-8
		if b != '\n' {
			continue
		}

		w.line = append(w.line, p[start:i]...)
		w.println(w.line)
		w.line = w.line[:0]
		start = i
	}

	if start != len(p)-1 {
		w.line = append(w.line, p[start:len(p)-1]...)
	}
	return len(p), nil
}

func (w *ProviderLogWriter) Close() error {
	w.println(w.line)
	return nil
}

// println assumes input has no linebreaks. It trims right spaces
// and writes the line. The line gets a [DEBUG] prefix. This is convention
// by Terraform.
func (w *ProviderLogWriter) println(bs []byte) {
	trimmed := strings.TrimRightFunc(string(bs), unicode.IsSpace)
	if trimmed == "" {
		return
	}
	// TODO: Handle possible error
	fmt.Fprintf(w.w, "[DEBUG] %s\n", trimmed)
}
