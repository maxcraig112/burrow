package progress

import (
	"fmt"
	"os"
	"time"

	"github.com/schollz/progressbar/v3"
)

// Bar is a byte-oriented terminal progress bar backed by
// github.com/schollz/progressbar/v3.
type Bar struct {
	pb *progressbar.ProgressBar
}

// NewBar returns a progress bar sized for a transfer of total bytes.
func NewBar(total int64) *Bar {
	max := total
	if max < 1 {
		max = 1 // avoid a divide-by-zero render for zero-length transfers
	}
	pb := progressbar.NewOptions64(max,
		progressbar.OptionSetWriter(os.Stdout),
		progressbar.OptionSetDescription(" "),
		progressbar.OptionShowBytes(true),
		progressbar.OptionShowCount(),
		progressbar.OptionSetPredictTime(true),
		progressbar.OptionFullWidth(),
		progressbar.OptionThrottle(100*time.Millisecond),
		progressbar.OptionSetRenderBlankState(true),
		progressbar.OptionSetTheme(progressbar.Theme{
			Saucer:        "█",
			SaucerPadding: "░",
			BarStart:      "[",
			BarEnd:        "]",
		}),
		progressbar.OptionOnCompletion(func() { fmt.Fprintln(os.Stdout) }),
	)
	return &Bar{pb: pb}
}

// Print sets the bar to an absolute byte count.
func (b *Bar) Print(current int64) { _ = b.pb.Set64(current) }

// Done completes the bar at 100%.
func (b *Bar) Done() { _ = b.pb.Finish() }

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
