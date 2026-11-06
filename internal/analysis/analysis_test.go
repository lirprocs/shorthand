package analysis

import (
	"math"
	"math/rand"
	"testing"
)

// smoothPlane — «естественное» изображение: плавный фон и слабый шум.
func smoothPlane(w, h int, seed int64) Plane {
	rng := rand.New(rand.NewSource(seed))
	p := Plane{W: w, H: h, Pix: make([]uint8, w*h)}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			v := 128 + 60*math.Sin(float64(x)/23) + 40*math.Cos(float64(y)/31+float64(x)/57) + rng.NormFloat64()*3
			p.Pix[y*w+x] = uint8(math.Max(0, math.Min(255, v)))
		}
	}
	return p
}

func lsbReplace(p Plane, rate float64, rng *rand.Rand) Plane {
	q := Plane{W: p.W, H: p.H, Pix: append([]uint8(nil), p.Pix...)}
	for i := range q.Pix {
		if rng.Float64() < rate {
			q.Pix[i] = q.Pix[i]&^1 | uint8(rng.Intn(2))
		}
	}
	return q
}

func lsbMatch(p Plane, rate float64, rng *rand.Rand) Plane {
	q := Plane{W: p.W, H: p.H, Pix: append([]uint8(nil), p.Pix...)}
	for i, v := range q.Pix {
		if rng.Float64() < rate && rng.Intn(2) == 1 { // половина битов уже совпадает
			switch {
			case v == 0:
				v = 1
			case v == 255:
				v = 254
			case rng.Intn(2) == 0:
				v++
			default:
				v--
			}
			q.Pix[i] = v
		}
	}
	return q
}

// RS и SPA должны оценивать долю замены младших битов с точностью ~0,05.
func TestEstimatorsTrackLSBReplacement(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	cover := smoothPlane(256, 256, 1)
	for _, rate := range []float64{0, 0.1, 0.3, 0.6} {
		st := []Plane{lsbReplace(cover, rate, rng)}
		rs, spa := RS(st), SPA(st)
		t.Logf("замена, доля %.1f: RS %.3f, SPA %.3f, χ² %.3f", rate, rs, spa, ChiSquare(st))
		if math.Abs(rs-rate) > 0.06 || math.Abs(spa-rate) > 0.06 {
			t.Fatalf("доля %.1f: RS %.3f, SPA %.3f", rate, rs, spa)
		}
	}
}

// χ² различает только контейнеры с неровной гистограммой пар: у гладкого
// шума (как в smoothPlane) пары и так выровнены — известное ограничение
// метода. Здесь чётные значения встречаются вчетверо чаще нечётных.
func TestChiSquare(t *testing.T) {
	rng := rand.New(rand.NewSource(6))
	cover := smoothPlane(256, 256, 7)
	for i, v := range cover.Pix {
		v &^= 1
		if rng.Intn(5) == 0 && v < 255 {
			v++
		}
		cover.Pix[i] = v
	}
	if p := ChiSquare([]Plane{cover}); p > 0.05 {
		t.Fatalf("χ² на пустом: %.3f", p)
	}
	if p := ChiSquare([]Plane{lsbReplace(cover, 1, rng)}); p < 0.9 {
		t.Fatalf("χ² при полной замене: %.3f", p)
	}
}

func TestLSBMatchingFoolsEstimators(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	cover := smoothPlane(256, 256, 3)
	st := []Plane{lsbMatch(cover, 0.6, rng)}
	t.Logf("±1, доля 0,6: RS %.3f, SPA %.3f, χ² %.3f", RS(st), SPA(st), ChiSquare(st))
}

func TestAUC(t *testing.T) {
	if a := AUC([]float64{1, 2, 3}, []float64{4, 5, 6}); a != 1 {
		t.Fatal(a)
	}
	if a := AUC([]float64{1, 2}, []float64{1, 2}); a != 0.5 {
		t.Fatal(a)
	}
}

func TestPSNRSSIM(t *testing.T) {
	p := smoothPlane(64, 64, 4)
	if !math.IsInf(PSNR(p.Pix, p.Pix), 1) {
		t.Fatal("PSNR одинаковых")
	}
	l := Luma(p.Pix, 64, 64, 1)
	if s := SSIM(l, l, 64, 64); math.Abs(s-1) > 1e-9 {
		t.Fatalf("SSIM одинаковых %v", s)
	}
	q := lsbReplace(p, 1, rand.New(rand.NewSource(5)))
	if ps := PSNR(p.Pix, q.Pix); ps < 50 || ps > 54 {
		t.Fatalf("PSNR после замены всех младших битов %.2f (ожидается ≈ 51,1)", ps)
	}
}
