// Package progress renders download progress, mostly for terminals.
package progress

import (
	"fmt"
	"io"
	"os"
	"time"
)

// Writer renders progress to a terminal, and stays quiet (except for a single
// summary line) when writing to a file or pipe.
type Writer struct {
	w       io.Writer
	label   string
	total   int64
	written int64
	last    time.Time
	enabled bool
}

// New builds a progress writer. A nil destination discards all output.
func New(w io.Writer, label string, total int64) *Writer {
	if w == nil {
		w = io.Discard
	}

	return &Writer{
		w:       w,
		label:   label,
		total:   total,
		enabled: isTerminal(w),
	}
}

// Write records progress. It always reports a full write.
func (p *Writer) Write(b []byte) (int, error) {
	p.written += int64(len(b))
	if p.enabled && time.Since(p.last) > 100*time.Millisecond {
		p.last = time.Now()
		p.render()
	}

	return len(b), nil
}

func (p *Writer) render() {
	if p.total > 0 {
		fmt.Fprintf(p.w, "\r\033[K  %s %s / %s (%.0f%%)",
			p.label, humanBytes(p.written), humanBytes(p.total),
			float64(p.written)/float64(p.total)*100)
		return
	}

	fmt.Fprintf(p.w, "\r\033[K  %s %s", p.label, humanBytes(p.written))
}

// Done finishes the progress line.
func (p *Writer) Done() {
	if p.enabled {
		fmt.Fprintf(p.w, "\r\033[K  %s %s\n", p.label, humanBytes(p.written))
		return
	}

	if p.label != "" {
		fmt.Fprintf(p.w, "  %s %s\n", p.label, humanBytes(p.written))
	}
}

func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}

	return fi.Mode()&os.ModeCharDevice != 0
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}

	val := float64(n)
	units := []string{"KiB", "MiB", "GiB", "TiB"}
	for _, u := range units {
		val /= unit
		if val < unit {
			return fmt.Sprintf("%.1f %s", val, u)
		}
	}

	return fmt.Sprintf("%.1f PiB", val/unit)
}
