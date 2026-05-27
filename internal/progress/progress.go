package progress

import (
	"fmt"
	"strings"
	"time"
)

type Bar struct {
	total      int64
	start      time.Time
	lastPrint  time.Time
	lastBytes  int64
	printEvery int64 // redraw every N bytes
	width      int
}

func NewBar(total int64) *Bar {
	// Aim for ~100 redraws; never less than 1 MB per step.
	printEvery := total / 100
	if printEvery < 1<<20 {
		printEvery = 1 << 20
	}
	return &Bar{
		total:      total,
		start:      time.Now(),
		printEvery: printEvery,
		width:      28,
	}
}

func (p *Bar) Print(current int64) {
	if current < p.total {
		bytesMoved := current-p.lastBytes >= p.printEvery
		timePassed := time.Since(p.lastPrint) >= 200*time.Millisecond
		// Redraw if either condition is met; skip if neither is.
		if !bytesMoved && !timePassed {
			return
		}
	}
	p.lastPrint = time.Now()
	p.lastBytes = current

	elapsed := time.Since(p.start).Seconds()

	var pct float64
	if p.total > 0 {
		pct = float64(current) / float64(p.total)
	}

	filled := int(pct * float64(p.width))
	if filled > p.width {
		filled = p.width
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", p.width-filled)

	speed := "---"
	eta := ""
	if elapsed > 0.5 {
		bps := float64(current) / elapsed
		speed = FormatBytes(int64(bps)) + "/s"
		if bps > 0 && current < p.total {
			remaining := float64(p.total-current) / bps
			eta = "  ETA " + formatDuration(time.Duration(remaining*float64(time.Second)))
		}
	}

	// \x1b[K clears from cursor to end of line, preventing remnants from
	// longer previous lines showing through (e.g. "ETA 0s" after done()).
	fmt.Printf("\r  [%s] %5.1f%%  %s / %s  %s%s\x1b[K",
		bar, pct*100,
		formatBytesAs(current, p.total), FormatBytes(p.total),
		speed, eta,
	)
}

func (p *Bar) Done() {
	elapsed := time.Since(p.start)
	speed := float64(p.total) / elapsed.Seconds()
	bar := strings.Repeat("█", p.width)
	fmt.Printf("\r  [%s] 100.0%%  %s / %s  %s  (%s)\x1b[K\n",
		bar,
		FormatBytes(p.total), FormatBytes(p.total),
		FormatBytes(int64(speed))+"/s",
		formatDuration(elapsed),
	)
}

// FormatBytes formats n using binary-scale prefixes (K=1024).
func FormatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

// formatBytesAs formats current using the same unit tier as total so both
// values always display in the same denomination.
func formatBytesAs(current, total int64) string {
	const unit = 1024
	if total < unit {
		return fmt.Sprintf("%d B", current)
	}
	div, exp := int64(unit), 0
	for v := total / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(current)/float64(div), "KMGTPE"[exp])
}

func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh%02dm%02ds", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%dm%02ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}
