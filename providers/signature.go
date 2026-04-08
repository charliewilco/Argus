package providers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
)

func ValidateHMACSHA256Hex(secret string, payload []byte, headerValue, prefix string) error {
	if secret == "" {
		return ErrWebhookSecretRequired
	}
	if headerValue == "" {
		return ErrMissingWebhookSignature
	}

	expected := prefix + ComputeHMACSHA256Hex(secret, payload)
	if !hmac.Equal([]byte(headerValue), []byte(expected)) {
		return ErrInvalidWebhookSignature
	}

	return nil
}

func ValidateHMACSHA256Base64(secret string, payload []byte, headerValue, prefix string) error {
	if secret == "" {
		return ErrWebhookSecretRequired
	}
	if headerValue == "" {
		return ErrMissingWebhookSignature
	}

	expected := prefix + ComputeHMACSHA256Base64(secret, payload)
	if !hmac.Equal([]byte(headerValue), []byte(expected)) {
		return ErrInvalidWebhookSignature
	}

	return nil
}

func ComputeHMACSHA256Hex(secret string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

func ComputeHMACSHA256Base64(secret string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}
