package main

import (
	"fmt"
	"os"
	"path/filepath"

	"gopherhole/internal/client"
	"gopherhole/internal/progress"

	"github.com/joho/godotenv"
)

func init() {
	godotenv.Load(".env") // missing file is not an error
	if addr := os.Getenv("EXCHANGE_ADDR"); addr != "" {
		client.ExchangeAddr = addr
	}
	if addr := os.Getenv("RELAY_ADDR"); addr != "" {
		client.RelayAddr = addr
	}
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "send":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: wormhole send <file>")
			os.Exit(1)
		}
		cmdSend(os.Args[2])
	case "receive":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: wormhole receive <code>")
			os.Exit(1)
		}
		cmdReceive(os.Args[2])
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", os.Args[1])
		usage()
		os.Exit(1)
	}
}

func cmdSend(filePath string) {
	info, err := os.Stat(filePath)
	if err != nil {
		fatalf("file error: %v", err)
	}
	if info.IsDir() {
		fatalf("%s is a directory — only files are supported", filePath)
	}

	fmt.Println("Connecting to exchange server...")
	sess, err := client.StartSend()
	if err != nil {
		fatalf("exchange error: %v", err)
	}

	fmt.Printf("\nCode: %s\n", sess.Nameplate)
	fmt.Printf("On the other machine run:\n\n    wormhole receive %s\n\n", sess.Nameplate)
	fmt.Println("Waiting for receiver...")

	keys, err := sess.WaitForReceiver()
	if err != nil {
		fatalf("exchange error: %v", err)
	}

	fmt.Printf("Receiver connected. Sending %s (%s)...\n",
		filepath.Base(filePath), progress.FormatBytes(info.Size()))

	bar := progress.NewBar(info.Size())
	err = client.SendFile(keys, filePath, func(sent, _ int64) {
		bar.Print(sent)
	})
	if err != nil {
		fmt.Println()
		fatalf("send error: %v", err)
	}
	bar.Done()
	fmt.Println("Done!")
}

func cmdReceive(code string) {
	fmt.Println("Connecting to exchange server...")

	keys, err := client.Receive(code)
	if err != nil {
		fatalf("exchange error: %v", err)
	}

	fmt.Println("Connecting to relay...")

	incoming, err := client.ReceiveHeader(keys)
	if err != nil {
		fatalf("relay error: %v", err)
	}

	fmt.Printf("Receiving %s (%s)...\n", incoming.Name, progress.FormatBytes(incoming.Size))

	destDir, _ := os.Getwd()
	bar := progress.NewBar(incoming.Size)
	savedPath, err := incoming.Save(destDir, func(received, _ int64) {
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

func usage() {
	fmt.Println(`wormhole - encrypted peer-to-peer file transfer

Usage:
  wormhole send <file>      Send a file and display the receive code
  wormhole receive <code>   Receive a file using the code from the sender`)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}
