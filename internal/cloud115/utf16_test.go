package cloud115

import "testing"

// TestDecodeUTF16LE_ASCII verifies that plain ASCII encoded as UTF-16 LE is
// decoded correctly.
func TestDecodeUTF16LE_ASCII(t *testing.T) {
	// "ABC" in UTF-16 LE: 41 00  42 00  43 00
	data := []byte{'A', 0, 'B', 0, 'C', 0}
	result := decodeUTF16LE(data)
	if result != "ABC" {
		t.Fatalf("got %q, want ABC", result)
	}
}

// TestDecodeUTF16LE_Chinese verifies that a Chinese character encoded as
// UTF-16 LE is decoded correctly.
func TestDecodeUTF16LE_Chinese(t *testing.T) {
	// "影" = U+5F71, UTF-16 LE = 0x71 0x5F
	data := []byte{0x71, 0x5F}
	result := decodeUTF16LE(data)
	if result != "影" {
		t.Fatalf("got %q, want 影", result)
	}
}

// TestDecodeUTF16LE_BOM verifies that a leading BOM (FF FE) is stripped before
// decoding.
func TestDecodeUTF16LE_BOM(t *testing.T) {
	// BOM (FF FE) + "A" (41 00)
	data := []byte{0xFF, 0xFE, 0x41, 0x00}
	result := decodeUTF16LE(data)
	if result != "A" {
		t.Fatalf("got %q, want A (BOM should be stripped)", result)
	}
}

// TestDecodeUTF16LE_Empty verifies that nil input returns an empty string.
func TestDecodeUTF16LE_Empty(t *testing.T) {
	result := decodeUTF16LE(nil)
	if result != "" {
		t.Fatalf("got %q, want empty", result)
	}
}

// TestDecodeUTF16LE_SurrogatePair verifies that surrogate pairs are decoded
// into the correct Unicode code point.
func TestDecodeUTF16LE_SurrogatePair(t *testing.T) {
	// U+1F600 (😀) = surrogate pair D83D DE00 in UTF-16.
	// LE bytes: 3D D8  00 DE
	data := []byte{0x3D, 0xD8, 0x00, 0xDE}
	result := decodeUTF16LE(data)
	if result != "😀" {
		t.Fatalf("got %q, want 😀", result)
	}
}
