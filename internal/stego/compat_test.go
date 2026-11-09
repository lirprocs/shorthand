package stego

import (
	"image/png"
	"os"
	"testing"

	"github.com/lirprocs/shorthand/internal/alphabet"
	"github.com/lirprocs/shorthand/internal/crypto/suite"
)

// Изображения, записанные программой с заголовком версии 3 (ветка v3,
// коммит 8e03055), должны читаться и дальше. Файлы в testdata созданы той
// версией: ключ testMaster, имитозащищаемые данные «v3».
func TestReadsV3Images(t *testing.T) {
	for _, g := range []struct {
		name string
		mode suite.Mode
		lvl  Level
	}{
		{"v3_kuznyechik_normal", suite.KuznyechikMGM, LevelNormal},
		{"v3_ascon_high", suite.AsconAEAD128, LevelHigh},
		{"v3_magma_max", suite.MagmaMGM, LevelMax},
	} {
		f, err := os.Open("testdata/" + g.name + ".png")
		if err != nil {
			t.Fatal(err)
		}
		img, err := png.Decode(f)
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
		got, err := ExtractCover(FromImage(img), "", []byte("v3"), testMaster)
		if err != nil {
			t.Fatalf("%s: %v", g.name, err)
		}
		if want := alphabet.Normalize("Эталон формата v3: " + g.name); got.Text != want || got.Mode != g.mode || got.Level != g.lvl {
			t.Fatalf("%s: получено %q, %v, %v", g.name, got.Text, got.Mode, got.Level)
		}
	}
}
