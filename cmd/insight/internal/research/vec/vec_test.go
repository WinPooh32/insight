package vec_test

import (
	"bytes"
	"testing"

	"github.com/WinPooh32/insight/cmd/insight/internal/research/vec"
)

func TestRoundTrip(t *testing.T) {
	t.Parallel()

	// The BLOB must be little-endian float32: 1.0 and 2.0.
	want := []byte{
		0x00, 0x00, 0x80, 0x3F, // 1.0
		0x00, 0x00, 0x00, 0x40, // 2.0
	}

	if got := vec.Encode([]float32{1, 2}); !bytes.Equal(got, want) {
		t.Errorf("Encode = %x, want %x", got, want)
	}

	got, err := vec.Decode(want)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Errorf("Decode = %v, want [1 2]", got)
	}
}

func TestDecodeNotMultiple(t *testing.T) {
	t.Parallel()

	// A BLOB whose length is not a multiple of 4 cannot decode.
	if _, err := vec.Decode([]byte{1, 2, 3}); err == nil {
		t.Error("expected error for a BLOB of 3 bytes")
	}
}
