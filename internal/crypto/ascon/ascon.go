// Package ascon реализует лёгкую криптографию Ascon по стандарту NIST SP 800-232
// (август 2025): аутентифицированное шифрование Ascon-AEAD128 и функцию с
// расширяемым выходом Ascon-XOF128.
//
// Всё построено на одной перестановке Ascon-p над 320-битным состоянием из
// пяти 64-битных слов: подстановочный слой из 5-битных S-блоков (в битслайс-
// записи — несколько операций AND/XOR/NOT над словами) и линейный слой из
// сдвигов. Байты загружаются в слова в порядке little-endian (как в стандарте).
package ascon

import (
	"crypto/cipher"
	"crypto/subtle"
	"encoding/binary"
	"errors"
	"math/bits"
)

const (
	KeySize   = 16
	NonceSize = 16
	TagSize   = 16
	rate      = 16 // байт на блок в AEAD

	ivAEAD128 = 0x00001000808c0001
	ivXOF128  = 0x0000080000cc0003
)

// roundConst — константы раундов; Ascon-p[r] использует последние r из 12.
var roundConst = [12]uint64{0xf0, 0xe1, 0xd2, 0xc3, 0xb4, 0xa5, 0x96, 0x87, 0x78, 0x69, 0x5a, 0x4b}

type state [5]uint64

// permute — перестановка Ascon-p[rounds].
func (s *state) permute(rounds int) {
	x0, x1, x2, x3, x4 := s[0], s[1], s[2], s[3], s[4]
	for _, c := range roundConst[12-rounds:] {
		// добавление константы
		x2 ^= c
		// подстановочный слой
		x0 ^= x4
		x4 ^= x3
		x2 ^= x1
		t0 := ^x0 & x1
		t1 := ^x1 & x2
		t2 := ^x2 & x3
		t3 := ^x3 & x4
		t4 := ^x4 & x0
		x0 ^= t1
		x1 ^= t2
		x2 ^= t3
		x3 ^= t4
		x4 ^= t0
		x1 ^= x0
		x0 ^= x4
		x3 ^= x2
		x2 = ^x2
		// линейный слой
		x0 ^= bits.RotateLeft64(x0, -19) ^ bits.RotateLeft64(x0, -28)
		x1 ^= bits.RotateLeft64(x1, -61) ^ bits.RotateLeft64(x1, -39)
		x2 ^= bits.RotateLeft64(x2, -1) ^ bits.RotateLeft64(x2, -6)
		x3 ^= bits.RotateLeft64(x3, -10) ^ bits.RotateLeft64(x3, -17)
		x4 ^= bits.RotateLeft64(x4, -7) ^ bits.RotateLeft64(x4, -41)
	}
	s[0], s[1], s[2], s[3], s[4] = x0, x1, x2, x3, x4
}

// load читает до 8 байт little-endian (неполное слово дополняется нулями).
func load(b []byte) uint64 {
	if len(b) >= 8 {
		return binary.LittleEndian.Uint64(b)
	}
	var buf [8]byte
	copy(buf[:], b)
	return binary.LittleEndian.Uint64(buf[:])
}

func store(dst []byte, v uint64) {
	if len(dst) >= 8 {
		binary.LittleEndian.PutUint64(dst, v)
		return
	}
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], v)
	copy(dst, buf[:])
}

// pad — бит-дополнение «1, затем нули» после n байт слова.
func pad(n int) uint64 { return 1 << (8 * uint(n)) }

// ─── Ascon-AEAD128 ──────────────────────────────────────────────────────────

type aead struct {
	k0, k1 uint64
}

var errOpen = errors.New("ascon: имитовставка не совпала")

// NewAEAD128 возвращает Ascon-AEAD128 с 128-битным ключом, nonce и имитовставкой.
func NewAEAD128(key []byte) (cipher.AEAD, error) {
	if len(key) != KeySize {
		return nil, errors.New("ascon: ключ должен быть 16 байт")
	}
	return &aead{k0: load(key[:8]), k1: load(key[8:])}, nil
}

func (a *aead) NonceSize() int { return NonceSize }
func (a *aead) Overhead() int  { return TagSize }

// init — инициализация и обработка дополнительных данных.
func (a *aead) init(nonce, ad []byte) state {
	s := state{ivAEAD128, a.k0, a.k1, load(nonce[:8]), load(nonce[8:16])}
	s.permute(12)
	s[3] ^= a.k0
	s[4] ^= a.k1
	if len(ad) > 0 {
		for len(ad) >= rate {
			s[0] ^= load(ad[:8])
			s[1] ^= load(ad[8:16])
			s.permute(8)
			ad = ad[rate:]
		}
		// последний (возможно пустой) блок с дополнением
		if len(ad) >= 8 {
			s[0] ^= load(ad[:8])
			s[1] ^= load(ad[8:]) ^ pad(len(ad)-8)
		} else {
			s[0] ^= load(ad) ^ pad(len(ad))
		}
		s.permute(8)
	}
	s[4] ^= 1 << 63 // разделение доменов
	return s
}

func (a *aead) finalize(s *state, tag []byte) {
	s[2] ^= a.k0
	s[3] ^= a.k1
	s.permute(12)
	store(tag[:8], s[3]^a.k0)
	store(tag[8:16], s[4]^a.k1)
}

func (a *aead) Seal(dst, nonce, plaintext, additionalData []byte) []byte {
	if len(nonce) != NonceSize {
		panic("ascon: nonce должен быть 16 байт")
	}
	ret, out := sliceForAppend(dst, len(plaintext)+TagSize)
	s := a.init(nonce, additionalData)
	p, c := plaintext, out
	for len(p) >= rate {
		s[0] ^= load(p[:8])
		s[1] ^= load(p[8:16])
		store(c[:8], s[0])
		store(c[8:16], s[1])
		s.permute(8)
		p, c = p[rate:], c[rate:]
	}
	n := len(p)
	if n >= 8 {
		s[0] ^= load(p[:8])
		s[1] ^= load(p[8:])
		store(c[:8], s[0])
		store(c[8:n], s[1])
		s[1] ^= pad(n - 8)
	} else {
		s[0] ^= load(p)
		store(c[:n], s[0])
		s[0] ^= pad(n)
	}
	a.finalize(&s, out[len(plaintext):])
	return ret
}

func (a *aead) Open(dst, nonce, ciphertext, additionalData []byte) ([]byte, error) {
	if len(nonce) != NonceSize {
		panic("ascon: nonce должен быть 16 байт")
	}
	if len(ciphertext) < TagSize {
		return nil, errOpen
	}
	ct := ciphertext[:len(ciphertext)-TagSize]
	tag := ciphertext[len(ciphertext)-TagSize:]
	ret, out := sliceForAppend(dst, len(ct))
	s := a.init(nonce, additionalData)
	c, p := ct, out
	for len(c) >= rate {
		c0, c1 := load(c[:8]), load(c[8:16])
		store(p[:8], s[0]^c0)
		store(p[8:16], s[1]^c1)
		s[0], s[1] = c0, c1
		s.permute(8)
		c, p = c[rate:], p[rate:]
	}
	n := len(c)
	if n >= 8 {
		c0 := load(c[:8])
		store(p[:8], s[0]^c0)
		s[0] = c0
		m := n - 8
		c1 := load(c[8:])
		pt1 := (s[1] ^ c1) & (pad(m) - 1)
		store(p[8:n], pt1)
		s[1] ^= pt1 ^ pad(m)
	} else {
		c0 := load(c)
		pt0 := (s[0] ^ c0) & (pad(n) - 1)
		store(p[:n], pt0)
		s[0] ^= pt0 ^ pad(n)
	}
	var want [TagSize]byte
	a.finalize(&s, want[:])
	if subtle.ConstantTimeCompare(want[:], tag) != 1 {
		clear(out)
		return nil, errOpen
	}
	return ret, nil
}

// ─── Ascon-XOF128 ───────────────────────────────────────────────────────────

// XOF — Ascon-XOF128: сначала Write (сколько угодно раз), затем Read выдаёт
// неограниченный поток. После первого Read запись запрещена.
type XOF struct {
	s        state
	buf      [8]byte
	n        int // байт в buf
	squeeze  bool
	out      [8]byte
	outAvail int
}

// NewXOF128 создаёт Ascon-XOF128.
func NewXOF128() *XOF {
	x := &XOF{}
	x.s = state{ivXOF128}
	x.s.permute(12)
	return x
}

func (x *XOF) Write(p []byte) (int, error) {
	if x.squeeze {
		panic("ascon: Write после Read")
	}
	total := len(p)
	if x.n > 0 {
		k := copy(x.buf[x.n:], p)
		x.n += k
		p = p[k:]
		if x.n < 8 {
			return total, nil
		}
		x.s[0] ^= load(x.buf[:])
		x.s.permute(12)
		x.n = 0
	}
	for len(p) >= 8 {
		x.s[0] ^= load(p[:8])
		x.s.permute(12)
		p = p[8:]
	}
	x.n = copy(x.buf[:], p)
	return total, nil
}

func (x *XOF) Read(p []byte) (int, error) {
	if !x.squeeze {
		x.s[0] ^= load(x.buf[:x.n]) ^ pad(x.n)
		x.s.permute(12)
		x.squeeze = true
		store(x.out[:], x.s[0])
		x.outAvail = 8
	}
	total := len(p)
	for len(p) > 0 {
		if x.outAvail == 0 {
			x.s.permute(12)
			store(x.out[:], x.s[0])
			x.outAvail = 8
		}
		k := copy(p, x.out[8-x.outAvail:])
		x.outAvail -= k
		p = p[k:]
	}
	return total, nil
}

// sliceForAppend — как в crypto/cipher.
func sliceForAppend(in []byte, n int) (head, tail []byte) {
	if total := len(in) + n; cap(in) >= total {
		head = in[:total]
	} else {
		head = make([]byte, total)
		copy(head, in)
	}
	tail = head[len(in):]
	return
}
