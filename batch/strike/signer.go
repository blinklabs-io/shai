// Copyright 2026 Blink Labs Software
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package strike

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	dexstrike "github.com/blinklabs-io/shai/dex/strike"
)

const (
	HeaderWalletPublicKey = "X-API-Wallet-Public-Key"
	HeaderWalletSignature = "X-API-Wallet-Signature"
	HeaderWalletTimestamp = "X-API-Wallet-Timestamp"
	HeaderWalletNonce     = "X-API-Wallet-Nonce"
)

// Ed25519Signer signs Strike authenticated API requests. Header values are
// hex encoded so they match the usual Cardano public-key representation.
type Ed25519Signer struct {
	publicKey  ed25519.PublicKey
	privateKey ed25519.PrivateKey
}

func NewEd25519Signer(
	publicKey ed25519.PublicKey,
	privateKey ed25519.PrivateKey,
) (*Ed25519Signer, error) {
	if len(publicKey) != ed25519.PublicKeySize {
		return nil, fmt.Errorf(
			"%w: public key must be %d bytes",
			dexstrike.ErrInvalidExternalAPIConfig,
			ed25519.PublicKeySize,
		)
	}
	var signingKey ed25519.PrivateKey
	switch len(privateKey) {
	case ed25519.SeedSize:
		signingKey = ed25519.NewKeyFromSeed(privateKey)
	case ed25519.PrivateKeySize:
		signingKey = privateKey
	default:
		return nil, fmt.Errorf(
			"%w: private key must be a %d-byte seed or %d-byte private key",
			dexstrike.ErrInvalidExternalAPIConfig,
			ed25519.SeedSize,
			ed25519.PrivateKeySize,
		)
	}
	if !bytes.Equal(signingKey.Public().(ed25519.PublicKey), publicKey) {
		return nil, fmt.Errorf(
			"%w: private key does not match public key",
			dexstrike.ErrInvalidExternalAPIConfig,
		)
	}
	return &Ed25519Signer{
		publicKey:  append(ed25519.PublicKey(nil), publicKey...),
		privateKey: append(ed25519.PrivateKey(nil), signingKey...),
	}, nil
}

func (s *Ed25519Signer) PublicKeyHex() string {
	return hex.EncodeToString(s.publicKey)
}

func (s *Ed25519Signer) SignPayload(payload string) string {
	signature := ed25519.Sign(s.privateKey, []byte(payload))
	return hex.EncodeToString(signature)
}

func (s *Ed25519Signer) SignRequest(
	req *http.Request,
	body []byte,
	timestamp string,
	nonce string,
) error {
	if s == nil {
		return dexstrike.ErrMissingSigner
	}
	payload := SignaturePayload(
		req.Method,
		CanonicalRequestPath(req.URL),
		timestamp,
		nonce,
		BodyHash(req.Method, body),
	)
	req.Header.Set(HeaderWalletPublicKey, s.PublicKeyHex())
	req.Header.Set(HeaderWalletSignature, s.SignPayload(payload))
	req.Header.Set(HeaderWalletTimestamp, timestamp)
	req.Header.Set(HeaderWalletNonce, nonce)
	return nil
}

func SignaturePayload(
	method string,
	path string,
	timestamp string,
	nonce string,
	bodyHash string,
) string {
	return fmt.Sprintf(
		"%s:%s:%s:%s:%s",
		strings.ToUpper(method),
		path,
		timestamp,
		nonce,
		bodyHash,
	)
}

func BodyHash(method string, body []byte) string {
	if strings.EqualFold(method, http.MethodGet) {
		return ""
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func CanonicalRequestPath(reqURL *url.URL) string {
	path := reqURL.EscapedPath()
	if path == "" {
		path = "/"
	}
	if reqURL.RawQuery != "" {
		path += "?" + reqURL.RawQuery
	}
	return path
}
