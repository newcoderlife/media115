package cloud115

import (
	"bytes"
	"encoding/base64"
	"testing"
)

// TestGenerateM115Key verifies that GenerateM115Key returns 16 bytes and that
// two successive calls produce different values (birthday probability ~2^-128).
func TestGenerateM115Key(t *testing.T) {
	k1 := GenerateM115Key()
	k2 := GenerateM115Key()
	if len(k1) != 16 {
		t.Fatalf("expected 16 bytes, got %d", len(k1))
	}
	if bytes.Equal(k1, k2) {
		t.Fatal("two generated keys should not be equal")
	}
}

// TestM115EncodeDecodeRoundtrip tests the internal symmetry of M115Encode.
//
// M115Encode (client→server) and M115Decode (server→client) use different XOR
// keys and are not inverses of each other; a true roundtrip requires the 115
// server.  This test instead verifies the individual sub-operations:
//   - xorDeriveKey is deterministic
//   - xorTransform(key, xorTransform(key, data)) == data  (self-inverse)
//   - M115Encode produces valid base64
func TestM115EncodeDecodeRoundtrip(t *testing.T) {
	key := GenerateM115Key()
	plaintext := "hello, 115 world! 你好世界"

	// Verify that our xorTransform is self-inverse (encode then undo manually).
	data := []byte(plaintext)
	derivedKey := xorDeriveKey(key, 4)
	xorTransform(derivedKey, data)
	// Reverse.
	for i, j := 0, len(data)-1; i < j; i, j = i+1, j-1 {
		data[i], data[j] = data[j], data[i]
	}
	xorTransform(xorClientKey, data)
	// Undo: xorTransform(clientKey) is self-inverse.
	xorTransform(xorClientKey, data)
	// Undo reverse.
	for i, j := 0, len(data)-1; i < j; i, j = i+1, j-1 {
		data[i], data[j] = data[j], data[i]
	}
	// Undo xorTransform(derivedKey).
	xorTransform(derivedKey, data)
	if string(data) != plaintext {
		t.Fatalf("manual roundtrip failed: got %q, want %q", string(data), plaintext)
	}

	// Also confirm M115Encode produces valid base64.
	encoded := M115Encode(key, plaintext)
	if _, err := base64.StdEncoding.DecodeString(encoded); err != nil {
		t.Fatalf("M115Encode output is not valid base64: %v", err)
	}
}

// TestM115EncodeIsBase64 verifies that M115Encode output is valid standard base64.
func TestM115EncodeIsBase64(t *testing.T) {
	key := GenerateM115Key()
	encoded := M115Encode(key, "test data")
	if _, err := base64.StdEncoding.DecodeString(encoded); err != nil {
		t.Fatalf("output is not valid base64: %v", err)
	}
}

// TestRsaEncryptSliceOutputLength verifies that rsaEncryptSlice always returns
// exactly 128 bytes regardless of input length.
func TestRsaEncryptSliceOutputLength(t *testing.T) {
	for _, inputLen := range []int{1, 16, 64, 117} {
		input := make([]byte, inputLen)
		out := rsaEncryptSlice(input)
		if len(out) != rsaKeyLen {
			t.Errorf("inputLen=%d: expected %d bytes, got %d", inputLen, rsaKeyLen, len(out))
		}
	}
}

// TestXorDeriveKeyDeterministic verifies that xorDeriveKey is deterministic:
// same source and size always produce the same key.
func TestXorDeriveKeyDeterministic(t *testing.T) {
	source := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10}
	k1 := xorDeriveKey(source, 4)
	k2 := xorDeriveKey(source, 4)
	if !bytes.Equal(k1, k2) {
		t.Fatal("xorDeriveKey is not deterministic")
	}
	// Different size should produce different result.
	k3 := xorDeriveKey(source, 12)
	if len(k3) != 12 {
		t.Fatalf("expected 12-byte key, got %d", len(k3))
	}
}

// TestXorTransformReversible verifies that applying xorTransform twice restores
// the original data.
func TestXorTransformReversible(t *testing.T) {
	key := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	original := []byte("hello, world! this is a test string 123")
	data := make([]byte, len(original))
	copy(data, original)
	xorTransform(key, data)
	if bytes.Equal(data, original) {
		t.Fatal("xorTransform should change the data")
	}
	xorTransform(key, data)
	if !bytes.Equal(data, original) {
		t.Fatal("double xorTransform should restore original data")
	}
}
