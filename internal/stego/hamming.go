package stego

// Матричное встраивание кодом Хэмминга (R. Crandall, 1998; A. Westfeld, F5, 2001).
//
// p бит сообщения — синдром группы из n = 2^p − 1 элементов:
//
//	s = ⊕ i·yᵢ,  i = 1…n  (yᵢ — младший бит i-го элемента группы),
//
// первый бит сообщения — старший бит s. Чтобы синдром стал равен нужным p
// битам m, достаточно изменить один элемент — с номером d = s ⊕ m. Ту же
// разность даёт любая пара элементов i, j с i ⊕ j = d. Пара выбирается, если
// она по стоимости HILL хотя бы в hamPairGain раз дешевле одиночного
// изменения: так изменения уходят из ровных областей, а их число почти не
// растёт (docs/EXPERIMENTS.md, раздел 7).
//
// Порча одного элемента портит только свои p бит. При p = 4 группа — половина
// байта: байт сообщения зависит от 30 элементов (при чётности групп из
// четырёх — от 32), а изменений нужно в полтора раза меньше.
//
// Если число бит не кратно p, последняя группа короче: r бит на 2^r − 1
// элементов. При p = 1 группа — один элемент: прямая запись бита.
//
// Всё это — часть формата (заголовок версии 5).

const (
	hamCode     = 32 // поле h заголовка: hamCode + p — код Хэмминга с p битами на группу
	hamMaxP     = 8
	hamPairGain = 2
)

// hamElems — сколько элементов нужно на msgBits бит.
func hamElems(msgBits, p int) int {
	n := msgBits / p * (1<<p - 1)
	if tail := msgBits % p; tail > 0 {
		n += 1<<tail - 1
	}
	return n
}

// hamEmbed возвращает y (hamElems(len(msg), p) бит) с синдромами msg.
// x — исходные младшие биты, rho — стоимости их изменения.
func hamEmbed(x []uint8, rho []float64, msg []uint8, p int) []uint8 {
	y := append([]uint8(nil), x[:hamElems(len(msg), p)]...)
	off := 0
	for b := 0; b < len(msg); b += p {
		r := min(p, len(msg)-b)
		n := 1<<r - 1
		s, m := 0, 0
		for i := 1; i <= n; i++ {
			if x[off+i-1]&1 == 1 {
				s ^= i
			}
		}
		for k := 0; k < r; k++ {
			m = m<<1 | int(msg[b+k]&1)
		}
		if d := s ^ m; d != 0 {
			bi, bj, best := d, 0, rho[off+d-1]
			for i := 1; i <= n; i++ {
				j := i ^ d
				if j <= i { // j = 0 при i = d; каждая пара — один раз
					continue
				}
				if c := (rho[off+i-1] + rho[off+j-1]) * hamPairGain; c < best {
					bi, bj, best = i, j, c
				}
			}
			y[off+bi-1] ^= 1
			if bj != 0 {
				y[off+bj-1] ^= 1
			}
		}
		off += n
	}
	return y
}

// hamExtract — синдромы групп.
func hamExtract(y []uint8, msgBits, p int) []uint8 {
	m := make([]uint8, msgBits)
	off := 0
	for b := 0; b < msgBits; b += p {
		r := min(p, msgBits-b)
		n := 1<<r - 1
		s := 0
		for i := 1; i <= n; i++ {
			if y[off+i-1]&1 == 1 {
				s ^= i
			}
		}
		for k := 0; k < r; k++ {
			m[b+k] = uint8(s >> (r - 1 - k) & 1)
		}
		off += n
	}
	return m
}

// hamInfluence — какие биты сообщения зависят от элемента t (для отметки
// стираний): биты его группы.
func hamInfluence(t, msgBits, p int) (first, last int) {
	full := msgBits / p
	if b := t / (1<<p - 1); b < full {
		return b * p, b*p + p - 1
	}
	return full * p, msgBits - 1
}

// coding — способ записи битов в младшие биты элементов: STC с параметрами
// (h, w) или код Хэмминга с ham битами на группу.
type coding struct {
	h, w int
	ham  int
}

// codingOf — способ записи по полям h и w заголовка.
func codingOf(h, w int) coding {
	if h > hamCode {
		return coding{ham: h - hamCode}
	}
	return coding{h: h, w: w}
}

// fields — значения полей h и w заголовка.
func (c coding) fields() (h, w int) {
	if c.ham > 0 {
		return hamCode + c.ham, 1
	}
	return c.h, c.w
}

// elems — сколько элементов занимают bits бит.
func (c coding) elems(bits int) int {
	if c.ham > 0 {
		return hamElems(bits, c.ham)
	}
	return bits * c.w
}

func (c coding) embed(x []uint8, rho []float64, msg []uint8) ([]uint8, error) {
	if c.ham > 0 {
		return hamEmbed(x, rho, msg, c.ham), nil
	}
	y, _, err := stcEmbed(x, rho, msg, c.h, c.w)
	return y, err
}

func (c coding) extract(y []uint8, bits int) []uint8 {
	if c.ham > 0 {
		return hamExtract(y, bits, c.ham)
	}
	return stcExtract(y, bits, c.h, c.w)
}

func (c coding) influence(t, bits int) (first, last int) {
	if c.ham > 0 {
		return hamInfluence(t, bits, c.ham)
	}
	return stcInfluence(t, bits, c.h, c.w)
}
