package compress

// Этап 1 «на потом» (ветка later/stage1): fuzz-тесты распаковки и алфавита.
//
//	go test -run=^$ -fuzz=FuzzDecompress -fuzztime=30s ./internal/compress

import (
	"bytes"
	"testing"

	"github.com/lirprocs/shorthand/internal/alphabet"
)

func FuzzDecompress(f *testing.F) {
	for _, s := range samples {
		sym := alphabet.Encode(s)
		for _, m := range []Method{Packed7, Context, Deflate} {
			f.Add(uint8(m), CompressWith(m, sym))
		}
	}
	f.Fuzz(func(t *testing.T, m uint8, data []byte) {
		sym, err := Decompress(Method(m%3), data)
		if err != nil {
			return
		}
		for _, c := range sym {
			if c >= alphabet.Size {
				t.Fatalf("символ %d вне алфавита", c)
			}
		}
		alphabet.Decode(sym) // не должно паниковать
	})
}

// Сжатие и распаковка обратимы для любого текста.
func FuzzRoundTrip(f *testing.F) {
	for _, s := range samples {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		sym := alphabet.Encode(s)
		m, data := Compress(sym)
		back, err := Decompress(m, data)
		if err != nil || !bytes.Equal(back, sym) {
			t.Fatalf("%v: не восстановилось (%v)", m, err)
		}
	})
}
