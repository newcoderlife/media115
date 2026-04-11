package cloud115

import (
	"bytes"
	"encoding/base64"
	"testing"
)

// TestNewEC115Cipher verifies that a cipher can be created without error.
func TestNewEC115Cipher(t *testing.T) {
	c, err := NewEC115Cipher()
	if err != nil {
		t.Fatalf("NewEC115Cipher() error: %v", err)
	}
	if c == nil {
		t.Fatal("NewEC115Cipher() returned nil")
	}
}

// TestEC115PubKeyLength verifies that the public key is exactly 30 bytes.
func TestEC115PubKeyLength(t *testing.T) {
	c, err := NewEC115Cipher()
	if err != nil {
		t.Fatalf("NewEC115Cipher() error: %v", err)
	}
	if len(c.pubKey) != 30 {
		t.Fatalf("expected pubKey length 30, got %d", len(c.pubKey))
	}
	if c.pubKey[0] != 0x1d {
		t.Fatalf("expected pubKey[0] == 0x1d, got 0x%02x", c.pubKey[0])
	}
}

// TestEC115EncodeToken verifies that EncodeToken returns a non-empty,
// valid base64 string.
func TestEC115EncodeToken(t *testing.T) {
	c, err := NewEC115Cipher()
	if err != nil {
		t.Fatalf("NewEC115Cipher() error: %v", err)
	}
	token := c.EncodeToken()
	if token == "" {
		t.Fatal("EncodeToken() returned empty string")
	}
	raw, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		t.Fatalf("EncodeToken() output is not valid base64: %v", err)
	}
	// 44 bytes token + 4 bytes CRC = 48 bytes.
	if len(raw) != 48 {
		t.Fatalf("decoded token should be 48 bytes, got %d", len(raw))
	}
}

// TestEC115EncodeTokenDifferent verifies that two separate ciphers produce
// different tokens (due to different key pairs and random XOR bytes).
func TestEC115EncodeTokenDifferent(t *testing.T) {
	c1, err := NewEC115Cipher()
	if err != nil {
		t.Fatalf("NewEC115Cipher() c1 error: %v", err)
	}
	c2, err := NewEC115Cipher()
	if err != nil {
		t.Fatalf("NewEC115Cipher() c2 error: %v", err)
	}
	t1 := c1.EncodeToken()
	t2 := c2.EncodeToken()
	if t1 == t2 {
		t.Fatal("two different ciphers should produce different tokens")
	}
}

// TestEC115Encode verifies that Encode produces output different from input,
// and that the output length is a multiple of 16 (AES block size).
func TestEC115Encode(t *testing.T) {
	c, err := NewEC115Cipher()
	if err != nil {
		t.Fatalf("NewEC115Cipher() error: %v", err)
	}
	input := []byte("some plaintext data for testing EC115 encode path")
	output := c.Encode(input)
	if bytes.Equal(input, output[:len(input)]) {
		t.Fatal("Encode() output should differ from input")
	}
	if len(output)%16 != 0 {
		t.Fatalf("Encode() output length %d is not a multiple of 16", len(output))
	}
}
