package cloud115

import "testing"

func FuzzM115Encode(f *testing.F) {
	f.Add([]byte("0123456789abcdef"), "hello, 115 world!")
	f.Add([]byte("0123456789abcdef"), "")
	f.Add([]byte("0123456789abcdef"), "你好世界")

	f.Fuzz(func(t *testing.T, key []byte, plaintext string) {
		if len(key) != 16 {
			t.Skip("key must be 16 bytes")
		}
		// M115Encode must not panic on any 16-byte key + arbitrary plaintext.
		M115Encode(key, plaintext)
	})
}

func FuzzXorTransformSelfInverse(f *testing.F) {
	f.Add([]byte("key"), []byte("data"))
	f.Add([]byte("k"), []byte(""))
	f.Add([]byte("longerkeyvalue!!"), []byte("short"))

	f.Fuzz(func(t *testing.T, key, data []byte) {
		if len(key) == 0 {
			t.Skip("empty key")
		}
		original := make([]byte, len(data))
		copy(original, data)

		xorTransform(key, data)
		xorTransform(key, data)

		for i := range data {
			if data[i] != original[i] {
				t.Fatalf("xorTransform is not self-inverse at byte %d", i)
			}
		}
	})
}
