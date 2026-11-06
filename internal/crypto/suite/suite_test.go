package suite

import (
	"bytes"
	"testing"
)

func TestModes(t *testing.T) {
	for _, m := range All {
		key := bytes.Repeat([]byte{0x42}, m.KeySize())
		a, err := m.NewAEAD(key)
		if err != nil {
			t.Fatalf("%v: %v", m, err)
		}
		if a.NonceSize() != m.NonceSize() {
			t.Fatalf("%v: NonceSize %d != %d", m, a.NonceSize(), m.NonceSize())
		}
		nonce := make([]byte, m.NonceSize())
		pt := []byte("проверка режима")
		ct := a.Seal(nil, nonce, pt, []byte("ad"))
		back, err := a.Open(nil, nonce, ct, []byte("ad"))
		if err != nil || !bytes.Equal(back, pt) {
			t.Fatalf("%v: не расшифровалось", m)
		}
		if _, err := a.Open(nil, nonce, ct, []byte("другое")); err == nil {
			t.Fatalf("%v: чужие доп. данные приняты", m)
		}

		k32 := bytes.Repeat([]byte{1}, 32)
		r1, r2 := m.NewRand(k32), m.NewRand(k32)
		for i := 0; i < 1000; i++ {
			if r1.Uint64() != r2.Uint64() {
				t.Fatalf("%v: генератор недетерминирован", m)
			}
		}
		r3 := m.NewRand(bytes.Repeat([]byte{2}, 32))
		if r3.Uint64() == m.NewRand(k32).Uint64() {
			t.Fatalf("%v: разные ключи — одинаковый поток", m)
		}
	}
}

// Грубая проверка равномерности Uint64n: 16 корзин, χ² с 15 степенями свободы.
func TestUniform(t *testing.T) {
	for _, m := range All {
		r := m.NewRand(bytes.Repeat([]byte{3}, 32))
		var cnt [16]float64
		const n = 160000
		for i := 0; i < n; i++ {
			cnt[r.Uint64n(16)]++
		}
		chi := 0.0
		for _, c := range cnt {
			d := c - n/16
			chi += d * d / (n / 16)
		}
		if chi > 37.7 { // p = 0.001
			t.Fatalf("%v: χ² = %.1f", m, chi)
		}
	}
}
