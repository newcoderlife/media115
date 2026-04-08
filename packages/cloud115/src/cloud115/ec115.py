"""EC115 encryption for 115 upload API.

Based on py115 (MIT license, github.com/deadblue/py115).
Uses ECDH P-224 + AES-128-CBC + LZ4 for the upload init channel.
"""
from __future__ import annotations

import base64
import random
import struct
import time
from binascii import crc32

from Crypto.Cipher import AES
from Crypto.PublicKey import ECC
import lz4.block

# fmt: off
# 115 server public key (P-224, SEC1 uncompressed, 57 bytes)
_SERVER_PUB_KEY = bytes([
    0x04, 0x57, 0xa2, 0x92, 0x57, 0xcd, 0x23, 0x20,
    0xe5, 0xd6, 0xd1, 0x43, 0x32, 0x2f, 0xa4, 0xbb,
    0x8a, 0x3c, 0xf9, 0xd3, 0xcc, 0x62, 0x3e, 0xf5,
    0xed, 0xac, 0x62, 0xb7, 0x67, 0x8a, 0x89, 0xc9,
    0x1a, 0x83, 0xba, 0x80, 0x0d, 0x61, 0x29, 0xf5,
    0x22, 0xd0, 0x34, 0xc8, 0x95, 0xdd, 0x24, 0x65,
    0x24, 0x3a, 0xdd, 0xc2, 0x50, 0x95, 0x3b, 0xee,
    0xba,
])
# fmt: on

# CRC salt for encode_token checksum
_CRC_SALT = b"^j>WD3Kr?J2gLFjD4W2y@"

# Token salt for HMAC-style token computation
TOKEN_SALT = "Qclm8MGWUv59TnrR0XPg"


class EC115Cipher:
    """EC115 cipher: ECDH key exchange, AES-128-CBC encrypt/decrypt, LZ4 decompress."""

    def __init__(self):
        server_key = ECC.import_key(encoded=_SERVER_PUB_KEY, curve_name="P-224")
        client_key = ECC.generate(curve="P-224")
        # pub_key = 0x1d prefix + compressed SEC1 (29 bytes) = 30 bytes total
        self._pub_key = b"\x1d" + client_key.public_key().export_key(
            format="SEC1", compress=True
        )
        # ECDH shared secret: server_pub * client_priv
        shared_point = server_key.pointQ * client_key.d
        shared_secret = shared_point.x.to_bytes(28, "big")
        self._aes_key = shared_secret[:16]
        self._aes_iv = shared_secret[-16:]

    def encode_token(self) -> str:
        """Build the k_ec query parameter from the client public key."""
        pub_key = self._pub_key  # 30 bytes
        timestamp = int(time.time())
        # struct: 15s B I I 15s B I  (total = 15+1+4+4+15+1+4 = 44 bytes)
        token = bytearray(
            struct.pack(
                "<15sBII15sBI",
                pub_key[:15],
                0,
                115,
                timestamp,
                pub_key[15:],
                0,
                1,
            )
        )
        r1 = random.randint(0, 0xFF)
        r2 = random.randint(0, 0xFF)
        for i in range(len(token)):
            token[i] ^= r1 if i < 24 else r2
        checksum = crc32(_CRC_SALT + bytes(token)) & 0xFFFFFFFF
        token += struct.pack("<I", checksum)
        return base64.b64encode(bytes(token)).decode()

    def encode(self, data: bytes) -> bytes:
        """AES-128-CBC encrypt. Zero-pad to 16-byte boundary (no padding if already aligned)."""
        pad_size = 16 - len(data) % 16
        if pad_size != 16:
            data = data + b"\x00" * pad_size
        cipher = AES.new(key=self._aes_key, mode=AES.MODE_CBC, iv=self._aes_iv)
        return cipher.encrypt(data)

    def decode(self, data: bytes) -> bytes:
        """AES-128-CBC decrypt + LZ4 block decompress.

        The last 12 bytes are a trailer: first 4 bytes (XOR'd with byte 7) hold the
        total uncompressed size; the rest is padding.  The ciphertext (data[:-12]) is
        decrypted with AES-CBC, then decompressed in 8 KiB chunks.
        """
        ciphertext = data[:-12]
        tail = bytearray(data[-12:])
        cipher = AES.new(key=self._aes_key, mode=AES.MODE_CBC, iv=self._aes_iv)
        plaintext = bytearray(cipher.decrypt(ciphertext))

        # Decode total uncompressed size from tail
        for i in range(4):
            tail[i] ^= tail[7]
        dst_size = struct.unpack("<I", bytes(tail[:4]))[0]

        buf: list[bytes] = []
        offset = 0
        while dst_size > 0:
            uncompressed_size = min(dst_size, 8192)
            src_size = struct.unpack("<H", bytes(plaintext[offset : offset + 2]))[0]
            chunk = bytes(plaintext[offset + 2 : offset + 2 + src_size])
            buf.append(lz4.block.decompress(chunk, uncompressed_size))
            offset += 2 + src_size
            dst_size -= uncompressed_size

        return b"".join(buf)
