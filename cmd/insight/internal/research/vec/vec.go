// Package vec serializes float32 vectors to little-endian bytes.
package vec

import (
	"encoding/binary"
	"fmt"
	"math"
)

// float32Size is the size of one float32 in the cache BLOB.
const float32Size = 4

// Encode serializes a float32 vector to little-endian bytes.
func Encode(vec []float32) []byte {
	buf := make([]byte, float32Size*len(vec))
	for i, v := range vec {
		binary.LittleEndian.PutUint32(buf[float32Size*i:], math.Float32bits(v))
	}

	return buf
}

// Decode reverses Encode.
func Decode(buf []byte) ([]float32, error) {
	if len(buf)%float32Size != 0 {
		return nil, fmt.Errorf("vector blob of %d bytes is not a multiple of %d", len(buf), float32Size)
	}

	vec := make([]float32, len(buf)/float32Size)
	for i := range vec {
		vec[i] = math.Float32frombits(binary.LittleEndian.Uint32(buf[float32Size*i:]))
	}

	return vec, nil
}
