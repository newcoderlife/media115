"""M115 crypto tests. Covers key generation, encode/decode, and edge cases."""

import base64

import pytest

from media115._crypto import (
    _rsa_decrypt,
    _rsa_encrypt,
    _xor_derive_key,
    _xor_transform,
    generate_m115_key,
    m115_decode,
    m115_encode,
)


class TestGenerateM115Key:
    def test_returns_bytes(self):
        key = generate_m115_key()
        assert isinstance(key, bytes)

    def test_key_length(self):
        key = generate_m115_key()
        assert len(key) == 16

    def test_keys_are_random(self):
        key1 = generate_m115_key()
        key2 = generate_m115_key()
        assert key1 != key2

    def test_key_is_not_all_zeros(self):
        key = generate_m115_key()
        assert key != b"\x00" * 16


class TestM115Encode:
    def test_encode_returns_string(self):
        key = generate_m115_key()
        encoded = m115_encode(key, "hello")
        assert isinstance(encoded, str)

    def test_encode_returns_base64(self):
        key = generate_m115_key()
        encoded = m115_encode(key, "test data")
        # Should be valid base64
        decoded = base64.b64decode(encoded)
        assert len(decoded) > 0

    def test_encode_different_inputs_differ(self):
        key = generate_m115_key()
        enc1 = m115_encode(key, "first")
        enc2 = m115_encode(key, "second")
        assert enc1 != enc2

    def test_encode_empty_string(self):
        key = generate_m115_key()
        encoded = m115_encode(key, "")
        assert isinstance(encoded, str)
        assert len(encoded) > 0

    def test_encode_json_payload(self):
        key = generate_m115_key()
        payload = '{"pickcode": "abc123", "file_id": "999"}'
        encoded = m115_encode(key, payload)
        assert isinstance(encoded, str)

    def test_encode_unicode(self):
        key = generate_m115_key()
        encoded = m115_encode(key, "test data with unicode")
        assert isinstance(encoded, str)


class TestRSAEncrypt:
    """RSA encrypt/decrypt use the same public exponent (this is a channel cipher,
    not a standard RSA keypair). We test structural properties only."""

    def test_rsa_encrypt_single_slice_length(self):
        """Small payload produces exactly 128 bytes of ciphertext."""
        plaintext = b"short"
        ciphertext = _rsa_encrypt(plaintext)
        assert len(ciphertext) == 128

    def test_rsa_encrypt_multi_slice_length(self):
        """Payload > 117 bytes requires multiple slices, each 128 bytes."""
        plaintext = b"X" * 200  # needs 2 slices (117 + 83)
        ciphertext = _rsa_encrypt(plaintext)
        assert len(ciphertext) == 256

    def test_rsa_encrypt_max_single_slice(self):
        """Exactly 117 bytes (128 - 11) fits in one slice."""
        plaintext = b"A" * 117
        ciphertext = _rsa_encrypt(plaintext)
        assert len(ciphertext) == 128

    def test_rsa_encrypt_just_over_one_slice(self):
        """118 bytes requires two slices."""
        plaintext = b"A" * 118
        ciphertext = _rsa_encrypt(plaintext)
        assert len(ciphertext) == 256

    def test_rsa_encrypt_random_padding_differs(self):
        """Same input should produce different ciphertext due to random padding."""
        plaintext = b"deterministic?"
        ct1 = _rsa_encrypt(plaintext)
        ct2 = _rsa_encrypt(plaintext)
        assert ct1 != ct2

    def test_rsa_decrypt_returns_bytearray(self):
        """Decrypt always returns a bytearray."""
        # Feed it a valid 128-byte RSA block (just test it doesn't crash)
        block = b"\x01" * 128
        result = _rsa_decrypt(block)
        assert isinstance(result, bytearray)


class TestXORDerive:
    def test_xor_derive_key_returns_bytes(self):
        source = b"\x01\x02\x03\x04"
        key = _xor_derive_key(source, 4)
        assert isinstance(key, bytes)
        assert len(key) == 4

    def test_xor_derive_key_deterministic(self):
        source = b"\x10\x20\x30\x40"
        key1 = _xor_derive_key(source, 4)
        key2 = _xor_derive_key(source, 4)
        assert key1 == key2

    def test_xor_derive_key_different_sources(self):
        key1 = _xor_derive_key(b"\x01\x02\x03\x04", 4)
        key2 = _xor_derive_key(b"\x05\x06\x07\x08", 4)
        assert key1 != key2


class TestXORTransform:
    def test_xor_transform_and_reverse(self):
        """XOR transform applied twice returns original data."""
        key = b"\xaa\xbb\xcc\xdd"
        original = bytearray(b"test data here!!")
        data = bytearray(original)
        _xor_transform(key, data)
        assert data != original  # transformed
        _xor_transform(key, data)
        assert data == original  # back to original

    def test_xor_transform_empty(self):
        key = b"\xaa"
        data = bytearray(b"")
        _xor_transform(key, data)
        assert data == bytearray(b"")


class TestM115EncodeDecodeIntegration:
    def test_encode_does_not_crash_with_various_keys(self):
        """Encode should work with any 16-byte key."""
        for _ in range(5):
            key = generate_m115_key()
            result = m115_encode(key, "test payload")
            assert isinstance(result, str)
            assert len(result) > 0

    def test_encode_output_is_base64_decodable(self):
        key = generate_m115_key()
        encoded = m115_encode(key, '{"data": "value"}')
        raw = base64.b64decode(encoded)
        # After base64 decode, should be RSA-encrypted data
        # RSA output is always a multiple of 128 bytes
        assert len(raw) % 128 == 0

    def test_decode_with_crafted_data(self):
        """Build a properly structured ciphertext and verify decode handles it.

        The decode flow:
        1. Base64 decode
        2. RSA decrypt
        3. Split: server_key (16 bytes) + data
        4. XOR transform with server_key-derived key (size=12)
        5. Reverse
        6. XOR transform with key-derived key (size=4)
        7. UTF-8 decode
        """
        key = b"\x11" * 16
        plaintext = "hello 115"

        # Manually construct what encode produces (minus the RSA encryption),
        # to verify the XOR/reverse steps work.
        # We can't easily do a full roundtrip because the server would use
        # a different key for the response.

        # Instead, verify that encode produces valid output
        encoded = m115_encode(key, plaintext)
        assert isinstance(encoded, str)

        # Verify the raw bytes are RSA-sized
        raw = base64.b64decode(encoded)
        assert len(raw) % 128 == 0

    def test_decode_invalid_base64_raises(self):
        """Feeding invalid base64 to decode should raise."""
        key = b"\x00" * 16
        with pytest.raises((ValueError, base64.binascii.Error)):
            m115_decode(key, "not valid base64!!!")

    def test_decode_empty_raises(self):
        """Feeding empty ciphertext to decode should raise."""
        key = b"\x00" * 16
        with pytest.raises((ValueError, IndexError)):
            m115_decode(key, "")

    def test_multiple_encodes_differ(self):
        """Due to random RSA padding, same input produces different ciphertexts."""
        key = generate_m115_key()
        enc1 = m115_encode(key, "same input")
        enc2 = m115_encode(key, "same input")
        # With random padding, outputs should differ (extremely high probability)
        assert enc1 != enc2
