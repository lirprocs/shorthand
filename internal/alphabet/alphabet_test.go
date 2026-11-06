package alphabet

import (
	"strings"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	cases := map[string]string{
		"Привет, Мир!":      "привет, мир!",
		"ЁЛКА ёлка":         "ёлка ёлка",
		"Hello, World 2026": "hello, world 2026",
		"a\r\nb\rc":         "a\nb\nc",
		"№ 5 — «цитата» – … ₽ ° §":                        "№ 5 — «цитата» – … ₽ ° §",
		"Їжак і Єва, Ўладзімір":                           "їжак і єва, ўладзімір",
		"эмодзи 😀 и ґ и “кавычки”":                        "эмодзи 😀 и ґ и “кавычки”",
		"неразрывный пробел":                              "неразрывный пробел",
		"все знаки: !\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~\t": "все знаки: !\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~\t",
		"": "",
	}
	for in, want := range cases {
		sym := Encode(in)
		for _, c := range sym {
			if c >= Size {
				t.Fatalf("%q: код %d вне 7 бит", in, c)
			}
		}
		got, err := Decode(sym)
		if err != nil || got != want {
			t.Fatalf("%q -> %q (err %v), want %q", in, got, err, want)
		}
	}
}

func TestOneSymbolPerCommonChar(t *testing.T) {
	text := "съешь же ещё этих мягких французских булок, да выпей чаю. the quick brown fox 0123456789"
	if n := len(Encode(text)); n != len([]rune(text)) {
		t.Fatalf("%d символов вместо %d", n, len([]rune(text)))
	}
}

func TestCorrupt(t *testing.T) {
	for _, bad := range [][]uint8{{ESC, 1}, {120}, {127}, {ESC, 127, 127, 127}} {
		if _, err := Decode(bad); err == nil {
			t.Fatalf("%v: ошибка не обнаружена", bad)
		}
	}
	if s, err := Decode([]uint8{5, EOF, 6}); err != nil || s != "а" {
		t.Fatalf("EOF не завершил текст: %q %v", s, err)
	}
}

func TestTableSize(t *testing.T) {
	if len(table) != 117 || strings.ContainsRune(string(table), 'Ё') {
		t.Fatal("таблица изменилась — это меняет формат")
	}
}
