// Package kuznyechik реализует блочный шифр «Кузнечик»
// (ГОСТ Р 34.12-2015, n = 128, k = 256; английская версия — RFC 7801).
//
// Зашифрование табличное: преобразования S и L объединены в 16 таблиц
// по 256 записей (64 КиБ), раунд — 16 обращений к таблицам и XOR. Это нужно
// генератору позиций, которому требуются миллионы блоков. Табличная реализация
// не защищена от атак по времени обращения к кэшу; для программы, которая
// работает на компьютере владельца ключа, это допустимо.
//
// Расшифрование сделано прямо по стандарту (S⁻¹, L⁻¹): режимам MGM и CTR
// оно не нужно, но его требует интерфейс cipher.Block.
package kuznyechik

import (
	"crypto/cipher"
	"encoding/binary"
	"fmt"
)

const (
	BlockSize = 16
	KeySize   = 32
)

// pi — подстановка π' из ГОСТ Р 34.12-2015 (совпадает с π из ГОСТ Р 34.11-2012).
var pi = [256]byte{
	252, 238, 221, 17, 207, 110, 49, 22, 251, 196, 250, 218, 35, 197, 4, 77,
	233, 119, 240, 219, 147, 46, 153, 186, 23, 54, 241, 187, 20, 205, 95, 193,
	249, 24, 101, 90, 226, 92, 239, 33, 129, 28, 60, 66, 139, 1, 142, 79,
	5, 132, 2, 174, 227, 106, 143, 160, 6, 11, 237, 152, 127, 212, 211, 31,
	235, 52, 44, 81, 234, 200, 72, 171, 242, 42, 104, 162, 253, 58, 206, 204,
	181, 112, 14, 86, 8, 12, 118, 18, 191, 114, 19, 71, 156, 183, 93, 135,
	21, 161, 150, 41, 16, 123, 154, 199, 243, 145, 120, 111, 157, 158, 178, 177,
	50, 117, 25, 61, 255, 53, 138, 126, 109, 84, 198, 128, 195, 189, 13, 87,
	223, 245, 36, 169, 62, 168, 67, 201, 215, 121, 214, 246, 124, 34, 185, 3,
	224, 15, 236, 222, 122, 148, 176, 188, 220, 232, 40, 80, 78, 51, 10, 74,
	167, 151, 96, 115, 30, 0, 98, 68, 26, 184, 56, 130, 100, 159, 38, 65,
	173, 69, 70, 146, 39, 94, 85, 47, 140, 163, 165, 125, 105, 213, 149, 59,
	7, 88, 179, 64, 134, 172, 29, 247, 48, 55, 107, 228, 136, 217, 231, 137,
	225, 27, 131, 73, 76, 63, 248, 254, 141, 83, 170, 144, 202, 216, 133, 97,
	32, 113, 103, 164, 45, 43, 9, 91, 203, 155, 37, 208, 190, 229, 108, 82,
	89, 166, 116, 210, 230, 244, 180, 192, 209, 102, 175, 194, 57, 75, 99, 182,
}

// lCoef — коэффициенты линейного преобразования l (148, 32, 133, ...).
// lCoef[i] умножается на байт блока с индексом i (байт 0 — старший, a₁₅).
var lCoef = [16]byte{148, 32, 133, 16, 194, 192, 1, 251, 1, 192, 194, 16, 133, 32, 148, 1}

var (
	piInv [256]byte
	// lsTable[i][v] — L(eᵢ·π(v)): L от блока, у которого байт i равен π(v),
	// а остальные — нули. L(S(x)) = ⊕ᵢ lsTable[i][xᵢ].
	lsTable [16][256][2]uint64
	// roundConst — итерационные константы C₁..C₃₂ = L(Vec₁₂₈(i)).
	roundConst [32][2]uint64
)

// gfMul — умножение в GF(2⁸) по модулю x⁸ + x⁷ + x⁶ + x + 1.
func gfMul(a, b byte) byte {
	var p byte
	for b != 0 {
		if b&1 != 0 {
			p ^= a
		}
		hi := a & 0x80
		a <<= 1
		if hi != 0 {
			a ^= 0xC3
		}
		b >>= 1
	}
	return p
}

// rStep — преобразование R: l(a) в начало, остальное сдвигается вправо на байт.
func rStep(a *[BlockSize]byte) {
	var z byte
	for i := 0; i < BlockSize; i++ {
		z ^= gfMul(a[i], lCoef[i])
	}
	copy(a[1:], a[:15])
	a[0] = z
}

// rInv — обратное к R.
func rInv(a *[BlockSize]byte) {
	first := a[0]
	copy(a[:15], a[1:])
	z := first
	for i := 0; i < 15; i++ {
		z ^= gfMul(a[i], lCoef[i])
	}
	a[15] = z
}

func lTransform(a *[BlockSize]byte) {
	for i := 0; i < 16; i++ {
		rStep(a)
	}
}

func lInv(a *[BlockSize]byte) {
	for i := 0; i < 16; i++ {
		rInv(a)
	}
}

func init() {
	for i, v := range pi {
		piInv[v] = byte(i)
	}
	// L линейно, поэтому L(S(x)) = ⊕ᵢ L(eᵢ·π(xᵢ)), где eᵢ·v — блок с одним
	// ненулевым байтом на месте i.
	for pos := 0; pos < BlockSize; pos++ {
		for v := 0; v < 256; v++ {
			var blk [BlockSize]byte
			blk[pos] = pi[v]
			lTransform(&blk)
			lsTable[pos][v][0] = binary.BigEndian.Uint64(blk[:8])
			lsTable[pos][v][1] = binary.BigEndian.Uint64(blk[8:])
		}
	}
	for i := 0; i < 32; i++ {
		var c [BlockSize]byte
		c[15] = byte(i + 1)
		lTransform(&c)
		roundConst[i] = [2]uint64{binary.BigEndian.Uint64(c[:8]), binary.BigEndian.Uint64(c[8:])}
	}
}

// lsx вычисляет L(S(a ⊕ k)) по таблицам.
func lsx(hi, lo, khi, klo uint64) (uint64, uint64) {
	x0, x1 := hi^khi, lo^klo
	t := &lsTable
	r0 := t[0][byte(x0>>56)][0] ^ t[1][byte(x0>>48)][0] ^ t[2][byte(x0>>40)][0] ^ t[3][byte(x0>>32)][0] ^
		t[4][byte(x0>>24)][0] ^ t[5][byte(x0>>16)][0] ^ t[6][byte(x0>>8)][0] ^ t[7][byte(x0)][0] ^
		t[8][byte(x1>>56)][0] ^ t[9][byte(x1>>48)][0] ^ t[10][byte(x1>>40)][0] ^ t[11][byte(x1>>32)][0] ^
		t[12][byte(x1>>24)][0] ^ t[13][byte(x1>>16)][0] ^ t[14][byte(x1>>8)][0] ^ t[15][byte(x1)][0]
	r1 := t[0][byte(x0>>56)][1] ^ t[1][byte(x0>>48)][1] ^ t[2][byte(x0>>40)][1] ^ t[3][byte(x0>>32)][1] ^
		t[4][byte(x0>>24)][1] ^ t[5][byte(x0>>16)][1] ^ t[6][byte(x0>>8)][1] ^ t[7][byte(x0)][1] ^
		t[8][byte(x1>>56)][1] ^ t[9][byte(x1>>48)][1] ^ t[10][byte(x1>>40)][1] ^ t[11][byte(x1>>32)][1] ^
		t[12][byte(x1>>24)][1] ^ t[13][byte(x1>>16)][1] ^ t[14][byte(x1>>8)][1] ^ t[15][byte(x1)][1]
	return r0, r1
}

// Cipher — ключ, развёрнутый в 10 раундовых ключей.
type Cipher struct {
	rk [10][2]uint64
}

var _ cipher.Block = (*Cipher)(nil)

// New разворачивает 256-битный ключ (ГОСТ Р 34.12-2015, п. 4.4.4).
func New(key []byte) (*Cipher, error) {
	if len(key) != KeySize {
		return nil, fmt.Errorf("kuznyechik: длина ключа %d байт, нужно %d", len(key), KeySize)
	}
	c := &Cipher{}
	k1h, k1l := binary.BigEndian.Uint64(key[0:8]), binary.BigEndian.Uint64(key[8:16])
	k2h, k2l := binary.BigEndian.Uint64(key[16:24]), binary.BigEndian.Uint64(key[24:32])
	c.rk[0] = [2]uint64{k1h, k1l}
	c.rk[1] = [2]uint64{k2h, k2l}
	for i := 0; i < 4; i++ {
		for j := 0; j < 8; j++ {
			cc := roundConst[8*i+j]
			// F[C](a₁, a₀) = (LSX[C](a₁) ⊕ a₀, a₁)
			th, tl := lsx(k1h, k1l, cc[0], cc[1])
			k1h, k1l, k2h, k2l = th^k2h, tl^k2l, k1h, k1l
		}
		c.rk[2+2*i] = [2]uint64{k1h, k1l}
		c.rk[3+2*i] = [2]uint64{k2h, k2l}
	}
	return c, nil
}

func (c *Cipher) BlockSize() int { return BlockSize }

// Encrypt зашифровывает один блок: X[K₁₀]LSX[K₉]…LSX[K₁](a).
func (c *Cipher) Encrypt(dst, src []byte) {
	if len(src) < BlockSize || len(dst) < BlockSize {
		panic("kuznyechik: блок короче 16 байт")
	}
	hi, lo := binary.BigEndian.Uint64(src[:8]), binary.BigEndian.Uint64(src[8:16])
	for i := 0; i < 9; i++ {
		hi, lo = lsx(hi, lo, c.rk[i][0], c.rk[i][1])
	}
	binary.BigEndian.PutUint64(dst[:8], hi^c.rk[9][0])
	binary.BigEndian.PutUint64(dst[8:16], lo^c.rk[9][1])
}

// Decrypt расшифровывает один блок: X[K₁]S⁻¹L⁻¹X[K₂]…S⁻¹L⁻¹X[K₁₀](b).
func (c *Cipher) Decrypt(dst, src []byte) {
	if len(src) < BlockSize || len(dst) < BlockSize {
		panic("kuznyechik: блок короче 16 байт")
	}
	var a [BlockSize]byte
	copy(a[:], src[:BlockSize])
	xorKey := func(k [2]uint64) {
		var kb [BlockSize]byte
		binary.BigEndian.PutUint64(kb[:8], k[0])
		binary.BigEndian.PutUint64(kb[8:], k[1])
		for i := range a {
			a[i] ^= kb[i]
		}
	}
	xorKey(c.rk[9])
	for r := 8; r >= 0; r-- {
		lInv(&a)
		for i := range a {
			a[i] = piInv[a[i]]
		}
		xorKey(c.rk[r])
	}
	copy(dst, a[:])
}
