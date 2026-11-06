// Package prng — детерминированные криптографические генераторы псевдослучайных
// чисел из ключа. Поток выдаёт выбранный примитив:
//
//   - блочный шифр в режиме гаммирования CTR (ГОСТ Р 34.13-2015, п. 5.2):
//     «Кузнечик» или «Магма», счётчик с нуля;
//   - Ascon-XOF128 от ключа (NIST SP 800-232).
//
// Поверх потока — равномерные целые без смещения (метод Лемира).
package prng

import (
	"crypto/cipher"
	"encoding/binary"
	"io"
	"math/bits"

	"github.com/lirprocs/shorthand/internal/crypto/ascon"
)

// Rand — генератор поверх ключевого потока.
type Rand struct {
	src io.Reader
	buf [4096]byte
	pos int
}

func newRand(src io.Reader) *Rand {
	r := &Rand{src: src}
	r.pos = len(r.buf)
	return r
}

type ctrReader struct{ s cipher.Stream }

func (c ctrReader) Read(p []byte) (int, error) {
	clear(p)
	c.s.XORKeyStream(p, p)
	return len(p), nil
}

// NewCTR — генератор на блочном шифре в режиме CTR с нулевым начальным счётчиком.
// Ключ у каждого генератора свой, поэтому нулевой счётчик безопасен.
func NewCTR(b cipher.Block) *Rand {
	iv := make([]byte, b.BlockSize())
	return newRand(ctrReader{cipher.NewCTR(b, iv)})
}

// NewXOF — генератор Ascon-XOF128(key).
func NewXOF(key []byte) *Rand {
	x := ascon.NewXOF128()
	x.Write(key)
	return newRand(x)
}

// Read заполняет p байтами потока.
func (r *Rand) Read(p []byte) (int, error) {
	n := len(p)
	for len(p) > 0 {
		if r.pos == len(r.buf) {
			r.src.Read(r.buf[:])
			r.pos = 0
		}
		k := copy(p, r.buf[r.pos:])
		r.pos += k
		p = p[k:]
	}
	return n, nil
}

// Uint64 — следующие 8 байт потока.
func (r *Rand) Uint64() uint64 {
	if len(r.buf)-r.pos < 8 {
		var b [8]byte
		r.Read(b[:])
		return binary.LittleEndian.Uint64(b[:])
	}
	v := binary.LittleEndian.Uint64(r.buf[r.pos:])
	r.pos += 8
	return v
}

// Uint64n — равномерное число из [0, n) без смещения (D. Lemire, 2019).
func (r *Rand) Uint64n(n uint64) uint64 {
	if n == 0 {
		panic("prng: n = 0")
	}
	hi, lo := bits.Mul64(r.Uint64(), n)
	if lo < n {
		thresh := -n % n
		for lo < thresh {
			hi, lo = bits.Mul64(r.Uint64(), n)
		}
	}
	return hi
}

// Bit — один случайный бит.
func (r *Rand) Bit() bool { return r.Uint64()&1 == 1 }
