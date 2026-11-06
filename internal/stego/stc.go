package stego

import (
	"errors"
	"math"
	"runtime"
	"sync"
)

// Синдромно-решётчатые коды (STC; T. Filler, J. Judas, J. Fridrich, 2011).
//
// Сообщение m (длина L бит) — это синдром стегопоследовательности y:
// H·y = m, где y — младшие биты выбранных элементов. Матрица H (L × L·w)
// составлена из одной подматрицы Ĥ (h × w), сдвигаемой на строку вниз и на
// w столбцов вправо. Кодер ищет y с минимальной суммарной стоимостью
// изменений Σ ρᵢ·[xᵢ ≠ yᵢ] алгоритмом Витерби по решётке с 2^h состояниями;
// декодеру стоимости не нужны — он просто считает H·y.
//
// w = 1/α — сколько элементов приходится на бит сообщения; h — высота
// ограничения: больше h — ближе к теоретическому пределу эффективности, но
// медленнее (2^h состояний) и сильнее размножаются ошибки канала (бит
// стегопоследовательности влияет на h соседних битов сообщения).
//
// Длинное сообщение делится на куски, у каждого — свой отрезок контейнера;
// куски кодируются параллельно. Размер куска выбран так, чтобы путь Витерби
// (len·w·2^h бит) занимал не больше 4 МиБ: от 256 до 4096 бит сообщения.

func chunkBitsFor(h, w int) int {
	return max(256, min(4096, (1<<25)/(w<<h)))
}

// hhatColumns — столбцы Ĥ: h-битные числа со старшим и младшим битом 1
// (так решётка всегда проходима), середина — из генератора SplitMix64.
// Для h = 10 вариант генератора для каждого w подобран перебором по
// эффективности (stc_search_test.go, как в работе авторов STC). Всё это —
// часть формата v3.
func hhatColumns(h, w int) []uint32 {
	v := 0
	if h == 10 && w < len(hhatVariant10) {
		v = int(hhatVariant10[w])
	}
	return hhatColumnsVariant(h, w, v)
}

func hhatColumnsVariant(h, w, variant int) []uint32 {
	cols := make([]uint32, w)
	s := uint64(0x5348_4f52_5448_4e44) ^ uint64(h)<<32 ^ uint64(w) ^ uint64(variant)<<48
	for j := range cols {
		// SplitMix64
		s += 0x9e3779b97f4a7c15
		z := s
		z = (z ^ z>>30) * 0xbf58476d1ce4e5b9
		z = (z ^ z>>27) * 0x94d049bb133111eb
		z ^= z >> 31
		c := uint32(z) & (1<<h - 1)
		c |= 1 | 1<<(h-1)
		cols[j] = c
	}
	return cols
}

// hhatVariant10[w] — номер варианта генератора для h = 10 (TestSearchHhat:
// 16 вариантов, лучший по эффективности на случайных данных).
var hhatVariant10 = [65]uint8{
	0, 3, 13, 3, 9, 4, 5, 5, 4, 12, 9, 15, 11,
	1, 4, 11, 1, 0, 5, 11, 1, 1, 8, 3, 11, 10,
	13, 9, 3, 12, 11, 7, 1, 14, 15, 13, 7, 14, 2,
	5, 1, 4, 3, 8, 7, 9, 9, 8, 5, 9, 9, 12,
	9, 14, 3, 1, 14, 8, 14, 0, 13, 14, 7, 15, 1,
}

var ErrSTC = errors.New("stc: встроить нельзя — все варианты требуют изменить недоступные элементы")

type chunk struct {
	msgOff, msgLen int // биты сообщения
	covOff         int // начало отрезка контейнера (длина msgLen·w)
}

func chunksOf(msgLen, h, w int) []chunk {
	cb := chunkBitsFor(h, w)
	var out []chunk
	for off := 0; off < msgLen; off += cb {
		l := min(cb, msgLen-off)
		out = append(out, chunk{off, l, off * w})
	}
	return out
}

// stcEmbed возвращает y (len(msg)·w бит) с H·y = msg и минимальной стоимостью.
// x — исходные младшие биты, rho — стоимости их изменения.
func stcEmbed(x []uint8, rho []float64, msg []uint8, h, w int) ([]uint8, float64, error) {
	if len(x) < len(msg)*w {
		return nil, 0, errors.New("stc: контейнер короче, чем нужно")
	}
	cols := hhatColumns(h, w)
	y := make([]uint8, len(msg)*w)
	chs := chunksOf(len(msg), h, w)
	costs := make([]float64, len(chs))
	errs := make([]error, len(chs))
	var wg sync.WaitGroup
	sem := make(chan struct{}, runtime.NumCPU())
	for i, c := range chs {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, c chunk) {
			defer wg.Done()
			defer func() { <-sem }()
			n := c.msgLen * w
			yy, cost, err := viterbi(x[c.covOff:c.covOff+n], rho[c.covOff:c.covOff+n], msg[c.msgOff:c.msgOff+c.msgLen], h, w, cols)
			copy(y[c.covOff:], yy)
			costs[i], errs[i] = cost, err
		}(i, c)
	}
	wg.Wait()
	total := 0.0
	for i := range chs {
		if errs[i] != nil {
			return nil, 0, errs[i]
		}
		total += costs[i]
	}
	return y, total, nil
}

func viterbi(x []uint8, rho []float64, m []uint8, h, w int, cols []uint32) ([]uint8, float64, error) {
	ns := 1 << h
	words := (ns + 63) / 64
	n := len(m) * w
	inf := math.Inf(1)
	wght := make([]float64, ns)
	nw := make([]float64, ns)
	for i := range wght {
		wght[i] = inf
	}
	wght[0] = 0
	path := make([]uint64, n*words)
	idx := 0
	for i := range m {
		rows := min(h, len(m)-i)
		rmask := uint32(1)<<rows - 1
		for j := 0; j < w; j++ {
			col := int(cols[j] & rmask)
			var c0, c1 float64 // стоимость y = 0 и y = 1
			if x[idx] == 0 {
				c1 = rho[idx]
			} else {
				c0 = rho[idx]
			}
			p := path[idx*words : (idx+1)*words]
			for k := 0; k < ns; k++ {
				w0 := wght[k] + c0
				w1 := wght[k^col] + c1
				if w1 < w0 {
					nw[k] = w1
					p[k>>6] |= 1 << uint(k&63)
				} else {
					nw[k] = w0
				}
			}
			wght, nw = nw, wght
			idx++
		}
		// младший бит состояния — строка i: оставить состояния с битом m[i] и сдвинуть
		half := ns / 2
		for k := 0; k < half; k++ {
			nw[k] = wght[2*k+int(m[i])]
		}
		for k := half; k < ns; k++ {
			nw[k] = inf
		}
		wght, nw = nw, wght
	}
	cost := wght[0]
	if math.IsInf(cost, 1) || math.IsNaN(cost) {
		return nil, 0, ErrSTC
	}
	// обратный проход
	y := make([]uint8, n)
	state := 0
	idx = n - 1
	for i := len(m) - 1; i >= 0; i-- {
		state = (state<<1 | int(m[i])) & (ns - 1)
		rows := min(h, len(m)-i)
		rmask := uint32(1)<<rows - 1
		for j := w - 1; j >= 0; j-- {
			bit := uint8(path[idx*words+state>>6] >> uint(state&63) & 1)
			y[idx] = bit
			if bit == 1 {
				state ^= int(cols[j] & rmask)
			}
			idx--
		}
	}
	return y, cost, nil
}

// stcExtract — H·y.
func stcExtract(y []uint8, msgLen, h, w int) []uint8 {
	cols := hhatColumns(h, w)
	m := make([]uint8, msgLen)
	for _, c := range chunksOf(msgLen, h, w) {
		seg := y[c.covOff : c.covOff+c.msgLen*w]
		mm := m[c.msgOff : c.msgOff+c.msgLen]
		for t, bit := range seg {
			if bit == 0 {
				continue
			}
			i, j := t/w, t%w
			rows := min(h, c.msgLen-i)
			col := cols[j] & (uint32(1)<<rows - 1)
			for b := 0; col != 0; b++ {
				if col&1 == 1 {
					mm[i+b] ^= 1
				}
				col >>= 1
			}
		}
	}
	return m
}

// stcInfluence — какие биты сообщения зависят от элемента t контейнера
// (для отметки стираний): строки i…i+rows−1 куска.
func stcInfluence(t, msgLen, h, w int) (first, last int) {
	cb := chunkBitsFor(h, w)
	c := t / (cb * w)
	off := c * cb
	l := min(cb, msgLen-off)
	local := t - off*w
	i := local / w
	return off + i, off + min(i+h, l) - 1
}
