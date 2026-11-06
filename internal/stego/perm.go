package stego

import "github.com/lirprocs/shorthand/internal/crypto/prng"

// Perm — ключевая псевдослучайная перестановка чисел 0…n−1 без хранения
// массива: сбалансированная сеть Фейстеля на 2h бит (2^{2h} ≥ n) со случайными
// функциями раундов, заданными таблицами из генератора режима (конструкция
// Луби–Ракоффа), и «прогулка по циклу»: если результат ≥ n, перестановка
// применяется снова. Позиция j-го используемого элемента — At(j).
//
// Память — 6 таблиц по 2^h слов (не больше 1,5 МиБ при n ≤ 2³²) вместо
// массива на n элементов; вычисление At — шесть обращений к таблицам.
type Perm struct {
	n      uint64
	h      uint
	mask   uint64
	tables [permRounds][]uint32
}

const permRounds = 6

// NewPerm строит перестановку из генератора r.
func NewPerm(r *prng.Rand, n int) *Perm {
	p := &Perm{n: uint64(n)}
	h := uint(1)
	for uint64(1)<<(2*h) < p.n {
		h++
	}
	p.h = h
	p.mask = uint64(1)<<h - 1
	for i := range p.tables {
		t := make([]uint32, 1<<h)
		for j := range t {
			t[j] = uint32(r.Uint64() & p.mask)
		}
		p.tables[i] = t
	}
	return p
}

func (p *Perm) feistel(x uint64) uint64 {
	l, r := x>>p.h, x&p.mask
	for i := 0; i < permRounds; i++ {
		l, r = r, l^uint64(p.tables[i][r])
	}
	return l<<p.h | r
}

// At — образ числа j (0 ≤ j < n).
func (p *Perm) At(j int) int {
	x := p.feistel(uint64(j))
	for x >= p.n {
		x = p.feistel(x)
	}
	return int(x)
}
