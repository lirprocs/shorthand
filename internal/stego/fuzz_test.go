package stego

// Этап 1 «на потом» (ветка later/stage1): fuzz-тесты. Испорченный или
// подложенный файл не должен ронять программу: только ошибка.
//
//	go test -run=^$ -fuzz=FuzzParseHeader -fuzztime=30s ./internal/stego
//	go test -run=^$ -fuzz=FuzzExtract -fuzztime=30s ./internal/stego

import (
	"bytes"
	"testing"
)

func FuzzParseHeader(f *testing.F) {
	h := Header{Mode: 1, Method: 1, Level: 1, H: 1, W: 4, Length: 100}
	f.Add(h.Marshal())
	f.Add(make([]byte, headerLen))
	f.Fuzz(func(t *testing.T, b []byte) {
		if hdr, err := parseHeader(b); err == nil {
			// разобранный заголовок собирается обратно в те же байты
			if !bytes.Equal(hdr.Marshal(), b) {
				t.Fatalf("заголовок не обратим: %x", b)
			}
		}
	})
}

func FuzzExtract(f *testing.F) {
	key := bytes.Repeat([]byte{0x33}, 32)
	cv := goldenCover(64, 48)
	out, _, err := EmbedCover(cv, "сообщение", Options{Master: key, Nonce: make([]byte, 16)})
	if err != nil {
		f.Fatal(err)
	}
	f.Add(out.Pix[:512], uint16(7))
	f.Add(make([]byte, 64), uint16(0))
	f.Fuzz(func(t *testing.T, data []byte, flip uint16) {
		c := &Cover{W: 64, H: 48, C: 3, Pix: append([]uint8(nil), out.Pix...)}
		// данные «наложить» на изображение и испортить часть элементов
		for i, b := range data {
			c.Pix[(i*7919+int(flip))%len(c.Pix)] ^= b
		}
		ExtractCover(c, "", nil, key) // ошибка допустима, паника — нет
	})
}
