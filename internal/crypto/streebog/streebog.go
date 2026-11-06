// Package streebog реализует хэш-функцию «Стрибог»
// (ГОСТ Р 34.11-2012; английская версия — RFC 6986) с длиной хэш-кода
// 256 и 512 бит и интерфейсом hash.Hash, поэтому подходит для crypto/hmac
// (HMAC_GOSTR3411_2012_256, Р 50.1.113-2016) и crypto/hkdf.
//
// 512-битные векторы хранятся как 64 байта в порядке little-endian: байт 0 —
// младший (a₀). В RFC числа записаны старшим байтом вперёд, поэтому примеры
// из стандарта в тестах переворачиваются.
//
// Преобразования S (подстановка π), P (перестановка τ) и L (умножение на
// матрицу A) объединены в 8 таблиц по 256 слов, как в эталонных реализациях.
package streebog

import (
	"encoding/binary"
	"hash"
	"math/bits"
)

const (
	BlockSize = 64
	Size256   = 32
	Size512   = 64
)

// pi — подстановка π' (та же, что в «Кузнечике»).
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

type vec [8]uint64 // 512 бит: слово k — байты 8k..8k+7 (little-endian)

var (
	// lps[j][x] — вклад байта x, стоящего в строке j после перестановки τ,
	// в слово результата LPS. τ(8k+j) = 8j+k, поэтому байт a[8j+k] попадает
	// в позицию j слова k.
	lps [8][256]uint64
	// cWords — итерационные константы C₁…C₁₂ как векторы.
	cWords [12]vec
)

func init() {
	for j := 0; j < 8; j++ {
		for x := 0; x < 256; x++ {
			v := uint64(pi[x])
			var r uint64
			for t := 0; t < 8; t++ {
				if v>>t&1 != 0 {
					r ^= matA[63-(8*j+t)]
				}
			}
			lps[j][x] = r
		}
	}
	for i := range iterC {
		cWords[i] = load(iterC[i][:])
	}
}

func load(b []byte) vec {
	var v vec
	for k := range v {
		v[k] = binary.LittleEndian.Uint64(b[8*k:])
	}
	return v
}

// transform — LPS(a): out[k] = ⊕ⱼ lps[j][байт k слова j].
func transform(a *vec) vec {
	var out vec
	for k := 0; k < 8; k++ {
		sh := uint(8 * k)
		out[k] = lps[0][byte(a[0]>>sh)] ^ lps[1][byte(a[1]>>sh)] ^
			lps[2][byte(a[2]>>sh)] ^ lps[3][byte(a[3]>>sh)] ^
			lps[4][byte(a[4]>>sh)] ^ lps[5][byte(a[5]>>sh)] ^
			lps[6][byte(a[6]>>sh)] ^ lps[7][byte(a[7]>>sh)]
	}
	return out
}

func xor(a, b *vec) vec {
	var r vec
	for i := range r {
		r[i] = a[i] ^ b[i]
	}
	return r
}

// add512 — сложение по модулю 2⁵¹².
func add512(a, b *vec) vec {
	var r vec
	var carry uint64
	for i := range r {
		r[i], carry = bits.Add64(a[i], b[i], carry)
	}
	return r
}

// gN — функция сжатия: g_N(h, m) = E(LPS(h ⊕ N), m) ⊕ h ⊕ m.
func gN(n, h, m *vec) vec {
	t := xor(h, n)
	k := transform(&t)
	s := *m
	for i := 0; i < 12; i++ {
		t = xor(&s, &k)
		s = transform(&t)
		t = xor(&k, &cWords[i])
		k = transform(&t)
	}
	s = xor(&s, &k)
	s = xor(&s, h)
	return xor(&s, m)
}

type digest struct {
	size     int
	h, n, s  vec // h — состояние, n — счётчик длины, s — контрольная сумма Σ
	buf      [BlockSize]byte
	nbuf     int
	iv       vec
	zero     vec
	blockLen vec
}

// New256 возвращает «Стрибог-256».
func New256() hash.Hash { return newDigest(Size256) }

// New512 возвращает «Стрибог-512».
func New512() hash.Hash { return newDigest(Size512) }

func newDigest(size int) *digest {
	d := &digest{size: size}
	if size == Size256 {
		for i := range d.iv {
			d.iv[i] = 0x0101010101010101
		}
	}
	d.blockLen[0] = 512
	d.Reset()
	return d
}

func (d *digest) Reset() {
	d.h = d.iv
	d.n = vec{}
	d.s = vec{}
	d.nbuf = 0
}

func (d *digest) Size() int      { return d.size }
func (d *digest) BlockSize() int { return BlockSize }

func (d *digest) block(b []byte) {
	m := load(b)
	d.h = gN(&d.n, &d.h, &m)
	d.n = add512(&d.n, &d.blockLen)
	d.s = add512(&d.s, &m)
}

func (d *digest) Write(p []byte) (int, error) {
	total := len(p)
	if d.nbuf > 0 {
		k := copy(d.buf[d.nbuf:], p)
		d.nbuf += k
		p = p[k:]
		if d.nbuf < BlockSize {
			return total, nil
		}
		d.block(d.buf[:])
		d.nbuf = 0
	}
	for len(p) >= BlockSize {
		d.block(p[:BlockSize])
		p = p[BlockSize:]
	}
	d.nbuf = copy(d.buf[:], p)
	return total, nil
}

func (d *digest) Sum(in []byte) []byte {
	c := *d // Sum не меняет состояние
	var last [BlockSize]byte
	copy(last[:], c.buf[:c.nbuf])
	last[c.nbuf] = 1
	m := load(last[:])
	c.h = gN(&c.n, &c.h, &m)
	var bitLen vec
	bitLen[0] = uint64(c.nbuf) * 8
	c.n = add512(&c.n, &bitLen)
	c.s = add512(&c.s, &m)
	c.h = gN(&c.zero, &c.h, &c.n)
	c.h = gN(&c.zero, &c.h, &c.s)
	var out [Size512]byte
	for k := range c.h {
		binary.LittleEndian.PutUint64(out[8*k:], c.h[k])
	}
	if c.size == Size256 {
		return append(in, out[32:]...) // MSB₂₅₆ — старшие 256 бит
	}
	return append(in, out[:]...)
}
