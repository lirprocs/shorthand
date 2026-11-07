package precheck

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"math"
	"math/rand"
	"testing"

	"github.com/lirprocs/shorthand/internal/stego"
)

// photo — «похожее на фото» изображение: плавный фон и текстура.
func photo(w, h int, seed int64, noise float64) *image.NRGBA {
	rng := rand.New(rand.NewSource(seed))
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var v [3]float64
			for c := range v {
				v[c] = 120 + 50*math.Sin(float64(x)/23+float64(c)) + 40*math.Cos(float64(y)/31) + rng.NormFloat64()*noise
				v[c] = math.Max(0, math.Min(255, v[c]))
			}
			img.Set(x, y, color.NRGBA{uint8(v[0]), uint8(v[1]), uint8(v[2]), 255})
		}
	}
	return img
}

func viaJPEG(t *testing.T, img image.Image, q int) image.Image {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: q}); err != nil {
		t.Fatal(err)
	}
	out, err := jpeg.Decode(&buf)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestJPEGTraces(t *testing.T) {
	src := photo(512, 384, 1, 10)
	if s, _, _ := JPEGScore(stego.FromImage(src)); s >= JPEGThreshold {
		t.Fatalf("исходник без JPEG: оценка %.3f", s)
	}
	for _, q := range []int{75, 90, 95} {
		j := stego.FromImage(viaJPEG(t, src, q))
		s, step, off := JPEGScore(j)
		t.Logf("JPEG %d: оценка %.3f, шаг %d, сдвиг %v", q, s, step, off)
		if s < JPEGThreshold {
			t.Fatalf("JPEG %d не распознан: %.3f", q, s)
		}
		if off != [2]int{0, 0} {
			t.Fatalf("JPEG %d: сдвиг сетки %v", q, off)
		}
		if d, _, _ := JPEGScore(Downscale2(j)); d >= JPEGThreshold {
			t.Fatalf("JPEG %d после уменьшения: %.3f", q, d)
		}
	}
	// кадрирование сдвигает сетку — она находится
	j := viaJPEG(t, src, 90).(interface {
		SubImage(image.Rectangle) image.Image
	}).SubImage(image.Rect(3, 5, 500, 380))
	s, _, off := JPEGScore(stego.FromImage(j))
	if s < JPEGThreshold || off != [2]int{5, 3} {
		t.Fatalf("кадрированный JPEG: оценка %.3f, сдвиг %v (ожидался [5 3])", s, off)
	}
}

func TestTexture(t *testing.T) {
	flat := image.NewNRGBA(image.Rect(0, 0, 100, 100))
	for i := range flat.Pix {
		flat.Pix[i] = 200
	}
	if s := TextureShare(stego.FromImage(flat)); s != 0 {
		t.Fatalf("ровная картинка: %.2f", s)
	}
	if s := TextureShare(stego.FromImage(photo(100, 100, 2, 10))); s < 0.5 {
		t.Fatalf("текстурная картинка: %.2f", s)
	}
	r := Check(stego.FromImage(flat))
	if !r.LowTexture || r.JPEG || len(r.Warnings) != 1 {
		t.Fatalf("%+v", r)
	}
}

func TestDownscale2(t *testing.T) {
	c := &stego.Cover{W: 4, H: 2, C: 1, Pix: []uint8{0, 2, 10, 10, 4, 6, 10, 11}}
	d := Downscale2(c)
	if d.W != 2 || d.H != 1 || d.Pix[0] != 3 || d.Pix[1] != 10 {
		t.Fatalf("%+v", d)
	}
}

func TestRisk(t *testing.T) {
	rep := &stego.Report{Level: stego.LevelNormal, Elements: 1_000_000}
	for _, c := range []struct {
		ct   int
		want Risk
	}{{1000, RiskLow}, {10_000, RiskMedium}, {20_000, RiskHigh}} {
		rep.Ciphertext = c.ct
		if r, _ := RiskOf(rep, nil); r != c.want {
			t.Fatalf("%d байт: %v, ожидался %v", c.ct, r, c.want)
		}
	}
	rep.Ciphertext = 1000
	rep.Level = stego.LevelMax
	if r, _ := RiskOf(rep, nil); r != RiskMedium {
		t.Fatalf("устойчивый уровень: %v", r)
	}
	rep.Level = stego.LevelNormal
	if r, _ := RiskOf(rep, &Report{JPEG: true}); r != RiskHigh {
		t.Fatalf("бывший JPEG: %v", r)
	}
	if r, _ := RiskOf(rep, &Report{LowTexture: true}); r != RiskMedium {
		t.Fatalf("мало текстуры: %v", r)
	}
}
