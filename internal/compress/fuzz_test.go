package compress

import (
	"bytes"
	"testing"
	"unicode/utf8"

	"github.com/lirprocs/shorthand/internal/alphabet"
)

// Любой текст должен пережить цикл алфавит → сжатие (каждым способом) →
// распаковка → алфавит и совпасть с alphabet.Normalize.
func FuzzTextRoundTrip(f *testing.F) {
	for _, s := range []string{
		"", "а", "Привет, мир!", "строка" + string(rune(13)) + string(rune(10)) + "вторая" + string(rune(13)) + "третья", "emoji 🙂 и ґ “кавычки”",
		string([]rune{0, 1}), string([]rune{0x2028, 0xa0, 0xfeff}), "ААААААААААААААААААААААААААААААААААААААААА", "a" + string(rune(9)) + "b" + string(rune(10)) + "c",
		string([]rune{0x10ffff, 0x10000}), "İ ß ǅ", // буквы, у которых ToLower меняет длину или не обратим
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, text string) {
		if !utf8.ValidString(text) || len(text) > 3000 {
			return
		}
		sym := alphabet.Encode(text)
		want, err := alphabet.Decode(sym)
		if err != nil {
			t.Fatalf("алфавит не разобрал собственный вывод: %q", text)
		}
		for _, m := range []Method{Packed7, Context, Deflate} {
			back, err := Decompress(m, CompressWith(m, sym))
			if err != nil {
				t.Fatalf("%v: %v (текст %q)", m, err, text)
			}
			if !bytes.Equal(back, sym) {
				t.Fatalf("%v: символы не совпали (текст %q)", m, text)
			}
		}
		m, c := Compress(sym)
		back, err := Decompress(m, c)
		if err != nil {
			t.Fatal(err)
		}
		if got, err := alphabet.Decode(back); err != nil || got != want {
			t.Fatalf("текст %q: получено %q, %v", text, got, err)
		}
	})
}

// Распаковка произвольных байтов: не паникует, не зависает, а всё, что она
// вернула без ошибки, алфавит либо разбирает, либо отвергает ошибкой.
func FuzzDecompressNoPanic(f *testing.F) {
	f.Add(uint8(0), []byte{0xff, 0xff})
	f.Add(uint8(1), []byte{0x12, 0x34, 0x56, 0x78, 0x9a})
	f.Add(uint8(2), []byte{0x4b, 0x04, 0x00})
	f.Fuzz(func(t *testing.T, m uint8, data []byte) {
		if len(data) > 2000 {
			return
		}
		sym, err := Decompress(Method(m%3), data)
		if err != nil {
			return
		}
		_, _ = alphabet.Decode(sym)
	})
}
