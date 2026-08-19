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
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"testing"
)

func TestSignRequestGETHashesEmptyBody(t *testing.T) {
	privateKey := ed25519.NewKeyFromSeed(
		[]byte("01234567890123456789012345678901"),
	)
	publicKey := privateKey.Public().(ed25519.PublicKey)
	signer, err := NewEd25519Signer(publicKey, privateKey)
	if err != nil {
		t.Fatalf("NewEd25519Signer returned error: %v", err)
	}
	req, err := http.NewRequest(
		http.MethodGet,
		"https://api.strikefinance.org/v2/orders?status=open",
		nil,
	)
	if err != nil {
		t.Fatalf("http.NewRequest returned error: %v", err)
	}

	const timestamp = "1700000000"
	const nonce = "nonce-1"
	if err := signer.SignRequest(req, nil, timestamp, nonce); err != nil {
		t.Fatalf("SignRequest returned error: %v", err)
	}

	emptyBodyHash := sha256.Sum256(nil)
	payload := SignaturePayload(
		http.MethodGet,
		"/v2/orders?status=open",
		timestamp,
		nonce,
		hex.EncodeToString(emptyBodyHash[:]),
	)
	want := hex.EncodeToString(ed25519.Sign(privateKey, []byte(payload)))
	if got := req.Header.Get(HeaderWalletSignature); got != want {
		t.Fatalf("signature = %q, want %q", got, want)
	}
}
