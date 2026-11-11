package rs

import (
	"bytes"
	"testing"
)

// Декодер на произвольном слове: не паникует; если сообщил об успехе —
// результат обязан быть кодовым словом, а при ошибке слово не меняется.
func FuzzDecode(f *testing.F) {
	f.Add(Encode([]byte("кодовое слово"), 16), uint8(16), []byte{3})
	f.Add(bytes.Repeat([]byte{0xff}, 60), uint8(32), []byte{})
	f.Add([]byte{1, 2, 3}, uint8(2), []byte{0, 0, 200})
	f.Fuzz(func(t *testing.T, cw []byte, r uint8, era []byte) {
		rr := int(r%128) + 1
		if len(cw) > 255 {
			cw = cw[:255]
		}
		erasures := make([]int, len(era))
		for i, e := range era {
			erasures[i] = int(e) - 3 // в том числе отрицательные и за концом слова
		}
		before := append([]byte(nil), cw...)
		n, err := Decode(cw, rr, erasures)
		if err != nil {
			if !bytes.Equal(cw, before) {
				t.Fatal("слово изменено при ошибке декодирования")
			}
			return
		}
		if _, bad := syndromes(cw, rr); bad {
			t.Fatal("декодер сообщил об успехе, но результат — не кодовое слово")
		}
		diff := 0
		for i := range cw {
			if cw[i] != before[i] {
				diff++
			}
		}
		if diff > n {
			t.Fatalf("изменено %d байт, сообщено %d", diff, n)
		}
	})
}

// Поток: кодирование, порча не больше исправляемого, декодирование.
func FuzzStreamRoundTrip(f *testing.F) {
	f.Add([]byte("данные потока"), uint8(1), uint16(0), uint8(0))
	f.Add(bytes.Repeat([]byte{7}, 700), uint8(0), uint16(300), uint8(9))
	f.Fuzz(func(t *testing.T, data []byte, lvl uint8, pos uint16, val uint8) {
		if len(data) == 0 || len(data) > 4000 {
			return
		}
		r := []int{32, 64, 128}[lvl%3]
		coded := EncodeStream(data, r)
		if len(coded) != CodedLen(len(data), r) {
			t.Fatalf("длина потока %d, CodedLen %d", len(coded), CodedLen(len(data), r))
		}
		// пачка из r/2 подряд идущих байтов: перемежение разносит её по блокам
		start := int(pos) % len(coded)
		for i := 0; i < r/2 && start+i < len(coded); i++ {
			coded[start+i] ^= val | 1
		}
		got, _, err := DecodeStream(coded, len(data), r, nil)
		if err != nil || !bytes.Equal(got, data) {
			t.Fatalf("r=%d, длина %d, пачка с %d: %v", r, len(data), start, err)
		}
	})
}

// DecodeStream на произвольном потоке и произвольной длине не паникует.
func FuzzDecodeStreamNoPanic(f *testing.F) {
	f.Add([]byte{1, 2, 3, 4}, uint16(1), uint8(0))
	f.Fuzz(func(t *testing.T, coded []byte, dataLen uint16, lvl uint8) {
		r := []int{32, 64, 128}[lvl%3]
		n := int(dataLen)%2000 + 1
		if len(coded) > CodedLen(n, r) {
			coded = coded[:CodedLen(n, r)]
		}
		_, _, _ = DecodeStream(coded, n, r, nil)
	})
}
