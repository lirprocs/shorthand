package compress

import (
	_ "embed"
	"sync"

	"github.com/lirprocs/shorthand/internal/alphabet"
)

// Контекстная модель и арифметический кодер.
//
// Вероятность следующего символа s оценивается по K предыдущим символам
// (контекст порядка K) с откатом к порядкам K−1, …, 0 — интерполированная
// оценка в духе Виттена–Белла:
//
//	pₖ(s) = (nₖ(s) + e·Tₖ·pₖ₋₁(s)) / (Nₖ + e·Tₖ),   p₋₁(s) = 1/119,
//
// где nₖ(s) — сколько раз s встречался после этих k символов, Nₖ — сумма,
// Tₖ — число разных символов, e — вес «ухода» на младший порядок. Счётчики
// n = (статистика корпуса) + β·(счётчики текущего текста): модель сразу
// «знает» русский язык и быстро подстраивается под сообщение.
//
// Все вычисления целочисленные: кодер и декодер должны получить одинаковые
// частоты на любом процессоре (с плавающей точкой Go вправе объединять
// операции в FMA, и результаты могли бы различаться).
//
// Корпус corpus_ru.txt и параметры ниже — часть формата v3: их изменение
// делает старые сообщения нечитаемыми (корпус защищён тестом с SHA-256).

//go:embed corpus_ru.txt
var corpus string

// Параметры подобраны перебором на тестовых сообщениях (не из корпуса) и на
// тексте записки НИР: K = 6, β = 3, e = 4 — лучший результат на коротких
// сообщениях; длинный текст — 2,65 бит на символ (deflate — 3,3).
const (
	maxOrder    = 6 // K — старший порядок контекста
	adaptWeight = 3 // β — вес счётчиков текущего текста
	escNum      = 4 // e = escNum/escDen — вес ухода на младший порядок
	escDen      = 1
)

const (
	probBits  = 24 // точность вероятностей
	probOne   = 1 << probBits
	freqShift = probBits - 16 // частоты для кодера — 16 бит
	startCtx  = 3             // начальный контекст — «перевод строки»
	numSym    = alphabet.Size
	usableSym = 119 // коды 119…127 зарезервированы и не кодируются
	ordersCap = 6
)

type counts [numSym]uint32

// staticModel — статистика корпуса: ctx[k][последние k символов] → счётчики.
type staticModel struct {
	ctx [ordersCap + 1]map[uint64]*counts
}

var (
	static     *staticModel
	staticOnce sync.Once
)

func ctxKey(hist uint64, k int) uint64 {
	return hist & (uint64(1)<<(7*uint(k)) - 1)
}

func initialHist() uint64 {
	var h uint64
	for i := 0; i < ordersCap; i++ {
		h = h<<7 | startCtx
	}
	return h
}

func bump(m map[uint64]*counts, key uint64, s uint8) {
	t := m[key]
	if t == nil {
		t = new(counts)
		m[key] = t
	}
	t[s]++
}

func getStatic() *staticModel {
	staticOnce.Do(func() {
		m := &staticModel{}
		for k := range m.ctx {
			m.ctx[k] = map[uint64]*counts{}
		}
		hist := initialHist()
		raw := 0
		for _, s := range alphabet.Encode(corpus) {
			if raw > 0 {
				raw--
				continue
			}
			if s == alphabet.ESC {
				raw = 3
			}
			for k := 0; k <= ordersCap; k++ {
				bump(m.ctx[k], ctxKey(hist, k), s)
			}
			hist = (hist<<7 | uint64(s)) & (uint64(1)<<(7*ordersCap) - 1)
		}
		static = m
	})
	return static
}

// model — состояние модели при кодировании одного сообщения.
type model struct {
	st   *staticModel
	ad   [ordersCap + 1]map[uint64]*counts
	hist uint64
	raw  int // сколько «сырых» символов кода Unicode осталось после ESC
	freq [numSym]uint32
	p    [numSym]uint64
}

func newModel() *model {
	m := &model{st: getStatic(), hist: initialHist()}
	for k := range m.ad {
		m.ad[k] = map[uint64]*counts{}
	}
	return m
}

// mix — один порядок: p[s] = (n(s) + e·T·p[s]) / (N + e·T), на месте.
func mix(p *[numSym]uint64, st, ad *counts) {
	var n, t uint64
	var cnt [numSym]uint64
	for s := 0; s < usableSym; s++ {
		var c uint64
		if ad != nil {
			c = uint64(adaptWeight) * uint64(ad[s])
		}
		if st != nil {
			c += uint64(st[s])
		}
		cnt[s] = c
		n += c
		if c > 0 {
			t++
		}
	}
	if n == 0 {
		return
	}
	e := t * uint64(escNum)
	d := n*uint64(escDen) + e
	for s := 0; s < usableSym; s++ {
		p[s] = (cnt[s]*uint64(escDen)*probOne + e*p[s]) / d
	}
}

// update пересчитывает m.freq для текущего контекста.
func (m *model) update() {
	if m.raw > 0 {
		// после ESC — три 7-битные части кода Unicode: равномерно по всем 128
		for s := range m.freq {
			m.freq[s] = 1
		}
		return
	}
	for s := 0; s < numSym; s++ {
		if s < usableSym {
			m.p[s] = probOne / usableSym
		} else {
			m.p[s] = 0
		}
	}
	for k := 0; k <= maxOrder; k++ {
		key := ctxKey(m.hist, k)
		mix(&m.p, m.st.ctx[k][key], m.ad[k][key])
	}
	for s := 0; s < numSym; s++ {
		if s < usableSym {
			m.freq[s] = uint32(m.p[s]>>freqShift) + 1
		} else {
			m.freq[s] = 0
		}
	}
}

func (m *model) learn(s uint8) {
	if m.raw > 0 {
		m.raw-- // части кода Unicode в статистику не попадают
		return
	}
	if s == alphabet.ESC {
		m.raw = 3
	}
	for k := 0; k <= maxOrder; k++ {
		bump(m.ad[k], ctxKey(m.hist, k), s)
	}
	m.hist = (m.hist<<7 | uint64(s)) & (uint64(1)<<(7*ordersCap) - 1)
}

// ─── арифметический кодер (Witten, Neal, Cleary, 1987), 32 бита ─────────────

const (
	acTop     = uint64(1)<<32 - 1
	acHalf    = uint64(1) << 31
	acQuarter = uint64(1) << 30
)

type bitWriter struct {
	out  []byte
	acc  byte
	nacc uint
}

func (w *bitWriter) bit(b uint64) {
	w.acc = w.acc<<1 | byte(b)
	w.nacc++
	if w.nacc == 8 {
		w.out = append(w.out, w.acc)
		w.acc, w.nacc = 0, 0
	}
}

func (w *bitWriter) flush() []byte {
	if w.nacc > 0 {
		w.out = append(w.out, w.acc<<(8-w.nacc))
	}
	return w.out
}

func encodeContext(sym []uint8) []byte {
	m := newModel()
	var w bitWriter
	low, high := uint64(0), acTop
	pending := 0
	emit := func(b uint64) {
		w.bit(b)
		for ; pending > 0; pending-- {
			w.bit(1 - b)
		}
	}
	encodeSym := func(s uint8) {
		m.update()
		var cumLow, total uint64
		for i := 0; i < numSym; i++ {
			if i == int(s) {
				cumLow = total
			}
			total += uint64(m.freq[i])
		}
		cumHigh := cumLow + uint64(m.freq[s])
		rng := high - low + 1
		high = low + rng*cumHigh/total - 1
		low = low + rng*cumLow/total
		for {
			switch {
			case high < acHalf:
				emit(0)
			case low >= acHalf:
				emit(1)
				low -= acHalf
				high -= acHalf
			case low >= acQuarter && high < 3*acQuarter:
				pending++
				low -= acQuarter
				high -= acQuarter
			default:
				m.learn(s)
				return
			}
			low <<= 1
			high = high<<1 | 1
		}
	}
	for _, s := range sym {
		encodeSym(s)
	}
	encodeSym(alphabet.EOF)
	pending++
	if low < acQuarter {
		emit(0)
	} else {
		emit(1)
	}
	return w.flush()
}

func decodeContext(data []byte) ([]uint8, error) {
	m := newModel()
	pos := 0 // номер следующего бита
	nextBit := func() uint64 {
		var b uint64
		if pos < len(data)*8 {
			b = uint64(data[pos>>3]>>(7-uint(pos&7))) & 1
		}
		pos++
		return b
	}
	var value uint64
	for i := 0; i < 32; i++ {
		value = value<<1 | nextBit()
	}
	low, high := uint64(0), acTop
	var out []uint8
	for {
		if pos > len(data)*8+64 {
			return nil, ErrCorrupt // данные кончились, а EOF так и не встретился
		}
		isRaw := m.raw > 0
		m.update()
		var total uint64
		for i := 0; i < numSym; i++ {
			total += uint64(m.freq[i])
		}
		rng := high - low + 1
		target := ((value-low+1)*total - 1) / rng
		var s int
		var cumLow uint64
		for s = 0; s < numSym; s++ {
			if cumLow+uint64(m.freq[s]) > target {
				break
			}
			cumLow += uint64(m.freq[s])
		}
		if s >= numSym || (!isRaw && s >= usableSym) {
			return nil, ErrCorrupt
		}
		cumHigh := cumLow + uint64(m.freq[s])
		high = low + rng*cumHigh/total - 1
		low = low + rng*cumLow/total
	normalize:
		for {
			switch {
			case high < acHalf:
			case low >= acHalf:
				low -= acHalf
				high -= acHalf
				value -= acHalf
			case low >= acQuarter && high < 3*acQuarter:
				low -= acQuarter
				high -= acQuarter
				value -= acQuarter
			default:
				break normalize
			}
			low <<= 1
			high = high<<1 | 1
			value = value<<1 | nextBit()
		}
		if !isRaw && s == alphabet.EOF {
			return out, nil
		}
		out = append(out, uint8(s))
		m.learn(uint8(s))
	}
}
