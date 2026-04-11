// Package cloud115 implements M115 (RSA+XOR) and EC115 (ECDH-P224+AES-CBC+LZ4)
// encryption for the 115 cloud storage API.
//
// Ported from py115 (MIT license, github.com/deadblue/py115).
package cloud115

import (
	"crypto/rand"
	"encoding/base64"
	"math/big"
)

// rsaN is the 1024-bit RSA modulus used by 115's M115 channel.
var rsaN = new(big.Int).SetBytes([]byte{
	0x86, 0x86, 0xc4, 0x01, 0x94, 0xad, 0x3c, 0x2c,
	0x45, 0xd5, 0x72, 0x5b, 0x71, 0x39, 0x84, 0x9a,
	0x98, 0xc0, 0x3a, 0x26, 0x59, 0xf5, 0x4c, 0x29,
	0xc2, 0xa5, 0x9e, 0x46, 0x9f, 0x30, 0x0d, 0x7f,
	0x60, 0x2b, 0x51, 0x42, 0xf7, 0xf8, 0x78, 0xf8,
	0x1a, 0xeb, 0x42, 0x65, 0x56, 0x32, 0x77, 0xb1,
	0xb1, 0xcc, 0x22, 0xb7, 0xf5, 0xa1, 0x20, 0xb1,
	0x26, 0x16, 0x5c, 0xf2, 0x3e, 0xae, 0x1b, 0x3e,
	0x0b, 0x84, 0x7f, 0xad, 0x0f, 0x62, 0x54, 0x02,
	0xb2, 0xaa, 0x7e, 0x7a, 0x4c, 0xfd, 0xa4, 0xd7,
	0xfe, 0x4d, 0x2a, 0x16, 0x5b, 0x67, 0xf0, 0xf6,
	0x21, 0x05, 0xd2, 0xb5, 0x98, 0x93, 0x70, 0x1d,
	0xd5, 0x10, 0x7d, 0xb2, 0xc2, 0x51, 0xf4, 0xe9,
	0x9d, 0x91, 0xcd, 0xe6, 0x4e, 0xf6, 0x94, 0xc1,
	0x67, 0x3e, 0xd8, 0x06, 0x3a, 0xf6, 0x06, 0xd1,
	0xb0, 0xd8, 0xa2, 0x46, 0x97, 0x74, 0x2b, 0x6b,
})

var rsaE = big.NewInt(0x10001)

const rsaKeyLen = 128 // bytes

// xorKeySeed is the 144-byte XOR key seed.
var xorKeySeed = []byte{
	0xf0, 0xe5, 0x69, 0xae, 0xbf, 0xdc, 0xbf, 0x8a,
	0x1a, 0x45, 0xe8, 0xbe, 0x7d, 0xa6, 0x73, 0xb8,
	0xde, 0x8f, 0xe7, 0xc4, 0x45, 0xda, 0x86, 0xc4,
	0x9b, 0x64, 0x8b, 0x14, 0x6a, 0xb4, 0xf1, 0xaa,
	0x38, 0x01, 0x35, 0x9e, 0x26, 0x69, 0x2c, 0x86,
	0x00, 0x6b, 0x4f, 0xa5, 0x36, 0x34, 0x62, 0xa6,
	0x2a, 0x96, 0x68, 0x18, 0xf2, 0x4a, 0xfd, 0xbd,
	0x6b, 0x97, 0x8f, 0x4d, 0x8f, 0x89, 0x13, 0xb7,
	0x6c, 0x8e, 0x93, 0xed, 0x0e, 0x0d, 0x48, 0x3e,
	0xd7, 0x2f, 0x88, 0xd8, 0xfe, 0xfe, 0x7e, 0x86,
	0x50, 0x95, 0x4f, 0xd1, 0xeb, 0x83, 0x26, 0x34,
	0xdb, 0x66, 0x7b, 0x9c, 0x7e, 0x9d, 0x7a, 0x81,
	0x32, 0xea, 0xb6, 0x33, 0xde, 0x3a, 0xa9, 0x59,
	0x34, 0x66, 0x3b, 0xaa, 0xba, 0x81, 0x60, 0x48,
	0xb9, 0xd5, 0x81, 0x9c, 0xf8, 0x6c, 0x84, 0x77,
	0xff, 0x54, 0x78, 0x26, 0x5f, 0xbe, 0xe8, 0x1e,
	0x36, 0x9f, 0x34, 0x80, 0x5c, 0x45, 0x2c, 0x9b,
	0x76, 0xd5, 0x1b, 0x8f, 0xcc, 0xc3, 0xb8, 0xf5,
}

// xorClientKey is the 12-byte client-side XOR key.
var xorClientKey = []byte{
	0x78, 0x06, 0xad, 0x4c, 0x33, 0x86, 0x5d, 0x18,
	0x4c, 0x01, 0x3f, 0x46,
}

// rsaEncryptSlice applies PKCS#1 v1.5 type-2 padding and performs RSA encryption
// (modular exponentiation) on a single segment (must be <= 117 bytes).
//
// Padding layout:  0x00 | 0x02 | <random non-zero bytes> | 0x00 | <segment>
// Random bytes are generated with crypto/rand (not math/rand).
func rsaEncryptSlice(segment []byte) []byte {
	padSize := rsaKeyLen - len(segment)
	pad := make([]byte, padSize)
	pad[0] = 0x00
	pad[1] = 0x02
	// Fill pad[2 .. padSize-2] with non-zero random bytes.
	for i := 2; i < padSize-1; i++ {
		b := make([]byte, 1)
		for {
			_, _ = rand.Read(b)
			if b[0] != 0 {
				break
			}
		}
		pad[i] = b[0]
	}
	pad[padSize-1] = 0x00
	padded := append(pad, segment...)
	msg := new(big.Int).SetBytes(padded)
	ct := new(big.Int).Exp(msg, rsaE, rsaN)
	out := make([]byte, rsaKeyLen)
	ct.FillBytes(out)
	return out
}

// rsaDecryptSlice performs RSA "decrypt" (which 115 implements as encrypt with
// the public key) on a single 128-byte ciphertext block, then strips the PKCS
// padding by scanning for the first 0x00 byte after position 0.
func rsaDecryptSlice(segment []byte) []byte {
	msg := new(big.Int).SetBytes(segment)
	pt := new(big.Int).Exp(msg, rsaE, rsaN)
	full := make([]byte, rsaKeyLen)
	pt.FillBytes(full)
	// Strip PKCS padding: find first 0x00 after index 0.
	for i := 1; i < len(full); i++ {
		if full[i] == 0 {
			return full[i+1:]
		}
	}
	return full
}

// rsaEncrypt splits plaintext into 117-byte chunks and encrypts each with
// rsaEncryptSlice, producing concatenated 128-byte ciphertext blocks.
func rsaEncrypt(plaintext []byte) []byte {
	const maxSlice = rsaKeyLen - 11 // 117
	var out []byte
	for len(plaintext) > 0 {
		end := maxSlice
		if end > len(plaintext) {
			end = len(plaintext)
		}
		out = append(out, rsaEncryptSlice(plaintext[:end])...)
		plaintext = plaintext[end:]
	}
	return out
}

// rsaDecrypt splits ciphertext into 128-byte blocks and decrypts each.
func rsaDecrypt(ciphertext []byte) []byte {
	var out []byte
	for len(ciphertext) > 0 {
		end := rsaKeyLen
		if end > len(ciphertext) {
			end = len(ciphertext)
		}
		out = append(out, rsaDecryptSlice(ciphertext[:end])...)
		ciphertext = ciphertext[end:]
	}
	return out
}

// xorDeriveKey derives an XOR key of the given size from source bytes and the
// global xorKeySeed.
//
// Python equivalent:
//
//	key[i] = (source[i] + seed[size*i]) & 0xFF ^ seed[size*(size-i-1)]
func xorDeriveKey(source []byte, size int) []byte {
	key := make([]byte, size)
	for i := 0; i < size; i++ {
		v := (int(source[i]) + int(xorKeySeed[size*i])) & 0xFF
		v ^= int(xorKeySeed[size*(size-i-1)])
		key[i] = byte(v)
	}
	return key
}

// xorTransform XORs data in-place using the given key.
//
// The first (len(data) % 4) bytes use key[i % keyLen]; the remaining bytes use
// key[(i - modSize) % keyLen].  This matches the Python _xor_transform exactly.
func xorTransform(key, data []byte) {
	keySize := len(key)
	dataSize := len(data)
	modSize := dataSize % 4
	for i := 0; i < dataSize; i++ {
		if i < modSize {
			data[i] ^= key[i%keySize]
		} else {
			data[i] ^= key[(i-modSize)%keySize]
		}
	}
}

// M115Encode encodes a plaintext string for the M115 download channel.
//
// Pipeline: XOR(deriveKey(key,4)) → reverse → XOR(clientKey) → prepend key →
// RSA encrypt → base64.
func M115Encode(key []byte, plaintext string) string {
	data := []byte(plaintext)
	xorTransform(xorDeriveKey(key, 4), data)
	// reverse
	for i, j := 0, len(data)-1; i < j; i, j = i+1, j-1 {
		data[i], data[j] = data[j], data[i]
	}
	xorTransform(xorClientKey, data)
	data = append(key, data...)
	return base64.StdEncoding.EncodeToString(rsaEncrypt(data))
}

// M115Decode decodes a base64 ciphertext from the M115 download channel.
//
// Pipeline: base64 → RSA decrypt → extract serverKey(16) → XOR(deriveKey(serverKey,12))
// → reverse → XOR(deriveKey(key,4)).
func M115Decode(key []byte, ciphertext string) string {
	raw, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return ""
	}
	data := rsaDecrypt(raw)
	serverKey := data[:16]
	data = data[16:]
	xorTransform(xorDeriveKey(serverKey, 12), data)
	// reverse
	for i, j := 0, len(data)-1; i < j; i, j = i+1, j-1 {
		data[i], data[j] = data[j], data[i]
	}
	xorTransform(xorDeriveKey(key, 4), data)
	return string(data)
}

// GenerateM115Key returns 16 cryptographically random bytes suitable for use
// as an M115 session key.
func GenerateM115Key() []byte {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return b
}
