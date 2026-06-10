package e2e

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/maxcraig112/burrow/internal/client"
)

// TestLateReceiver verifies that a sender waits for a receiver that connects
// after a short delay. This exercises the exchange session TTL window.
func TestLateReceiver(t *testing.T) {
	tests := []struct {
		name  string
		delay time.Duration
	}{
		{"no delay", 0},
		{"50ms delay", 50 * time.Millisecond},
		{"200ms delay", 200 * time.Millisecond},
		{"500ms delay", 500 * time.Millisecond},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			newHarness(t, "")

			content := randContent(1024)
			src := makeSourceFile(t, "payload.bin", content)
			destDir := t.TempDir()

			sess, err := client.StartSend()
			if err != nil {
				t.Fatalf("StartSend: %v", err)
			}

			sendErr := make(chan error, 1)
			go func() {
				keys, err := sess.WaitForReceiver()
				if err != nil {
					sendErr <- fmt.Errorf("WaitForReceiver: %w", err)
					return
				}
				sendErr <- client.SendFile(keys, src, nil)
			}()

			// Simulate a receiver that takes a moment to run the receive command.
			if tc.delay > 0 {
				time.Sleep(tc.delay)
			}

			recvKeys, err := client.Receive(sess.Nameplate)
			if err != nil {
				t.Fatalf("Receive (after %s delay): %v", tc.delay, err)
			}
			transfer, err := client.ReceiveTransfer(recvKeys)
			if err != nil {
				t.Fatalf("ReceiveTransfer: %v", err)
			}
			path, err := transfer.Save(destDir, nil, nil)
			if err != nil {
				t.Fatalf("Save: %v", err)
			}
			if err := <-sendErr; err != nil {
				t.Fatalf("sender: %v", err)
			}

			assertFileEqual(t, path, content)
		})
	}
}

// TestConcurrentTransfers verifies that multiple independent send+receive pairs
// running concurrently on the same relay do not interfere with each other.
// Each pair gets its own exchange session with a unique relay token.
func TestConcurrentTransfers(t *testing.T) {
	tests := []struct {
		name        string
		concurrency int
		fileSize    int
	}{
		{"2 concurrent files", 2, 32 * 1024},
		{"5 concurrent files", 5, 16 * 1024},
		{"10 concurrent small files", 10, 512},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// One shared harness — all transfers use the same exchange and relay.
			newHarness(t, "") // no fixed nameplate: each StartSend gets a unique one

			type result struct {
				idx int
				err error
			}
			results := make(chan result, tc.concurrency)

			var wg sync.WaitGroup
			wg.Add(tc.concurrency)
			for i := range tc.concurrency {
				go func(idx int) {
					defer wg.Done()

					content := randContent(tc.fileSize)
					src := makeSourceFile(t, fmt.Sprintf("file%d.bin", idx), content)
					destDir := t.TempDir()

					sess, err := client.StartSend()
					if err != nil {
						results <- result{idx, fmt.Errorf("StartSend[%d]: %w", idx, err)}
						return
					}

					sendErr := make(chan error, 1)
					go func() {
						keys, err := sess.WaitForReceiver()
						if err != nil {
							sendErr <- err
							return
						}
						sendErr <- client.SendFile(keys, src, nil)
					}()

					recvKeys, err := client.Receive(sess.Nameplate)
					if err != nil {
						results <- result{idx, fmt.Errorf("Receive[%d]: %w", idx, err)}
						return
					}
					transfer, err := client.ReceiveTransfer(recvKeys)
					if err != nil {
						results <- result{idx, fmt.Errorf("ReceiveTransfer[%d]: %w", idx, err)}
						return
					}
					path, err := transfer.Save(destDir, nil, nil)
					if err != nil {
						results <- result{idx, fmt.Errorf("Save[%d]: %w", idx, err)}
						return
					}
					if se := <-sendErr; se != nil {
						results <- result{idx, fmt.Errorf("sender[%d]: %w", idx, se)}
						return
					}
					// Verify content inline to avoid data races on t.
					got, err := readFile(t, path)
					if err != nil {
						results <- result{idx, fmt.Errorf("read result[%d]: %w", idx, err)}
						return
					}
					if len(got) != len(content) {
						results <- result{idx, fmt.Errorf("transfer[%d]: got %d bytes, want %d", idx, len(got), len(content))}
						return
					}
					results <- result{idx, nil}
				}(i)
			}

			// Close channel once all goroutines finish.
			go func() {
				wg.Wait()
				close(results)
			}()

			for r := range results {
				if r.err != nil {
					t.Errorf("transfer %d failed: %v", r.idx, r.err)
				}
			}
		})
	}
}

// readFile is a helper that reads a file and returns its content.
// Separate from assertFileEqual to avoid calling t.Fatal from a non-test goroutine.
func readFile(t *testing.T, path string) ([]byte, error) {
	t.Helper()
	return readFileBytes(path)
}
