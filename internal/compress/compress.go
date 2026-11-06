// Package compress сжимает последовательность 7-битных символов алфавита
// (internal/alphabet). Три способа, выбирается самый короткий результат:
//
//	Packed7  — по 7 бит на символ и EOF в конце: выигрыш 12,5 % без накладных
//	           расходов, лучше всего для очень коротких сообщений;
//	Context  — контекстная модель порядков 0–2 со сглаживанием Виттена–Белла
//	           поверх статистики русского корпуса и арифметический кодер:
//	           лучше всего для обычного текста от нескольких слов;
//	Deflate  — LZ77 + Хаффман (RFC 1951) без заголовков gzip: выигрывает на
//	           длинных текстах с повторами.
//
// Номер способа записывается в заголовок (2 бита).
package compress

import (
	"bytes"
	"compress/flate"
	"errors"
	"fmt"
	"io"

	"github.com/lirprocs/shorthand/internal/alphabet"
)

// Method — способ сжатия.
type Method uint8

const (
	Packed7 Method = 0
	Context Method = 1
	Deflate Method = 2
)

func (m Method) String() string {
	switch m {
	case Packed7:
		return "7 бит на символ"
	case Context:
		return "контекстная модель"
	case Deflate:
		return "deflate"
	}
	return fmt.Sprintf("способ %d", uint8(m))
}

var ErrCorrupt = errors.New("compress: повреждённые сжатые данные")

// Compress сжимает символы всеми способами и возвращает самый короткий.
func Compress(sym []uint8) (Method, []byte) {
	best, bestM := packSeven(sym), Packed7
	if c := encodeContext(sym); len(c) < len(best) {
		best, bestM = c, Context
	}
	if d := deflateSymbols(sym); len(d) < len(best) {
		best, bestM = d, Deflate
	}
	return bestM, best
}

// CompressWith сжимает заданным способом (для экспериментов).
func CompressWith(m Method, sym []uint8) []byte {
	switch m {
	case Context:
		return encodeContext(sym)
	case Deflate:
		return deflateSymbols(sym)
	}
	return packSeven(sym)
}

// Decompress восстанавливает символы (без EOF).
func Decompress(m Method, data []byte) ([]uint8, error) {
	switch m {
	case Packed7:
		return unpackSeven(data)
	case Context:
		return decodeContext(data)
	case Deflate:
		return inflateSymbols(data)
	}
	return nil, fmt.Errorf("compress: неизвестный способ %d", m)
}

// ─── Packed7 ────────────────────────────────────────────────────────────────

func packSeven(sym []uint8) []byte {
	out := make([]byte, 0, (len(sym)+1)*7/8+1)
	var acc uint32
	var n uint
	put := func(c uint8) {
		acc = acc<<7 | uint32(c&127)
		n += 7
		for n >= 8 {
			out = append(out, byte(acc>>(n-8)))
			n -= 8
		}
	}
	for _, c := range sym {
		put(c)
	}
	put(alphabet.EOF)
	if n > 0 {
		out = append(out, byte(acc<<(8-n)))
	}
	return out
}

func unpackSeven(data []byte) ([]uint8, error) {
	out := make([]uint8, 0, len(data)*8/7)
	var acc uint32
	var n uint
	raw := 0 // сколько «сырых» символов осталось после ESC: среди них может быть 0
	for _, b := range data {
		acc = acc<<8 | uint32(b)
		n += 8
		for n >= 7 {
			c := uint8(acc>>(n-7)) & 127
			n -= 7
			switch {
			case raw > 0:
				raw--
			case c == alphabet.EOF:
				return out, nil
			case c == alphabet.ESC:
				raw = 3
			}
			out = append(out, c)
		}
	}
	return nil, ErrCorrupt
}

// ─── Deflate ────────────────────────────────────────────────────────────────

func deflateSymbols(sym []uint8) []byte {
	var buf bytes.Buffer
	w, _ := flate.NewWriter(&buf, flate.BestCompression)
	w.Write(sym)
	w.Close()
	return buf.Bytes()
}

func inflateSymbols(data []byte) ([]uint8, error) {
	r := flate.NewReader(bytes.NewReader(data))
	defer r.Close()
	out, err := io.ReadAll(io.LimitReader(r, 64<<20))
	if err != nil {
		return nil, ErrCorrupt
	}
	for _, c := range out {
		if c >= alphabet.Size {
			return nil, ErrCorrupt
		}
	}
	return out, nil
}
