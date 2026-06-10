package e2e

import (
	"fmt"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/maxcraig112/burrow/internal/client"
)

// ── Random data helpers ───────────────────────────────────────────────────────

// fastRandContent fills a byte slice using a seeded PRNG. Much faster than
// crypto/rand for large payloads where cryptographic quality is not needed.
func fastRandContent(rng *rand.Rand, n int) []byte {
	b := make([]byte, n)
	for i := 0; i < n-7; i += 8 {
		v := rng.Uint64()
		b[i] = byte(v)
		b[i+1] = byte(v >> 8)
		b[i+2] = byte(v >> 16)
		b[i+3] = byte(v >> 24)
		b[i+4] = byte(v >> 32)
		b[i+5] = byte(v >> 40)
		b[i+6] = byte(v >> 48)
		b[i+7] = byte(v >> 56)
	}
	// Fill any remaining tail bytes.
	if tail := n % 8; tail != 0 {
		v := rng.Uint64()
		for i := n - tail; i < n; i++ {
			b[i] = byte(v)
			v >>= 8
		}
	}
	return b
}

// generateRandFiles creates n fileSpecs with random sizes in [minSize, maxSize].
func generateRandFiles(rng *rand.Rand, n, minSize, maxSize int) []fileSpec {
	files := make([]fileSpec, n)
	spread := maxSize - minSize + 1
	for i := range files {
		size := minSize
		if spread > 1 {
			size += rng.IntN(spread)
		}
		files[i] = fileSpec{
			path:    fmt.Sprintf("f%04d_%05dB.bin", i, size),
			content: fastRandContent(rng, size),
		}
	}
	return files
}

// totalBytes returns the sum of all file content lengths.
func totalBytes(files []fileSpec) int64 {
	var n int64
	for _, f := range files {
		n += int64(len(f.content))
	}
	return n
}

// formatBytes returns a human-readable size string.
func formatBytes(n int) string {
	switch {
	case n >= 1024*1024:
		return fmt.Sprintf("%d MB", n/(1024*1024))
	case n >= 1024:
		return fmt.Sprintf("%d KB", n/1024)
	default:
		return fmt.Sprintf("%d B", n)
	}
}

// ── Throughput tests ──────────────────────────────────────────────────────────

// TestDirTransferThroughput measures end-to-end transfer speed across a range
// of file-count and size combinations. Run with -v to see MB/s figures.
func TestDirTransferThroughput(t *testing.T) {
	tests := []struct {
		name     string
		numFiles int
		minSize  int
		maxSize  int
	}{
		{"100 tiny   (0 B – 1 KB)",     100,  0,        1 * 1024},
		{"200 tiny   (0 B – 1 KB)",     200,  0,        1 * 1024},
		{"500 tiny   (0 B – 1 KB)",     500,  0,        1 * 1024},
		{"200 small  (1 KB – 10 KB)",   200,  1 * 1024, 10 * 1024},
		{"100 medium (10 KB – 100 KB)", 100,  10 * 1024, 100 * 1024},
		{"50  large  (100 KB – 1 MB)",   50,  100 * 1024, 1024 * 1024},
		{"300 mixed  (0 B – 100 KB)",   300,  0,        100 * 1024},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			newHarness(t, "")

			rng := rand.New(rand.NewPCG(12345, 0))
			files := generateRandFiles(rng, tc.numFiles, tc.minSize, tc.maxSize)
			total := totalBytes(files)

			src := makeSourceDir(t, "perf", files)

			start := time.Now()
			doSendReceiveDir(t, src, files)
			elapsed := time.Since(start)

			mbps := float64(total) / elapsed.Seconds() / (1024 * 1024)
			t.Logf("%d files  %.1f KB total  →  %.2f MB/s  (%s)",
				tc.numFiles, float64(total)/1024, mbps, elapsed.Round(time.Millisecond))
		})
	}
}

// TestBufferSizeImpact transfers the same dataset (300 config-sized files,
// 50 B – 5 KB) at seven different IOBufferSize values and prints a comparison
// table. This tells you whether 256 KB is the right default or if more/less
// headroom is needed.
//
// Run with: go test ./e2e/... -v -run TestBufferSizeImpact
func TestBufferSizeImpact(t *testing.T) {
	// Fixed dataset: 300 files, 50 B – 5 KB — representative of game/app configs.
	rng := rand.New(rand.NewPCG(99999, 1))
	files := generateRandFiles(rng, 300, 50, 5*1024)
	total := totalBytes(files)

	bufferSizes := []int{
		4 * 1024,    //   4 KB
		16 * 1024,   //  16 KB
		32 * 1024,   //  32 KB
		64 * 1024,   //  64 KB — one chunk
		128 * 1024,  // 128 KB
		256 * 1024,  // 256 KB ← current default
		512 * 1024,  // 512 KB
		1024 * 1024, //   1 MB
	}

	type result struct {
		label   string
		elapsed time.Duration
		mbps    float64
	}
	results := make([]result, 0, len(bufferSizes))

	orig := client.IOBufferSize
	t.Cleanup(func() { client.IOBufferSize = orig })

	for _, bs := range bufferSizes {
		client.IOBufferSize = bs
		newHarness(t, "")

		src := makeSourceDir(t, fmt.Sprintf("buf%d", bs), files)

		start := time.Now()
		doSendReceiveDir(t, src, files)
		elapsed := time.Since(start)

		mbps := float64(total) / elapsed.Seconds() / (1024 * 1024)
		label := formatBytes(bs)
		if bs == 256*1024 {
			label += " (default)"
		}
		results = append(results, result{label, elapsed, mbps})
	}

	// Print the comparison table.
	t.Logf("\nBuffer size impact — 300 files, %.1f KB total:", float64(total)/1024)
	t.Logf("%-22s  %-10s  %s", "Buffer", "Elapsed", "Throughput")
	t.Logf("%-22s  %-10s  %s", "------", "-------", "----------")
	best := results[0]
	for _, r := range results {
		if r.mbps > best.mbps {
			best = r
		}
	}
	for _, r := range results {
		marker := ""
		if r.label == best.label {
			marker = " ← fastest"
		}
		t.Logf("%-22s  %-10s  %.2f MB/s%s",
			r.label, r.elapsed.Round(time.Millisecond), r.mbps, marker)
	}
}

// ── Benchmarks ────────────────────────────────────────────────────────────────

// BenchmarkSendReceiveDir measures bytes/sec for directory transfers.
// Run with: go test ./e2e/... -bench=BenchmarkSendReceiveDir -benchtime=5x
func BenchmarkSendReceiveDir(b *testing.B) {
	benchmarks := []struct {
		name     string
		numFiles int
		minSize  int
		maxSize  int
	}{
		{"100_tiny",    100, 0,         1 * 1024},
		{"500_tiny",    500, 0,         1 * 1024},
		{"200_small",   200, 1 * 1024,  10 * 1024},
		{"100_medium",  100, 10 * 1024, 100 * 1024},
		{"50_large",     50, 100 * 1024, 1024 * 1024},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			newHarness(b, "")

			// Pre-generate files once; exclude from benchmark timer.
			rng := rand.New(rand.NewPCG(42, 0))
			files := generateRandFiles(rng, bm.numFiles, bm.minSize, bm.maxSize)
			total := totalBytes(files)
			b.SetBytes(total)

			b.ResetTimer()
			for range b.N {
				b.StopTimer()
				src := makeSourceDir(b, "bench", files)
				b.StartTimer()

				doSendReceiveDir(b, src, files)
			}
		})
	}
}
