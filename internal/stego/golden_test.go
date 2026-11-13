package stego

import (
	"bytes"
	"fmt"
	"image/png"
	"os"
	"testing"

	"github.com/lirprocs/shorthand/internal/alphabet"
	"github.com/lirprocs/shorthand/internal/crypto/suite"
)

// Эталоны формата: изображения из testdata записаны программой и должны
// читаться всегда. Тест ловит любую правку, которая молча меняет формат:
// генератор, перестановку, строки info, подматрицы STC, код Хэмминга,
// заголовок, код Рида–Соломона, сжатие, алфавит.
//
//   - v5_*.png — нынешний заголовок версии 5 (ветка v3.2);
//   - v4_*.png — заголовок версии 4 (ветка v3.1, master до v3.2): только чтение,
//     записаны сборкой коммита cb1eea6;
//   - v3_*.png — заголовок версии 3: compat_test.go.
//
// Ключ — testMaster, имитозащищаемые данные — «v4» и «v5». Пересоздать эталоны
// версии 5 (только при намеренной смене формата; прежние эталоны остаются под
// своими именами, а читать их должен код совместимости):
//
//	SHORTHAND_WRITE_GOLDEN=1 go test -run TestReadsV5Images ./internal/stego
type golden struct {
	name string
	mode suite.Mode
	lvl  Level
	gray bool
}

var goldenV4 = []golden{
	{"v4_kuznyechik_normal", suite.KuznyechikMGM, LevelNormal, false},
	{"v4_kuznyechik_high", suite.KuznyechikMGM, LevelHigh, false},
	{"v4_kuznyechik_max", suite.KuznyechikMGM, LevelMax, true},
	{"v4_ascon_normal", suite.AsconAEAD128, LevelNormal, true},
	{"v4_ascon_high", suite.AsconAEAD128, LevelHigh, false},
	{"v4_magma_normal", suite.MagmaMGM, LevelNormal, false},
	{"v4_magma_max", suite.MagmaMGM, LevelMax, false},
}

var goldenV5 = []golden{
	{"v5_kuznyechik_normal", suite.KuznyechikMGM, LevelNormal, false},
	{"v5_kuznyechik_high", suite.KuznyechikMGM, LevelHigh, false},
	{"v5_kuznyechik_max", suite.KuznyechikMGM, LevelMax, true},
	{"v5_ascon_normal", suite.AsconAEAD128, LevelNormal, true},
	{"v5_ascon_high", suite.AsconAEAD128, LevelHigh, true},
	{"v5_magma_normal", suite.MagmaMGM, LevelNormal, false},
	{"v5_magma_high", suite.MagmaMGM, LevelHigh, false},
	{"v5_magma_max", suite.MagmaMGM, LevelMax, false},
}

func goldenText(version int, name string) string {
	return fmt.Sprintf("Эталон формата v%d: %s. Съешь ещё этих мягких французских булок, да выпей чаю — 1234567890!", version, name)
}

func readGolden(t *testing.T, g golden, version int) {
	t.Helper()
	f, err := os.Open("testdata/" + g.name + ".png")
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(f)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	got, err := ExtractCover(FromImage(img), "", []byte(fmt.Sprintf("v%d", version)), testMaster)
	if err != nil {
		t.Fatalf("%s: %v", g.name, err)
	}
	if want := alphabet.Normalize(goldenText(version, g.name)); got.Text != want || got.Mode != g.mode || got.Level != g.lvl || got.Version != version {
		t.Fatalf("%s: получено %q, %v, %v, версия %d", g.name, got.Text, got.Mode, got.Level, got.Version)
	}
	if got.Corrected != 0 {
		t.Fatalf("%s: эталон прочитан с исправлениями (%d байт)", g.name, got.Corrected)
	}
}

func TestReadsV4Images(t *testing.T) {
	for _, g := range goldenV4 {
		readGolden(t, g, 4)
	}
}

func TestReadsV5Images(t *testing.T) {
	for i, g := range goldenV5 {
		if os.Getenv("SHORTHAND_WRITE_GOLDEN") != "" {
			nonce := bytes.Repeat([]byte{byte(0xb0 + i)}, saltLen)
			out, _, err := Embed(testImage(72, 56, int64(500+i), g.gray), goldenText(5, g.name),
				Options{Mode: g.mode, Level: g.lvl, Master: testMaster, AAD: []byte("v5"), Nonce: nonce})
			if err != nil {
				t.Fatalf("%s: %v", g.name, err)
			}
			var buf bytes.Buffer
			if err := (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&buf, out); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile("testdata/"+g.name+".png", buf.Bytes(), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		readGolden(t, g, 5)
	}
}

// Заголовок не должен разбираться с паникой ни на каких 26 байтах.
func FuzzParseHeader(f *testing.F) {
	h := Header{Version: formatVersion, Mode: suite.MagmaMGM, Level: LevelHigh, H: hamCode + 4, W: 1, Length: 100}
	f.Add(h.Marshal())
	old := Header{Version: 4, Mode: suite.KuznyechikMGM, Level: LevelNormal, H: 10, W: 64, Length: 7}
	f.Add(old.Marshal())
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
