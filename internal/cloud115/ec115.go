package cloud115

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	mathrand "math/rand"
	"time"

	"github.com/pierrec/lz4/v4"
)

// serverPubKey is the 115 server's P-224 public key (SEC1 uncompressed, 57 bytes).
var serverPubKey = []byte{
	0x04, 0x57, 0xa2, 0x92, 0x57, 0xcd, 0x23, 0x20,
	0xe5, 0xd6, 0xd1, 0x43, 0x32, 0x2f, 0xa4, 0xbb,
	0x8a, 0x3c, 0xf9, 0xd3, 0xcc, 0x62, 0x3e, 0xf5,
	0xed, 0xac, 0x62, 0xb7, 0x67, 0x8a, 0x89, 0xc9,
	0x1a, 0x83, 0xba, 0x80, 0x0d, 0x61, 0x29, 0xf5,
	0x22, 0xd0, 0x34, 0xc8, 0x95, 0xdd, 0x24, 0x65,
	0x24, 0x3a, 0xdd, 0xc2, 0x50, 0x95, 0x3b, 0xee,
	0xba,
}

// crcSalt is the 21-byte salt prepended before CRC32 in EncodeToken.
var crcSalt = []byte("^j>WD3Kr?J2gLFjD4W2y@")

// TokenSalt is exported for callers that need the HMAC-style token salt.
const TokenSalt = "Qclm8MGWUv59TnrR0XPg"

// EC115Cipher holds all state for a single EC115 session.
type EC115Cipher struct {
	pubKey []byte // 30 bytes: 0x1d prefix + compressed SEC1 (29 bytes)
	aesKey []byte // 16 bytes: shared_secret[:16]
	aesIV  []byte // 16 bytes: shared_secret[12:28]
}

// NewEC115Cipher generates a fresh P-224 key pair, performs ECDH with the 115
// server public key, and derives the AES session key and IV.
func NewEC115Cipher() (*EC115Cipher, error) {
	curve := elliptic.P224()

	// Unmarshal server public key (SEC1 uncompressed).
	srvX, srvY := elliptic.Unmarshal(curve, serverPubKey) //nolint:staticcheck // P-224 not in crypto/ecdh

	// Generate client key pair.
	clientPriv, err := ecdsa.GenerateKey(curve, rand.Reader)
	if err != nil {
		return nil, err
	}

	// ECDH: shared point = serverPub * clientPriv.D
	sharedX, _ := curve.ScalarMult(srvX, srvY, clientPriv.D.Bytes()) //nolint:staticcheck // P-224 not in crypto/ecdh

	// Shared secret = sharedX padded to 28 bytes (P-224 field size).
	sharedSecret := make([]byte, 28)
	sharedXBytes := sharedX.Bytes()
	// Right-align into 28-byte buffer (big-endian, as in Python's to_bytes(28, 'big')).
	copy(sharedSecret[28-len(sharedXBytes):], sharedXBytes)

	aesKey := make([]byte, 16)
	copy(aesKey, sharedSecret[:16])
	aesIV := make([]byte, 16)
	copy(aesIV, sharedSecret[12:28])

	// pubKey = 0x1d + MarshalCompressed(clientPub) — total 30 bytes.
	compressed := elliptic.MarshalCompressed(curve, clientPriv.PublicKey.X, clientPriv.PublicKey.Y) //nolint:staticcheck // P-224 not in crypto/ecdh
	pubKey := append([]byte{0x1d}, compressed...)

	return &EC115Cipher{
		pubKey: pubKey,
		aesKey: aesKey,
		aesIV:  aesIV,
	}, nil
}

// EncodeToken builds the k_ec query parameter from the client public key.
//
// Pack layout (little-endian uint32):
//
//	pubKey[:15] | 0x00 | uint32(115) | uint32(timestamp) | pubKey[15:] | 0x00 | uint32(1)
//	= 15 + 1 + 4 + 4 + 15 + 1 + 4 = 44 bytes
//
// The first 24 bytes are XOR'd with random r1; the remaining bytes with r2.
// A CRC32 of (crcSalt + token) is appended as a little-endian uint32.
func (c *EC115Cipher) EncodeToken() string {
	pub := c.pubKey // 30 bytes
	timestamp := uint32(time.Now().Unix())

	token := make([]byte, 44)
	offset := 0

	copy(token[offset:], pub[:15])
	offset += 15
	token[offset] = 0x00
	offset++
	binary.LittleEndian.PutUint32(token[offset:], 115)
	offset += 4
	binary.LittleEndian.PutUint32(token[offset:], timestamp)
	offset += 4
	copy(token[offset:], pub[15:])
	offset += 15
	token[offset] = 0x00
	offset++
	binary.LittleEndian.PutUint32(token[offset:], 1)

	// XOR: first 24 bytes with r1, rest with r2.
	r1 := byte(mathrand.Intn(256)) //nolint:gosec // intentionally weak random for obfuscation
	r2 := byte(mathrand.Intn(256)) //nolint:gosec
	for i := range token {
		if i < 24 {
			token[i] ^= r1
		} else {
			token[i] ^= r2
		}
	}

	// CRC32 checksum over (crcSalt + token).
	checksum := crc32.ChecksumIEEE(append(crcSalt, token...))
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, checksum)
	token = append(token, buf...)

	return base64.StdEncoding.EncodeToString(token)
}

// Encode AES-128-CBC encrypts data.  Input is zero-padded to a 16-byte boundary
// if not already aligned (no padding added when len(data)%16 == 0).
func (c *EC115Cipher) Encode(data []byte) []byte {
	padSize := 16 - len(data)%16
	if padSize != 16 {
		padded := make([]byte, len(data)+padSize)
		copy(padded, data)
		data = padded
	}
	block, err := aes.NewCipher(c.aesKey)
	if err != nil {
		panic(err)
	}
	ct := make([]byte, len(data))
	cipher.NewCBCEncrypter(block, c.aesIV).CryptBlocks(ct, data)
	return ct
}

// Decode AES-128-CBC decrypts then LZ4-block-decompresses the server response.
//
// Format: ciphertext (all but last 12 bytes) | trailer (12 bytes)
// Trailer bytes 0–3 XOR'd with byte 7 give the total uncompressed size (uint32 LE).
// The decrypted plaintext is a sequence of chunks:
//
//	uint16 LE src_size | src_size bytes of lz4 block
//
// Chunks are decompressed in at most 8 KiB steps until dst_size bytes are recovered.
func (c *EC115Cipher) Decode(data []byte) ([]byte, error) {
	if len(data) < 12 {
		return nil, fmt.Errorf("ec115: data too short (%d bytes, need at least 12)", len(data))
	}
	ciphertext := data[:len(data)-12]
	tail := make([]byte, 12)
	copy(tail, data[len(data)-12:])

	block, err := aes.NewCipher(c.aesKey)
	if err != nil {
		return nil, err
	}
	plaintext := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, c.aesIV).CryptBlocks(plaintext, ciphertext)

	// Decode total uncompressed size: XOR bytes 0-3 with byte 7.
	for i := 0; i < 4; i++ {
		tail[i] ^= tail[7]
	}
	dstSize := int(binary.LittleEndian.Uint32(tail[:4]))

	var out []byte
	offset := 0
	for dstSize > 0 {
		uncompressedSize := dstSize
		if uncompressedSize > 8192 {
			uncompressedSize = 8192
		}
		srcSize := int(binary.LittleEndian.Uint16(plaintext[offset : offset+2]))
		chunk := plaintext[offset+2 : offset+2+srcSize]
		dst := make([]byte, uncompressedSize)
		n, err := lz4.UncompressBlock(chunk, dst)
		if err != nil {
			return nil, err
		}
		out = append(out, dst[:n]...)
		offset += 2 + srcSize
		dstSize -= uncompressedSize
	}
	return out, nil
}
