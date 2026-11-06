package magma

import (
	"bytes"
	"encoding/hex"
	"math/rand"
	"testing"
)

// Примеры из ГОСТ Р 34.12-2015, приложение А.2 (RFC 8891, приложение A).
func TestTransformT(t2 *testing.T) {
	cases := [][2]uint32{
		{0xfdb97531, 0x2a196f34},
		{0x2a196f34, 0xebd9f03a},
		{0xebd9f03a, 0xb039bb3d},
		{0xb039bb3d, 0x68695433},
	}
	for _, c := range cases {
		if got := t(c[0]); got != c[1] {
			t2.Fatalf("t(%08x) = %08x, want %08x", c[0], got, c[1])
		}
	}
}

func TestTransformG(t *testing.T) {
	cases := [][3]uint32{
		{0x87654321, 0xfedcba98, 0xfdcbc20c},
		{0xfdcbc20c, 0x87654321, 0x7e791a4b},
		{0x7e791a4b, 0xfdcbc20c, 0xc76549ec},
		{0xc76549ec, 0x7e791a4b, 0x9791c849},
	}
	for _, c := range cases {
		if got := g(c[0], c[1]); got != c[2] {
			t.Fatalf("g[%08x](%08x) = %08x, want %08x", c[0], c[1], got, c[2])
		}
	}
}

func TestBlock(t *testing.T) {
	key, _ := hex.DecodeString("ffeeddccbbaa99887766554433221100f0f1f2f3f4f5f6f7f8f9fafbfcfdfeff")
	c, err := New(key)
	if err != nil {
		t.Fatal(err)
	}
	pt, _ := hex.DecodeString("fedcba9876543210")
	ct := make([]byte, 8)
	c.Encrypt(ct, pt)
	if hex.EncodeToString(ct) != "4ee901e5c2d8ca3d" {
		t.Fatalf("Encrypt = %x", ct)
	}
	back := make([]byte, 8)
	c.Decrypt(back, ct)
	if !bytes.Equal(back, pt) {
		t.Fatalf("Decrypt = %x", back)
	}
}

func TestRoundTrip(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	for i := 0; i < 1000; i++ {
		key := make([]byte, 32)
		pt := make([]byte, 8)
		rng.Read(key)
		rng.Read(pt)
		c, _ := New(key)
		ct := make([]byte, 8)
		c.Encrypt(ct, pt)
		c.Decrypt(ct, ct)
		if !bytes.Equal(ct, pt) {
			t.Fatal("Decrypt не обратен Encrypt")
		}
	}
}

func BenchmarkEncrypt(b *testing.B) {
	c, _ := New(make([]byte, 32))
	blk := make([]byte, 8)
	b.SetBytes(8)
	for i := 0; i < b.N; i++ {
		c.Encrypt(blk, blk)
	}
}
