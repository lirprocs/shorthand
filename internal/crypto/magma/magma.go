// Package magma реализует блочный шифр «Магма»
// (ГОСТ Р 34.12-2015, n = 64, k = 256; английская версия — RFC 8891).
//
// «Магма» — 32 раунда сети Фейстеля с 64-битным блоком; по меркам ГОСТ это
// лёгкий шифр: раунд — сложение по модулю 2³², восемь 4-битных подстановок и
// циклический сдвиг. Здесь подстановки и сдвиг на 11 объединены в четыре
// таблицы по 256 32-битных слов.
package magma

import (
	"crypto/cipher"
	"encoding/binary"
	"fmt"
)

const (
	BlockSize = 8
	KeySize   = 32
)

// pi — подстановки π'₀…π'₇ (параметры id-tc26-gost-28147-param-Z).
var pi = [8][16]byte{
	{12, 4, 6, 2, 10, 5, 11, 9, 14, 8, 13, 7, 0, 3, 15, 1},
	{6, 8, 2, 3, 9, 10, 5, 12, 1, 14, 4, 7, 11, 13, 0, 15},
	{11, 3, 5, 8, 2, 15, 10, 13, 14, 1, 7, 4, 12, 9, 6, 0},
	{12, 8, 2, 1, 13, 4, 15, 6, 7, 0, 10, 5, 3, 14, 9, 11},
	{7, 15, 5, 10, 8, 1, 6, 13, 0, 9, 3, 14, 11, 4, 2, 12},
	{5, 13, 15, 6, 9, 2, 12, 10, 11, 7, 8, 1, 4, 3, 14, 0},
	{8, 14, 2, 5, 6, 9, 1, 12, 15, 4, 11, 0, 13, 10, 3, 7},
	{1, 7, 14, 13, 0, 5, 8, 3, 4, 15, 10, 6, 9, 12, 11, 2},
}

// sbox[j][b] = (t(b·2^{8j})) <<< 11: две подстановки байта j и сдвиг.
var sbox [4][256]uint32

func init() {
	for j := 0; j < 4; j++ {
		for b := 0; b < 256; b++ {
			lo := uint32(pi[2*j][b&15])
			hi := uint32(pi[2*j+1][b>>4])
			v := (hi<<4 | lo) << (8 * j)
			sbox[j][b] = v<<11 | v>>21
		}
	}
}

// g[k](a) = t(a ⊞ k) <<< 11.
func g(k, a uint32) uint32 {
	x := a + k
	return sbox[0][byte(x)] ^ sbox[1][byte(x>>8)] ^ sbox[2][byte(x>>16)] ^ sbox[3][byte(x>>24)]
}

// t — нелинейное преобразование без сдвига (для тестов по RFC 8891).
func t(a uint32) uint32 {
	var y uint32
	for i := 0; i < 8; i++ {
		y |= uint32(pi[i][(a>>(4*i))&15]) << (4 * i)
	}
	return y
}

// Cipher — ключ «Магмы»: раундовые ключи для зашифрования и расшифрования.
type Cipher struct {
	enc, dec [32]uint32
}

var _ cipher.Block = (*Cipher)(nil)

// New разворачивает ключ: K₁…K₈ — 32-битные слова ключа (big-endian), порядок
// раундов K₁…K₈ трижды и K₈…K₁.
func New(key []byte) (*Cipher, error) {
	if len(key) != KeySize {
		return nil, fmt.Errorf("magma: длина ключа %d байт, нужно %d", len(key), KeySize)
	}
	var k [8]uint32
	for i := range k {
		k[i] = binary.BigEndian.Uint32(key[4*i:])
	}
	c := &Cipher{}
	for i := 0; i < 24; i++ {
		c.enc[i] = k[i%8]
	}
	for i := 0; i < 8; i++ {
		c.enc[24+i] = k[7-i]
	}
	for i := range c.dec {
		c.dec[i] = c.enc[31-i]
	}
	return c, nil
}

func (c *Cipher) BlockSize() int { return BlockSize }

func crypt(dst, src []byte, keys *[32]uint32) {
	if len(src) < BlockSize || len(dst) < BlockSize {
		panic("magma: блок короче 8 байт")
	}
	a1, a0 := binary.BigEndian.Uint32(src[:4]), binary.BigEndian.Uint32(src[4:8])
	// G[k](a₁, a₀) = (a₀, g[k](a₀) ⊕ a₁), последний раунд G* — без перестановки половин
	for i := 0; i < 31; i++ {
		a1, a0 = a0, g(keys[i], a0)^a1
	}
	a1 ^= g(keys[31], a0)
	binary.BigEndian.PutUint32(dst[:4], a1)
	binary.BigEndian.PutUint32(dst[4:8], a0)
}

// Encrypt зашифровывает один 64-битный блок.
func (c *Cipher) Encrypt(dst, src []byte) { crypt(dst, src, &c.enc) }

// Decrypt расшифровывает один 64-битный блок (те же раунды в обратном порядке ключей).
func (c *Cipher) Decrypt(dst, src []byte) { crypt(dst, src, &c.dec) }
