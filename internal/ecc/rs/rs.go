// Package rs — код Рида–Соломона над GF(2⁸) с исправлением ошибок и стираний.
//
// Поле: многочлен x⁸ + x⁴ + x³ + x² + 1 (0x11D), примитивный элемент α = 2.
// Порождающий многочлен g(x) = (x − α⁰)(x − α¹)…(x − α^{r−1}), r — число
// проверочных байтов. Кодирование систематическое: слово = данные || проверка,
// длина слова до 255 (укороченный код для коротких данных).
//
// Декодирование:
//
//  1. синдромы S_j = R(α^j), j = 0…r−1;
//  2. многочлен локаторов стираний Γ(x) = ∏(1 − X_k·x) для известных позиций;
//  3. алгоритм Берлекэмпа–Мэсси, начатый с Γ (Blahut), — локатор ошибок и
//     стираний Λ(x);
//  4. поиск корней Λ (процедура Ченя) — позиции;
//  5. формула Форни — значения: e_k = X_k·Ω(X_k⁻¹) / Λ'(X_k⁻¹), Ω = S·Λ mod x^r.
//
// Исправляется любая комбинация из ν ошибок и ρ стираний при 2ν + ρ ≤ r.
package rs

import (
	"errors"
	"sync"
)

var (
	expTab [512]byte
	logTab [256]byte
)

func init() {
	x := 1
	for i := 0; i < 255; i++ {
		expTab[i] = byte(x)
		logTab[x] = byte(i)
		x <<= 1
		if x&0x100 != 0 {
			x ^= 0x11d
		}
	}
	for i := 255; i < 512; i++ {
		expTab[i] = expTab[i-255]
	}
}

func mul(a, b byte) byte {
	if a == 0 || b == 0 {
		return 0
	}
	return expTab[int(logTab[a])+int(logTab[b])]
}

func div(a, b byte) byte {
	if b == 0 {
		panic("rs: деление на 0")
	}
	if a == 0 {
		return 0
	}
	return expTab[int(logTab[a])+255-int(logTab[b])]
}

func inv(a byte) byte { return div(1, a) }

func pow(e int) byte { // α^e
	e %= 255
	if e < 0 {
		e += 255
	}
	return expTab[e]
}

// Многочлены ниже хранятся младшим коэффициентом вперёд: p[0] — свободный член.

func polyEval(p []byte, x byte) byte {
	var y byte
	for i := len(p) - 1; i >= 0; i-- {
		y = mul(y, x) ^ p[i]
	}
	return y
}

func polyMul(a, b []byte) []byte {
	r := make([]byte, len(a)+len(b)-1)
	for i, ai := range a {
		if ai == 0 {
			continue
		}
		for j, bj := range b {
			r[i+j] ^= mul(ai, bj)
		}
	}
	return r
}

var (
	genMu    sync.Mutex
	genCache = map[int][]byte{}
)

// generator — g(x) = ∏_{i<r}(x − αⁱ), старшим коэффициентом вперёд (для деления).
func generator(r int) []byte {
	genMu.Lock()
	defer genMu.Unlock()
	if g, ok := genCache[r]; ok {
		return g
	}
	g := []byte{1} // младшим вперёд
	for i := 0; i < r; i++ {
		g = polyMul(g, []byte{pow(i), 1})
	}
	// развернуть: старшим вперёд
	out := make([]byte, len(g))
	for i := range g {
		out[i] = g[len(g)-1-i]
	}
	genCache[r] = out
	return out
}

// Encode возвращает кодовое слово data || parity (len(data) + r ≤ 255).
func Encode(data []byte, r int) []byte {
	if len(data)+r > 255 || r < 1 {
		panic("rs: длина слова больше 255")
	}
	g := generator(r)
	// остаток от деления data·x^r на g — синтетическое деление
	rem := make([]byte, r)
	for _, d := range data {
		coef := d ^ rem[0]
		copy(rem, rem[1:])
		rem[r-1] = 0
		if coef != 0 {
			for j := 0; j < r; j++ {
				rem[j] ^= mul(g[j+1], coef)
			}
		}
	}
	out := make([]byte, len(data)+r)
	copy(out, data)
	copy(out[len(data):], rem)
	return out
}

var ErrTooMany = errors.New("rs: слишком много ошибок, исправить нельзя")

// Слово cw длины n: cw[i] — коэффициент при x^{n−1−i}; позиции i ↔ X = α^{n−1−i}.

func syndromes(cw []byte, r int) ([]byte, bool) {
	n := len(cw)
	s := make([]byte, r)
	nonzero := false
	for j := 0; j < r; j++ {
		x := pow(j)
		var y byte
		for i := 0; i < n; i++ {
			y = mul(y, x) ^ cw[i]
		}
		s[j] = y
		if y != 0 {
			nonzero = true
		}
	}
	return s, nonzero
}

// Decode исправляет слово на месте. erasures — известные позиции искажений
// (индексы в cw). Возвращает число исправленных байтов. Если исправить нельзя,
// слово не меняется.
func Decode(cw []byte, r int, erasures []int) (int, error) {
	work := append([]byte(nil), cw...)
	n, err := decode(work, r, erasures)
	if err == nil {
		copy(cw, work)
	}
	return n, err
}

func decode(cw []byte, r int, erasures []int) (int, error) {
	n := len(cw)
	if n > 255 || n <= r {
		return 0, errors.New("rs: неверная длина слова")
	}
	s, bad := syndromes(cw, r)
	if !bad {
		return 0, nil
	}
	if len(erasures) > r {
		return 0, ErrTooMany
	}
	// Γ(x) = ∏(1 − X_k x), X_k = α^{n−1−i}
	gamma := []byte{1}
	seen := map[int]bool{}
	for _, i := range erasures {
		if i < 0 || i >= n || seen[i] {
			continue
		}
		seen[i] = true
		gamma = polyMul(gamma, []byte{1, pow(n - 1 - i)})
	}
	rho := len(gamma) - 1

	// Берлекэмп–Мэсси с начальным Γ
	lambda := append([]byte(nil), gamma...)
	b := append([]byte(nil), gamma...)
	L := rho
	for k := rho + 1; k <= r; k++ {
		var delta byte
		for j := 0; j < len(lambda) && j <= k-1; j++ {
			delta ^= mul(lambda[j], s[k-1-j])
		}
		xb := append([]byte{0}, b...) // x·B(x)
		if delta == 0 {
			b = xb
			continue
		}
		t := make([]byte, max(len(lambda), len(xb)))
		copy(t, lambda)
		for j, c := range xb {
			t[j] ^= mul(delta, c)
		}
		if 2*L <= k-1+rho {
			L = k + rho - L
			dInv := inv(delta)
			b = make([]byte, len(lambda))
			for j, c := range lambda {
				b[j] = mul(c, dInv)
			}
		} else {
			b = xb
		}
		lambda = t
	}
	for len(lambda) > 1 && lambda[len(lambda)-1] == 0 {
		lambda = lambda[:len(lambda)-1]
	}
	deg := len(lambda) - 1
	if deg != L || 2*(L-rho)+rho > r {
		return 0, ErrTooMany
	}

	// поиск корней Λ(X⁻¹) = 0 по всем позициям слова
	var pos []int
	for i := 0; i < n; i++ {
		xInv := pow(-(n - 1 - i))
		if polyEval(lambda, xInv) == 0 {
			pos = append(pos, i)
		}
	}
	if len(pos) != deg {
		return 0, ErrTooMany
	}

	// Ω(x) = S(x)·Λ(x) mod x^r
	omega := polyMul(s, lambda)
	if len(omega) > r {
		omega = omega[:r]
	}
	// Λ'(x): в характеристике 2 остаются нечётные степени
	dl := make([]byte, len(lambda))
	for j := 1; j < len(lambda); j += 2 {
		dl[j-1] = lambda[j]
	}
	for _, i := range pos {
		x := pow(n - 1 - i)
		xInv := inv(x)
		den := polyEval(dl, xInv)
		if den == 0 {
			return 0, ErrTooMany
		}
		cw[i] ^= mul(x, div(polyEval(omega, xInv), den))
	}
	if _, still := syndromes(cw, r); still {
		return 0, ErrTooMany
	}
	return len(pos), nil
}

// ─── поток: несколько блоков с перемежением ─────────────────────────────────

// layout — размеры данных в блоках: длины различаются не больше чем на 1.
func layout(dataLen, r int) []int {
	k := 255 - r
	nb := (dataLen + k - 1) / k
	if nb == 0 {
		nb = 1
	}
	sizes := make([]int, nb)
	base, extra := dataLen/nb, dataLen%nb
	for i := range sizes {
		sizes[i] = base
		if i < extra {
			sizes[i]++
		}
	}
	return sizes
}

// CodedLen — длина закодированного потока для dataLen байт данных.
func CodedLen(dataLen, r int) int {
	return dataLen + len(layout(dataLen, r))*r
}

// interleave — номер блока и позиция в нём для каждого байта потока.
func interleave(sizes []int, r int) (blk, idx []int) {
	maxLen := 0
	for _, s := range sizes {
		maxLen = max(maxLen, s+r)
	}
	for j := 0; j < maxLen; j++ {
		for b, s := range sizes {
			if j < s+r {
				blk = append(blk, b)
				idx = append(idx, j)
			}
		}
	}
	return
}

// EncodeStream кодирует данные блоками по ≤255 байт и перемежает их байт за
// байтом: соседние байты потока попадают в разные блоки, поэтому локальное
// повреждение распределяется между блоками.
func EncodeStream(data []byte, r int) []byte {
	sizes := layout(len(data), r)
	words := make([][]byte, len(sizes))
	off := 0
	for b, s := range sizes {
		words[b] = Encode(data[off:off+s], r)
		off += s
	}
	blk, idx := interleave(sizes, r)
	out := make([]byte, len(blk))
	for p := range out {
		out[p] = words[blk[p]][idx[p]]
	}
	return out
}

// Stats — итог декодирования потока.
type Stats struct {
	Blocks    int // блоков
	Corrected int // исправлено байтов
}

// DecodeStream восстанавливает dataLen байт данных. erased[p] — байт p потока
// подозрителен (стирание); может быть nil.
func DecodeStream(coded []byte, dataLen, r int, erased []bool) ([]byte, Stats, error) {
	sizes := layout(dataLen, r)
	if len(coded) != CodedLen(dataLen, r) {
		return nil, Stats{}, errors.New("rs: неверная длина потока")
	}
	blk, idx := interleave(sizes, r)
	words := make([][]byte, len(sizes))
	era := make([][]int, len(sizes))
	for b, s := range sizes {
		words[b] = make([]byte, s+r)
	}
	for p := range coded {
		words[blk[p]][idx[p]] = coded[p]
		if erased != nil && erased[p] {
			era[blk[p]] = append(era[blk[p]], idx[p])
		}
	}
	st := Stats{Blocks: len(sizes)}
	out := make([]byte, 0, dataLen)
	for b, w := range words {
		e := era[b]
		if len(e) > r {
			e = nil // стираний больше, чем проверочных байтов: пробуем без них
		}
		n, err := Decode(w, r, e)
		if err != nil && e != nil {
			n, err = Decode(w, r, nil)
		}
		if err != nil {
			return nil, st, err
		}
		st.Corrected += n
		out = append(out, w[:sizes[b]]...)
	}
	return out, st, nil
}
