package adapter

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Errors returned by VerifySignature. Tests assert on these
// concretely to defeat any silent-pass refactor.
var (
	ErrSignatureMissingT  = errors.New("stripe sig: missing t= component")
	ErrSignatureMissingV1 = errors.New("stripe sig: missing v1= component")
	ErrSignatureExpired   = errors.New("stripe sig: timestamp beyond tolerance window")
	ErrSignatureMismatch  = errors.New("stripe sig: v1 HMAC mismatch")
	ErrInvalidTimestamp   = errors.New("stripe sig: invalid t= integer")
)

// VerifySignature implements the Stripe-Signature header verification
// algorithm:
//
//	signed_payload = fmt.Sprintf("%d.%s", ts, raw_body)
//	mac            = HMAC_SHA256(secret, signed_payload)
//	expected_v1    = hex(mac)
//
// header is the raw "Stripe-Signature" value, e.g.
// "t=1700000000,v1=abc,v0=...".
// secret is the per-instance endpoint secret (whsec_*).
// toleranceSecs is the half-width of the timestamp window; values <= 0
// strictly reject anything but the exact timestamp (used by the
// zero-tolerance test).
//
// Returns the parsed timestamp on success so the caller can record it
// in the dedup map / RTA envelope.
func VerifySignature(payload []byte, header string, secret []byte, toleranceSecs int64) (int64, error) {
	var ts int64
	var tsSet bool
	v1Hexes := make([]string, 0, 1)

	for _, part := range strings.Split(header, ",") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "t":
			n, err := strconv.ParseInt(kv[1], 10, 64)
			if err != nil {
				return 0, fmt.Errorf("%w: %s", ErrInvalidTimestamp, kv[1])
			}
			ts = n
			tsSet = true
		case "v1":
			v1Hexes = append(v1Hexes, kv[1])
		}
	}
	if !tsSet {
		return 0, ErrSignatureMissingT
	}
	if len(v1Hexes) == 0 {
		return 0, ErrSignatureMissingV1
	}

	now := time.Now().Unix()
	delta := now - ts
	if delta < 0 {
		delta = -delta
	}
	if delta > toleranceSecs {
		return 0, fmt.Errorf("%w: |now-ts|=%d > tol=%d", ErrSignatureExpired, delta, toleranceSecs)
	}

	signedPayload := []byte(fmt.Sprintf("%d.%s", ts, string(payload)))
	mac := hmac.New(sha256.New, secret)
	mac.Write(signedPayload)
	expected := mac.Sum(nil)

	for _, v1Hex := range v1Hexes {
		got, err := hex.DecodeString(v1Hex)
		if err != nil {
			continue
		}
		if subtle.ConstantTimeCompare(expected, got) == 1 {
			return ts, nil
		}
	}
	return 0, ErrSignatureMismatch
}
