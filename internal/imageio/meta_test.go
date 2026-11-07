package imageio

import (
	"bytes"
	"image"
	"image/png"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func randNRGBA(w, h int, opaque bool, seed int64) *image.NRGBA {
	rng := rand.New(rand.NewSource(seed))
	im := image.NewNRGBA(image.Rect(0, 0, w, h))
	rng.Read(im.Pix)
	for i := 3; i < len(im.Pix); i += 4 {
		if opaque {
			im.Pix[i] = 255
		}
	}
	return im
}

// pngWithChunks собирает PNG стандартным кодировщиком и вставляет блоки.
func pngWithChunks(t *testing.T, img image.Image, before, after []Chunk) []byte {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	d := buf.Bytes()
	// после IHDR (8 + 25 байт) — блоки «до», перед IEND — «после»
	var out bytes.Buffer
	out.Write(d[:33])
	for _, c := range before {
		writeChunk(&out, c.Type, c.Data)
	}
	out.Write(d[33 : len(d)-12])
	for _, c := range after {
		writeChunk(&out, c.Type, c.Data)
	}
	out.Write(d[len(d)-12:])
	return out.Bytes()
}

func samePixels(t *testing.T, a, b image.Image) {
	t.Helper()
	bb := a.Bounds()
	for y := bb.Min.Y; y < bb.Max.Y; y++ {
		for x := bb.Min.X; x < bb.Max.X; x++ {
			r1, g1, b1, a1 := a.At(x, y).RGBA()
			r2, g2, b2, a2 := b.At(x, y).RGBA()
			if r1 != r2 || g1 != g2 || b1 != b2 || a1 != a2 {
				t.Fatalf("пиксель (%d,%d) отличается", x, y)
			}
		}
	}
}

func TestPNGKeepsTypeAndChunks(t *testing.T) {
	dir := t.TempDir()
	before := []Chunk{
		{Type: "gAMA", Data: []byte{0, 0, 0xb1, 0x8f}},
		{Type: "pHYs", Data: []byte{0, 0, 0x0e, 0xc4, 0, 0, 0x0e, 0xc4, 1}},
		{Type: "tEXt", Data: []byte("Software\x00Камера")},
		{Type: "vpAg", Data: []byte{1, 2}}, // неизвестный, safe-to-copy (4-я буква строчная)
		{Type: "prVW", Data: []byte{3}},    // неизвестный, unsafe-to-copy — не переносится
	}
	after := []Chunk{{Type: "tIME", Data: []byte{0x07, 0xea, 11, 7, 12, 0, 0}}}
	for _, opaque := range []bool{true, false} {
		src := filepath.Join(dir, "src.png")
		img := randNRGBA(37, 23, opaque, 1)
		os.WriteFile(src, pngWithChunks(t, img, before, after), 0o644)
		old := time.Date(2025, 5, 1, 10, 0, 0, 0, time.Local)
		os.Chtimes(src, old, old)

		m, err := Inspect(src)
		if err != nil {
			t.Fatal(err)
		}
		wantType := uint8(2)
		if !opaque {
			wantType = 6
		}
		if m.ColorType != wantType || m.BitDepth != 8 || len(m.Chunks) != 6 {
			t.Fatalf("Inspect: тип %d, разрядность %d, блоков %d", m.ColorType, m.BitDepth, len(m.Chunks))
		}
		dst := filepath.Join(dir, "dst.png")
		if err := SaveLike(dst, img, PNG, m, true); err != nil {
			t.Fatal(err)
		}
		m2, _ := Inspect(dst)
		if m2.ColorType != m.ColorType {
			t.Fatalf("тип цвета %d → %d", m.ColorType, m2.ColorType)
		}
		var types []string
		for _, c := range m2.Chunks {
			types = append(types, c.Type)
		}
		want := []string{"gAMA", "pHYs", "tEXt", "vpAg", "tIME"}
		if len(types) != len(want) {
			t.Fatalf("блоки %v, ожидались %v", types, want)
		}
		for i := range want {
			if types[i] != want[i] {
				t.Fatalf("блоки %v, ожидались %v", types, want)
			}
		}
		if m2.Chunks[len(m2.Chunks)-1].BeforeIDAT {
			t.Fatal("tIME должен остаться после данных")
		}
		if !m2.ModTime.Equal(old) {
			t.Fatalf("дата изменения %v, ожидалась %v", m2.ModTime, old)
		}
		back, _, err := Load(dst)
		if err != nil {
			t.Fatal(err)
		}
		samePixels(t, img, back)
	}
}

// RGBA-исходник с полностью непрозрачными пикселями остаётся RGBA (стандартный
// кодировщик Go сохранил бы его как RGB).
func TestOpaqueRGBAStaysRGBA(t *testing.T) {
	dir := t.TempDir()
	img := randNRGBA(16, 16, true, 2)
	m := &Meta{Format: PNG, ColorType: 6, BitDepth: 8}
	dst := filepath.Join(dir, "o.png")
	if err := SaveLike(dst, img, PNG, m, false); err != nil {
		t.Fatal(err)
	}
	m2, _ := Inspect(dst)
	if m2.ColorType != 6 {
		t.Fatalf("тип цвета %d", m2.ColorType)
	}
}

func TestPNGGray(t *testing.T) {
	dir := t.TempDir()
	g := image.NewGray(image.Rect(0, 0, 33, 17))
	rand.New(rand.NewSource(3)).Read(g.Pix)
	dst := filepath.Join(dir, "g.png")
	if err := SaveLike(dst, g, PNG, &Meta{Format: PNG, ColorType: 0, BitDepth: 8}, false); err != nil {
		t.Fatal(err)
	}
	back, _, _ := Load(dst)
	samePixels(t, g, back)
}

func TestBMPKeepsBitsAndResolution(t *testing.T) {
	dir := t.TempDir()
	img := randNRGBA(21, 9, true, 4)
	for _, c := range []struct {
		bits    int
		topDown bool
	}{{24, false}, {32, false}, {24, true}} {
		m := &Meta{Format: BMP, BMPBits: c.bits, XPPM: 3780, YPPM: 3780, TopDown: c.topDown}
		dst := filepath.Join(dir, "b.bmp")
		if err := SaveLike(dst, img, BMP, m, false); err != nil {
			t.Fatal(err)
		}
		m2, err := Inspect(dst)
		if err != nil {
			t.Fatal(err)
		}
		if m2.BMPBits != c.bits || m2.XPPM != 3780 || m2.TopDown != c.topDown {
			t.Fatalf("%+v → %+v", m, m2)
		}
		back, _, err := Load(dst)
		if err != nil {
			t.Fatal(err)
		}
		samePixels(t, img, back)
	}
}

func TestChanges(t *testing.T) {
	m := &Meta{Format: PNG, ColorType: 3, BitDepth: 16, Interlaced: true}
	if len(m.Changes()) != 3 {
		t.Fatal(m.Changes())
	}
	if len((&Meta{Format: PNG, ColorType: 2, BitDepth: 8}).Changes()) != 0 {
		t.Fatal("лишние предупреждения")
	}
}
