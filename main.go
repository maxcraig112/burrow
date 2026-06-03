package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/maxcraig112/burrow/internal/client"
	"github.com/maxcraig112/burrow/internal/progress"

	"github.com/joho/godotenv"
)

var version = "dev"

func init() {
	// Priority (lowest to highest): user config < local .env < actual env var.
	cfg := make(map[string]string)
	if p, err := userConfigPath(); err == nil {
		if m, err := godotenv.Read(p); err == nil {
			for k, v := range m {
				cfg[k] = v
			}
		}
	}
	if m, err := godotenv.Read(".env"); err == nil {
		for k, v := range m {
			cfg[k] = v
		}
	}
	get := func(key string) string {
		if v := os.Getenv(key); v != "" {
			return v
		}
		return cfg[key]
	}
	if addr := get("EXCHANGE_ADDR"); addr != "" {
		client.ExchangeAddr = addr
	}
	if addr := get("RELAY_ADDR"); addr != "" {
		client.RelayAddr = addr
	}
	if addr := get("TUNNEL_ADDR"); addr != "" {
		client.TunnelAddr = addr
	}
}

func userConfigPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "burrow", "config"), nil
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "send":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: burrow send <file|dir>")
			os.Exit(1)
		}
		requireConfig()
		cmdSend(os.Args[2])
	case "receive":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: burrow receive <code>")
			os.Exit(1)
		}
		requireConfig()
		cmdReceive(os.Args[2])
	case "receive-web":
		requireConfig()
		cmdReceiveWeb(os.Args[2:])
	case "version", "--version", "-v":
		fmt.Println(version)
		return
	case "config":
		cmdConfig(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", os.Args[1])
		usage()
		os.Exit(1)
	}
}

func cmdSend(path string) {
	info, err := os.Stat(path)
	if err != nil {
		fatalf("file error: %v", err)
	}

	fmt.Println("Connecting to exchange server...")
	sess, err := client.StartSend()
	if err != nil {
		fatalf("exchange error: %v", err)
	}

	fmt.Printf("\nCode: %s\n", sess.Nameplate)
	fmt.Printf("On the other machine run:\n\n    burrow receive %s\n\n", sess.Nameplate)
	fmt.Println("Waiting for receiver...")

	keys, err := sess.WaitForReceiver()
	if err != nil {
		fatalf("exchange error: %v", err)
	}

	if info.IsDir() {
		var totalSize int64
		filepath.Walk(path, func(_ string, fi os.FileInfo, walkErr error) error { //nolint:errcheck
			if walkErr == nil && !fi.IsDir() {
				totalSize += fi.Size()
			}
			return nil
		})
		fmt.Printf("Receiver connected. Sending directory %s (%s)...\n",
			filepath.Base(path), progress.FormatBytes(totalSize))
		bar := progress.NewBar(totalSize)
		err = client.SendDir(keys, path,
			func(relPath string, size int64) {
				fmt.Printf("\n  → %s (%s)", relPath, progress.FormatBytes(size))
			},
			func(sent, _ int64) { bar.Print(sent) },
		)
		if err != nil {
			fmt.Println()
			fatalf("send error: %v", err)
		}
		bar.Done()
	} else {
		fmt.Printf("Receiver connected. Sending %s (%s)...\n",
			filepath.Base(path), progress.FormatBytes(info.Size()))
		bar := progress.NewBar(info.Size())
		err = client.SendFile(keys, path, func(sent, _ int64) {
			bar.Print(sent)
		})
		if err != nil {
			fmt.Println()
			fatalf("send error: %v", err)
		}
		bar.Done()
	}
	fmt.Println("Done!")
}

func cmdReceive(code string) {
	fmt.Println("Connecting to exchange server...")

	keys, err := client.Receive(code)
	if err != nil {
		fatalf("exchange error: %v", err)
	}

	fmt.Println("Connecting to relay...")

	t, err := client.ReceiveTransfer(keys)
	if err != nil {
		fatalf("relay error: %v", err)
	}

	if t.IsDir {
		fmt.Printf("Receiving directory %s (%d files, %s)...\n",
			t.Name, t.Count, progress.FormatBytes(t.Size))
	} else {
		fmt.Printf("Receiving %s (%s)...\n", t.Name, progress.FormatBytes(t.Size))
	}

	var onFile func(string, int64)
	if t.IsDir {
		onFile = func(relPath string, size int64) {
			fmt.Printf("\n  ← %s (%s)", relPath, progress.FormatBytes(size))
		}
	}

	destDir, _ := os.Getwd()
	bar := progress.NewBar(t.Size)
	savedPath, err := t.Save(destDir, onFile, func(received, _ int64) {
		bar.Print(received)
	})
	if err != nil {
		fmt.Println()
		fatalf("receive error: %v", err)
	}
	bar.Done()
	fmt.Printf("Saved to %s\n", savedPath)
	fmt.Println("Done!")
}

func requireConfig() {
	if client.ExchangeAddr == "" || client.RelayAddr == "" {
		fmt.Fprintln(os.Stderr, "No server configured. Run:")
		fmt.Fprintln(os.Stderr, "  burrow config <exchange-addr> <relay-addr>")
		fmt.Fprintln(os.Stderr, "Example:")
		fmt.Fprintln(os.Stderr, "  burrow config http://100.x.x.x:8080 100.x.x.x:9090")
		os.Exit(1)
	}
}

func cmdConfig(args []string) {
	if len(args) == 0 {
		p, err := userConfigPath()
		if err != nil {
			fatalf("could not locate config dir: %v", err)
		}
		fmt.Printf("Exchange : %s\n", client.ExchangeAddr)
		fmt.Printf("Relay    : %s\n", client.RelayAddr)
		fmt.Printf("Config   : %s\n", p)
		return
	}
	if len(args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: burrow config <exchange-addr> <relay-addr>")
		fmt.Fprintln(os.Stderr, "example: burrow config http://100.x.x.x:8080 100.x.x.x:9090")
		os.Exit(1)
	}
	p, err := userConfigPath()
	if err != nil {
		fatalf("could not locate config dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		fatalf("could not create config dir: %v", err)
	}
	content := fmt.Sprintf("EXCHANGE_ADDR=%s\nRELAY_ADDR=%s\n", args[0], args[1])
	if err := os.WriteFile(p, []byte(content), 0600); err != nil {
		fatalf("could not write config: %v", err)
	}
	fmt.Printf("Saved config to %s\n", p)
}

func usage() {
	fmt.Printf(`burrow %s - encrypted peer-to-peer file transfer

Usage:
  burrow send <file|dir>           Send a file or directory and display the receive code
  burrow receive <code>            Receive a file using the code from the sender
  burrow receive-web               Host a web upload page and display a QR code
  burrow version                   Print version
  burrow config                    Show current server addresses and config path
  burrow config <exchange> <relay> Save server addresses to user config
`, version)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}
