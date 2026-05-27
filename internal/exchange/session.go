package exchange

import "sync"

// Session holds the state for a single pending exchange.
// All exported methods are safe for concurrent use.
type Session struct {
	Nameplate    string
	senderPAKE   string       // base64 public key set by the sender
	receiverPAKE string       // base64 public key set atomically with claim
	pakeReady    chan struct{} // closed when senderPAKE is stored
	pakeOnce     sync.Once
	claimed      chan struct{} // closed when a receiver claims this session
	claimOnce    sync.Once
}

func newSession(nameplate string) *Session {
	return &Session{
		Nameplate: nameplate,
		pakeReady: make(chan struct{}),
		claimed:   make(chan struct{}),
	}
}

// SetSenderPAKE stores the sender's base64-encoded public key and signals
// PAKEReady. Safe to call exactly once; subsequent calls are no-ops.
func (s *Session) SetSenderPAKE(pake string) {
	s.pakeOnce.Do(func() {
		s.senderPAKE = pake
		close(s.pakeReady)
	})
}

// SenderPAKE returns the sender's base64-encoded public key.
// Only valid after PAKEReady() is closed.
func (s *Session) SenderPAKE() string { return s.senderPAKE }

// ReceiverPAKE returns the receiver's base64-encoded public key.
// Only valid after Claimed() is closed.
func (s *Session) ReceiverPAKE() string { return s.receiverPAKE }

// PAKEReady returns a channel that is closed once the sender has stored their
// public key. Receivers should select on this (with a short timeout) before
// reading SenderPAKE.
func (s *Session) PAKEReady() <-chan struct{} { return s.pakeReady }

// Claimed returns a channel that is closed when a receiver claims this session.
func (s *Session) Claimed() <-chan struct{} { return s.claimed }

// claimWithPAKE stores the receiver's PAKE and closes the claimed channel
// exactly once. Returns true if this call was the first to claim.
func (s *Session) claimWithPAKE(receiverPAKE string) bool {
	first := false
	s.claimOnce.Do(func() {
		s.receiverPAKE = receiverPAKE
		close(s.claimed)
		first = true
	})
	return first
}
