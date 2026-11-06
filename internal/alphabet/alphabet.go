// Package alphabet переводит текст в последовательность 7-битных символов
// (коды 0…127) и обратно. Регистр не сохраняется: прописные буквы заменяются
// строчными — по решению автора, смысл текста от этого не теряется, а
// алфавит вдвое меньше.
//
// Таблица (часть формата v3 — менять нельзя):
//
//	0       EOF — конец текста
//	1       ESC — следующий символ вне таблицы: три 7-битных символа кода Unicode
//	2–4     пробел, перевод строки, табуляция
//	5–37    а б в г д е ё ж з и й к л м н о п р с т у ф х ц ч ш щ ъ ы ь э ю я
//	38–63   a … z
//	64–73   0 … 9
//	74–105  ! " # $ % & ' ( ) * + , - . / : ; < = > ? @ [ \ ] ^ _ ` { | } ~
//	106–114 № — – « » … ₽ ° §
//	115–118 і ї є ў
//	119–127 резерв
//
// Символы вне таблицы (эмодзи, ґ, “ ” …) не теряются: ESC и 21 бит кода.
// Перевод строки \r\n и одиночный \r становятся \n, неразрывный пробел — пробелом.
package alphabet

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	EOF      = 0
	ESC      = 1
	Size     = 128
	firstRes = 119 // первый резервный код
)

// table — символы с кодами 2…118 по порядку.
var table = []rune(" \n\t" +
	"абвгдеёжзийклмнопрстуфхцчшщъыьэюя" +
	"abcdefghijklmnopqrstuvwxyz" +
	"0123456789" +
	"!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~" +
	"№—–«»…₽°§" +
	"іїєў")

var code = map[rune]uint8{}

func init() {
	if len(table) != firstRes-2 {
		panic(fmt.Sprintf("alphabet: в таблице %d символов, ожидалось %d", len(table), firstRes-2))
	}
	for i, r := range table {
		if _, dup := code[r]; dup {
			panic(fmt.Sprintf("alphabet: символ %q повторяется", r))
		}
		code[r] = uint8(i + 2)
	}
}

// Encode переводит текст в символы (без завершающего EOF).
func Encode(text string) []uint8 {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	out := make([]uint8, 0, len(text))
	for _, r := range text {
		switch r {
		case '\r':
			r = '\n'
		case ' ':
			r = ' '
		}
		r = unicode.ToLower(r)
		if c, ok := code[r]; ok {
			out = append(out, c)
			continue
		}
		out = append(out, ESC, uint8(r>>14)&127, uint8(r>>7)&127, uint8(r)&127)
	}
	return out
}

var ErrCorrupt = errors.New("alphabet: повреждённая последовательность символов")

// Decode переводит символы обратно в текст. EOF, если встретится, завершает текст.
func Decode(sym []uint8) (string, error) {
	var b strings.Builder
	for i := 0; i < len(sym); i++ {
		c := sym[i]
		switch {
		case c == EOF:
			return b.String(), nil
		case c == ESC:
			if i+3 >= len(sym) {
				return "", ErrCorrupt
			}
			r := rune(sym[i+1]&127)<<14 | rune(sym[i+2]&127)<<7 | rune(sym[i+3]&127)
			if !utf8.ValidRune(r) {
				return "", ErrCorrupt
			}
			b.WriteRune(r)
			i += 3
		case c >= firstRes:
			return "", ErrCorrupt
		default:
			b.WriteRune(table[c-2])
		}
	}
	return b.String(), nil
}

// Normalize показывает, во что превратится текст после Encode/Decode
// (строчные буквы, \n вместо \r\n).
func Normalize(text string) string {
	s, _ := Decode(Encode(text))
	return s
}
