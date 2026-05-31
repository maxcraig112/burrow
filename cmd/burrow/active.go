package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/maxcraig112/burrow/internal/client"
)

type sessionInfo struct {
	Nameplate      string    `json:"nameplate"`
	Description    string    `json:"description"`
	URL            string    `json:"url"`
	StartedAt      time.Time `json:"started_at"`
	FilesReceived  int       `json:"files_received"`
	ReceiverIP     string    `json:"receiver_ip"`
	LastUploaderIP string    `json:"last_uploader_ip"`
}

type sessionsResponse struct {
	Sessions []sessionInfo `json:"sessions"`
}

func cmdActive() {
	base := tunnelBaseURL()
	if base == "" {
		fatalf("no server configured — run: burrow config <exchange-addr> <relay-addr>")
	}

	resp, err := http.Get(base + "/api/sessions")
	if err != nil {
		fatalf("could not reach tunnel server at %s: %v", base, err)
	}
	defer resp.Body.Close()

	var result sessionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		fatalf("invalid response from server: %v", err)
	}

	if len(result.Sessions) == 0 {
		fmt.Println("No active sessions.")
		return
	}

	fmt.Printf("Active receive-web sessions (%d):\n", len(result.Sessions))
	for _, s := range result.Sessions {
		fmt.Println()
		if s.Description != "" {
			fmt.Printf("  %s  %q\n", s.Nameplate, s.Description)
		} else {
			fmt.Printf("  %s\n", s.Nameplate)
		}
		fmt.Printf("  Active      %s\n", time.Since(s.StartedAt).Round(time.Second))
		fmt.Printf("  Files       %d\n", s.FilesReceived)
		fmt.Printf("  Receiver    %s\n", s.ReceiverIP)
		if s.LastUploaderIP != "" {
			fmt.Printf("  Last from   %s\n", s.LastUploaderIP)
		}
		fmt.Printf("  URL         %s\n", s.URL)
	}
	fmt.Printf("\nDashboard: %s/active\n", base)
}

// tunnelBaseURL returns the base HTTP URL of the tunnel server.
// Uses TUNNEL_ADDR if configured, otherwise derives it from RELAY_ADDR
// by substituting the default tunnel port (8082).
func tunnelBaseURL() string {
	if client.TunnelAddr != "" {
		return client.TunnelAddr
	}
	if client.RelayAddr == "" {
		return ""
	}
	host, _, err := net.SplitHostPort(client.RelayAddr)
	if err != nil {
		host = client.RelayAddr
	}
	if host == "" {
		host = "localhost"
	}
	return "http://" + host + ":8082"
}
