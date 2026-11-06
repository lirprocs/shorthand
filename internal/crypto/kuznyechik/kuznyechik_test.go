package kuznyechik

import (
	"bytes"
	"encoding/hex"
	"math/rand"
	"testing"

	"github.com/lirprocs/Kuznyechik/KuznEncrypt"
)

func mustHex(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(err)
	}
	return b
}

// Пример из ГОСТ Р 34.12-2015, приложение А.1 (RFC 7801, раздел 5).
func TestGOSTExample(t *testing.T) {
	c, err := New(mustHex("8899aabbccddeeff0011223344556677fedcba98765432100123456789abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	wantKeys := []string{
		"8899aabbccddeeff0011223344556677", "fedcba98765432100123456789abcdef",
		"db31485315694343228d6aef8cc78c44", "3d4553d8e9cfec6815ebadc40a9ffd04",
		"57646468c44a5e28d3e59246f429f1ac", "bd079435165c6432b532e82834da581b",
		"51e640757e8745de705727265a0098b1", "5a7925017b9fdd3ed72a91a22286f984",
		"bb44e25378c73123a5f32f73cdb6e517", "72e9dd7416bcf45b755dbaa88e4a4043",
	}
	for i, w := range wantKeys {
		var got [16]byte
		hi, lo := c.rk[i][0], c.rk[i][1]
		for j := 0; j < 8; j++ {
			got[j] = byte(hi >> (56 - 8*j))
			got[8+j] = byte(lo >> (56 - 8*j))
		}
		if hex.EncodeToString(got[:]) != w {
			t.Fatalf("K%d = %x, want %s", i+1, got, w)
		}
	}
	pt := mustHex("1122334455667700ffeeddccbbaa9988")
	ct := make([]byte, 16)
	c.Encrypt(ct, pt)
	if hex.EncodeToString(ct) != "7f679d90bebc24305a468d42b9d4edcd" {
		t.Fatalf("Encrypt = %x", ct)
	}
	back := make([]byte, 16)
	c.Decrypt(back, ct)
	if !bytes.Equal(back, pt) {
		t.Fatalf("Decrypt = %x", back)
	}
}

// Сверка с исходной реализацией автора (github.com/lirprocs/Kuznyechik).
func TestMatchesAuthorImplementation(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	for i := 0; i < 50; i++ {
		var key [32]byte
		var pt [16]byte
		rng.Read(key[:])
		rng.Read(pt[:])
		c, _ := New(key[:])
		got := make([]byte, 16)
		c.Encrypt(got, pt[:])
		want := KuznEncrypt.Encrypt(pt, key)
		if !bytes.Equal(got, want[:]) {
			t.Fatalf("расхождение: %x != %x", got, want)
		}
		back := make([]byte, 16)
		c.Decrypt(back, got)
		if !bytes.Equal(back, pt[:]) {
			t.Fatal("Decrypt не обратен Encrypt")
		}
	}
}

func BenchmarkEncrypt(b *testing.B) {
	c, _ := New(make([]byte, 32))
	blk := make([]byte, 16)
	b.SetBytes(16)
	for i := 0; i < b.N; i++ {
		c.Encrypt(blk, blk)
	}
}
