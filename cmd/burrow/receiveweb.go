package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/maxcraig112/burrow/internal/client"
	"github.com/maxcraig112/burrow/internal/nameplate"
	"github.com/maxcraig112/burrow/internal/qr"
	"github.com/maxcraig112/burrow/internal/tunnel"
	"github.com/maxcraig112/burrow/internal/webupload"
)

func cmdReceiveWeb(args []string) {
	fs := flag.NewFlagSet("receive-web", flag.ExitOnError)
	description := fs.String("description", "", "session description shown in the dashboard")
	fs.StringVar(description, "d", "", "session description (shorthand)")
	fs.Parse(args) //nolint:errcheck

	np := nameplate.Generate()
	destDir, _ := os.Getwd()

	tc := tunnel.NewClient(client.RelayAddr, np, *description, nil)
	handler := webupload.NewHandler(destDir, np, *description, tc.NotifyUploaded)
	tc.SetHandler(handler)

	fmt.Println("Registering tunnel...")
	uploadURL, err := tc.Open()
	if err != nil {
		fatalf("tunnel: %v", err)
	}

	fmt.Printf("\nCode: %s\n", np)
	fmt.Printf("Scan the QR code or open in a browser:\n  %s\n\n", uploadURL)
	qr.Print(os.Stdout, uploadURL)

	// Derive and print the dashboard URL (http://host:port/active).
	if idx := strings.Index(uploadURL, "/t/"); idx >= 0 {
		fmt.Printf("\nDashboard : %s/active\n", uploadURL[:idx])
	}

	fmt.Println("\nWaiting for uploads — press Ctrl+C to stop.\n")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go tc.Serve(ctx)
	<-ctx.Done()

	tc.Close()
	fmt.Println("\nDone.")
}
