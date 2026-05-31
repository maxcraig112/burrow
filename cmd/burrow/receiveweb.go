package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/maxcraig112/burrow/internal/client"
	"github.com/maxcraig112/burrow/internal/nameplate"
	"github.com/maxcraig112/burrow/internal/qr"
	"github.com/maxcraig112/burrow/internal/tunnel"
	"github.com/maxcraig112/burrow/internal/webupload"
)

func cmdReceiveWeb() {
	np := nameplate.Generate()
	destDir, _ := os.Getwd()

	handler := webupload.NewHandler(destDir, np)
	tc := tunnel.NewClient(client.RelayAddr, np, handler)

	fmt.Println("Registering tunnel...")
	url, err := tc.Open()
	if err != nil {
		fatalf("tunnel: %v", err)
	}

	fmt.Printf("\nCode: %s\n", np)
	fmt.Printf("Scan the QR code or open in a browser:\n  %s\n\n", url)
	qr.Print(os.Stdout, url)
	fmt.Println("\nWaiting for uploads — press Ctrl+C to stop.\n")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go tc.Serve(ctx)
	<-ctx.Done()

	tc.Close()
	fmt.Println("\nDone.")
}
