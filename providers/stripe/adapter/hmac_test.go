package adapter

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// makeStripeSig builds a Stripe-Signature header value for the given
// payload + secret at timestamp ts. Used by every HMAC test and by the
// webhook server tests in webhook_server_test.go.
func makeStripeSig(payload []byte, secret []byte, ts int64) string {
	signed := []byte(fmt.Sprintf("%d.%s", ts, string(payload)))
	mac := hmac.New(sha256.New, secret)
	mac.Write(signed)
	return fmt.Sprintf("t=%d,v1=%s", ts, hex.EncodeToString(mac.Sum(nil)))
}

func TestVerifyStripe_KnownGoodSignature(t *testing.T) {
	payload := []byte(`{"id":"evt_1","type":"payment_intent.succeeded"}`)
	secret := []byte("whsec_test_001")
	ts := time.Now().Unix()
	header := makeStripeSig(payload, secret, ts)

	gotTS, err := VerifySignature(payload, header, secret, 300)
	require.NoError(t, err)
	require.Equal(t, ts, gotTS)
}

func TestVerifyStripe_TamperedBodyRejected(t *testing.T) {
	payload := []byte(`{"id":"evt_1","type":"payment_intent.succeeded"}`)
	secret := []byte("whsec_test_002")
	ts := time.Now().Unix()
	header := makeStripeSig(payload, secret, ts)

	tampered := append([]byte(nil), payload...)
	tampered[1] = 'X' // flip one byte

	_, err := VerifySignature(tampered, header, secret, 300)
	require.ErrorIs(t, err, ErrSignatureMismatch)
}

func TestVerifyStripe_ExpiredTimestamp(t *testing.T) {
	payload := []byte(`{"id":"evt_2"}`)
	secret := []byte("whsec_test_003")
	ts := time.Now().Unix() - 3600 // 1 h in the past
	header := makeStripeSig(payload, secret, ts)

	_, err := VerifySignature(payload, header, secret, 300)
	require.ErrorIs(t, err, ErrSignatureExpired)
}

func TestVerifyStripe_FutureTimestamp(t *testing.T) {
	payload := []byte(`{"id":"evt_3"}`)
	secret := []byte("whsec_test_004")
	ts := time.Now().Unix() + 3600 // 1 h in the future
	header := makeStripeSig(payload, secret, ts)

	_, err := VerifySignature(payload, header, secret, 300)
	require.ErrorIs(t, err, ErrSignatureExpired)
}

func TestVerifyStripe_MissingT(t *testing.T) {
	_, err := VerifySignature([]byte("body"), "v1=deadbeef", []byte("s"), 300)
	require.ErrorIs(t, err, ErrSignatureMissingT)
}

func TestVerifyStripe_MissingV1(t *testing.T) {
	_, err := VerifySignature([]byte("body"), "t=1700000000", []byte("s"), 300)
	require.ErrorIs(t, err, ErrSignatureMissingV1)
}

func TestVerifyStripe_ToleranceZero(t *testing.T) {
	payload := []byte(`{"id":"evt_4"}`)
	secret := []byte("whsec_test_005")
	ts := time.Now().Unix() - 1 // 1s old
	header := makeStripeSig(payload, secret, ts)

	_, err := VerifySignature(payload, header, secret, 0)
	require.ErrorIs(t, err, ErrSignatureExpired)
}
