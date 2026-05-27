package pake

import (
	"crypto/ecdh"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"

	"golang.org/x/crypto/hkdf"
)

// KeyPair is an ephemeral X25519 key pair generated for a single exchange.
type KeyPair struct {
	Private *ecdh.PrivateKey
}

// DerivedKeys holds the symmetric material produced after a successful ECDH
// exchange. Both sender and receiver derive identical values.
type DerivedKeys struct {
	FileKey    []byte // 32-byte AES-256-GCM key for encrypting the file
	RelayToken string // base64url token used to pair the two relay connections
}

// GenerateKeyPair creates a new ephemeral X25519 key pair.
func GenerateKeyPair() (*KeyPair, error) {
	priv, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate key pair: %w", err)
	}
	return &KeyPair{Private: priv}, nil
}

// PublicKeyBase64 returns the public key encoded as standard base64.
func (kp *KeyPair) PublicKeyBase64() string {
	return base64.StdEncoding.EncodeToString(kp.Private.PublicKey().Bytes())
}

// DeriveKeys performs X25519 ECDH with the peer's public key, then derives
// the file encryption key and relay token via HKDF-SHA256 keyed with the
// nameplate as a salt. Both peers derive identical keys from their own private
// key and the other's public key.
func DeriveKeys(priv *ecdh.PrivateKey, peerPubBase64, nameplate string) (*DerivedKeys, error) {
	peerBytes, err := base64.StdEncoding.DecodeString(peerPubBase64)
	if err != nil {
		return nil, fmt.Errorf("decode peer public key: %w", err)
	}
	peerPub, err := ecdh.X25519().NewPublicKey(peerBytes)
	if err != nil {
		return nil, fmt.Errorf("parse peer public key: %w", err)
	}
	sharedSecret, err := priv.ECDH(peerPub)
	if err != nil {
		return nil, fmt.Errorf("ECDH: %w", err)
	}

	derive := func(info string) ([]byte, error) {
		r := hkdf.New(sha256.New, sharedSecret, []byte(nameplate), []byte(info))
		key := make([]byte, 32)
		if _, err := io.ReadFull(r, key); err != nil {
			return nil, err
		}
		return key, nil
	}

	fileKey, err := derive("wormhole-file-key-v1")
	if err != nil {
		return nil, fmt.Errorf("derive file key: %w", err)
	}
	relayTokenBytes, err := derive("wormhole-relay-token-v1")
	if err != nil {
		return nil, fmt.Errorf("derive relay token: %w", err)
	}

	return &DerivedKeys{
		FileKey:    fileKey,
		RelayToken: base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(relayTokenBytes),
	}, nil
}
