package exchange

import (
	"time"

	"github.com/maxcraig112/burrow/internal/nameplate"
	cache "github.com/patrickmn/go-cache"
	"github.com/rs/zerolog"
)

// Exchange manages the lifecycle of pending send/receive sessions.
// All methods are safe for concurrent use.
type Exchange struct {
	cache    *cache.Cache
	ttl      time.Duration
	pakeWait time.Duration // how long Receive waits for the sender's PAKE
	logger   zerolog.Logger
	flags    Flags
}

// New creates an Exchange with the given session TTL and runtime flags.
// Expired entries are purged on an interval of 2× TTL.
func New(ttl time.Duration, logger zerolog.Logger, flags Flags) *Exchange {
	return &Exchange{
		cache:    cache.New(ttl, ttl*2),
		ttl:      ttl,
		pakeWait: 10 * time.Second,
		logger:   logger,
		flags:    flags,
	}
}

// Send generates a unique nameplate, stores a new session in the cache, and
// returns it. The caller must call StoreSenderPAKE before any receiver can
// complete the exchange. If a fixed nameplate is configured, it is always used.
func (e *Exchange) Send() (*Session, error) {
	np := e.uniqueNameplate()
	s := newSession(np)
	e.cache.Set(np, s, e.ttl)
	e.logger.Debug().Str("nameplate", np).Dur("ttl", e.ttl).Msg("session created")
	return s, nil
}

// StoreSenderPAKE records the sender's base64-encoded public key against the
// nameplate. This must be called before Receive can complete.
func (e *Exchange) StoreSenderPAKE(nameplate, pake string) error {
	if pake == "" {
		return ErrPAKEInvalid
	}
	val, ok := e.cache.Get(nameplate)
	if !ok {
		return ErrNotFound
	}
	val.(*Session).SetSenderPAKE(pake)
	e.logger.Debug().Str("nameplate", nameplate).Msg("sender PAKE stored")
	return nil
}

// Receive claims the session for nameplate and returns the sender's PAKE so
// the receiver can derive the shared key. It blocks briefly waiting for the
// sender's PAKE if it has not arrived yet.
//
// Returns:
//   - (senderPAKE, nil) on success; the waiting sender is signalled automatically.
//   - ("", ErrNotFound) if the nameplate is absent or expired.
//   - ("", ErrPAKENotReady) if the sender did not send their PAKE in time.
//   - ("", ErrAlreadyClaimed) if a concurrent receiver beat this call.
//   - ("", ErrPAKEInvalid) if receiverPAKE is empty.
func (e *Exchange) Receive(nameplate, receiverPAKE string) (string, error) {
	if receiverPAKE == "" {
		return "", ErrPAKEInvalid
	}
	val, ok := e.cache.Get(nameplate)
	if !ok {
		e.logger.Warn().Str("nameplate", nameplate).Msg("nameplate not found")
		return "", ErrNotFound
	}
	s := val.(*Session)

	// Wait briefly for the sender's PAKE in case of a tight race.
	select {
	case <-s.PAKEReady():
	case <-time.After(e.pakeWait):
		e.logger.Warn().Str("nameplate", nameplate).Msg("timed out waiting for sender PAKE")
		return "", ErrPAKENotReady
	}

	if !s.claimWithPAKE(receiverPAKE) {
		e.logger.Warn().Str("nameplate", nameplate).Msg("nameplate already claimed")
		return "", ErrAlreadyClaimed
	}

	e.cache.Delete(nameplate)
	e.logger.Info().Str("nameplate", nameplate).Msg("nameplate claimed, PAKE exchange complete")
	return s.SenderPAKE(), nil
}

// uniqueNameplate returns the fixed nameplate when configured, otherwise
// generates random nameplates until finding one not in the cache.
func (e *Exchange) uniqueNameplate() string {
	if e.flags.FixedNameplate != "" {
		return e.flags.FixedNameplate
	}
	for {
		np := nameplate.Generate()
		if _, exists := e.cache.Get(np); !exists {
			return np
		}
		e.logger.Debug().Str("nameplate", np).Msg("nameplate collision, retrying")
	}
}
