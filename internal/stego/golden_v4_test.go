package stego

import (
	"bytes"
	"image/png"
	"os"
	"testing"

	"github.com/lirprocs/shorthand/internal/alphabet"
	"github.com/lirprocs/shorthand/internal/crypto/suite"
)

// Эталоны нынешнего формата (заголовок версии 4): изображения из testdata
// записаны этой версией программы и должны читаться всегда. Тест ловит любую
// правку, которая молча меняет формат: генератор, перестановку, строки info,
// подматрицы STC, заголовок, код Рида–Соломона, сжатие, алфавит.
//
// Ключ — testMaster, имитозащищаемые данные — «v4». Пересоздать файлы (только
// при намеренной смене формата, со старыми эталонами под другим именем):
//
//	SHORTHAND_WRITE_GOLDEN=1 go test -run TestReadsV4Images ./internal/stego
var goldenV4 = []struct {
	name string
	mode suite.Mode
	lvl  Level
	gray bool
}{
	{"v4_kuznyechik_normal", suite.KuznyechikMGM, LevelNormal, false},
	{"v4_kuznyechik_high", suite.KuznyechikMGM, LevelHigh, false},
	{"v4_kuznyechik_max", suite.KuznyechikMGM, LevelMax, true},
	{"v4_ascon_normal", suite.AsconAEAD128, LevelNormal, true},
	{"v4_ascon_high", suite.AsconAEAD128, LevelHigh, false},
	{"v4_magma_normal", suite.MagmaMGM, LevelNormal, false},
	{"v4_magma_max", suite.MagmaMGM, LevelMax, false},
}

func goldenText(name string) string {
	return "Эталон формата v4: " + name + ". Съешь ещё этих мягких французских булок, да выпей чаю — 1234567890!"
}

func TestReadsV4Images(t *testing.T) {
	for i, g := range goldenV4 {
		path := "testdata/" + g.name + ".png"
		if os.Getenv("SHORTHAND_WRITE_GOLDEN") != "" {
			nonce := bytes.Repeat([]byte{byte(0xa0 + i)}, saltLen)
			out, _, err := Embed(testImage(72, 56, int64(400+i), g.gray), goldenText(g.name),
				Options{Mode: g.mode, Level: g.lvl, Master: testMaster, AAD: []byte("v4"), Nonce: nonce})
			if err != nil {
				t.Fatalf("%s: %v", g.name, err)
			}
			var buf bytes.Buffer
			if err := (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&buf, out); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		img, err := png.Decode(f)
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
		got, err := ExtractCover(FromImage(img), "", []byte("v4"), testMaster)
		if err != nil {
			t.Fatalf("%s: %v", g.name, err)
		}
		if want := alphabet.Normalize(goldenText(g.name)); got.Text != want || got.Mode != g.mode || got.Level != g.lvl {
			t.Fatalf("%s: получено %q, %v, %v", g.name, got.Text, got.Mode, got.Level)
		}
		if got.Corrected != 0 {
			t.Fatalf("%s: эталон прочитан с исправлениями (%d байт)", g.name, got.Corrected)
		}
	}
}

// Заголовок не должен разбираться с паникой ни на каких 26 байтах.
func FuzzParseHeader(f *testing.F) {
	h := Header{Version: formatVersion, Mode: suite.MagmaMGM, Level: LevelHigh, H: 1, W: 4, Length: 100}
	f.Add(h.Marshal())
	f.Add(make([]byte, headerLen))
	f.Add([]byte{1, 2, 3})
	f.Fuzz(func(t *testing.T, b []byte) {
		if h, err := parseHeader(b); err == nil {
			if !bytes.Equal(h.Marshal(), b) {
				t.Fatalf("разобранный заголовок записывается иначе: %x -> %x", b, h.Marshal())
			}
		}
		if len(b) == headerLen {
			_, _ = parseHeaderV3(b)
		}
	})
}

// Извлечение из произвольных пикселей не должно паниковать и запрашивать
// память сверх размера изображения.
func FuzzExtractNoPanic(f *testing.F) {
	f.Add([]byte{0}, uint8(8), uint8(8), false)
	f.Add(bytes.Repeat([]byte{0x55, 0xaa, 0x01}, 100), uint8(20), uint8(15), true)
	f.Fuzz(func(t *testing.T, pix []byte, w, h uint8, gray bool) {
		ww, hh, c := int(w%48)+1, int(h%48)+1, 3
		if gray {
			c = 1
		}
		cv := &Cover{W: ww, H: hh, C: c, Pix: make([]uint8, ww*hh*c)}
		for i := range cv.Pix {
			if len(pix) > 0 {
				cv.Pix[i] = pix[i%len(pix)] + uint8(i/len(pix))
			}
		}
		_, _ = ExtractCover(cv, "", nil, testMaster)
	})
}
