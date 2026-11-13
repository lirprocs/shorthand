package stego

import (
	"bytes"
	"math"
	"math/rand"
	"testing"

	"github.com/lirprocs/shorthand/internal/crypto/prng"
	"github.com/lirprocs/shorthand/internal/crypto/suite"
)

func testRand() *prng.Rand {
	return suite.KuznyechikMGM.NewRand(make([]byte, 32))
}

func TestSTCRoundTrip(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for _, h := range []int{1, 4, 7, 10} {
		for _, w := range []int{1, 2, 3, 8, 20} {
			for _, L := range []int{1, 7, 100, chunkBitsFor(h, w) + 37} {
				n := L * w
				x := make([]uint8, n)
				rho := make([]float64, n)
				for i := range x {
					x[i] = uint8(rng.Intn(2))
					rho[i] = rng.Float64()
				}
				msg := make([]uint8, L)
				for i := range msg {
					msg[i] = uint8(rng.Intn(2))
				}
				y, _, err := stcEmbed(x, rho, msg, h, w)
				if err != nil {
					t.Fatalf("h=%d w=%d L=%d: %v", h, w, L, err)
				}
				if got := stcExtract(y, L, h, w); !bytes.Equal(got, msg) {
					t.Fatalf("h=%d w=%d L=%d: извлечено не то", h, w, L)
				}
			}
		}
	}
}

// Эффективность встраивания (бит сообщения на одно изменение) при равных
// стоимостях должна расти с w и h и не превышать теоретический предел
// α / H⁻¹(α), α = 1/w.
func TestSTCEfficiency(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	L := 6000
	for _, c := range []struct {
		h, w     int
		min, max float64
	}{
		{1, 2, 1.9, 2.1},     // чётность пар: ровно 2 бита на изменение в среднем
		{10, 2, 3.9, 4.55},   // предел α/H⁻¹(α) при α = 1/2 — 4,54
		{10, 8, 6.0, 7.30},   // α = 1/8 — 7,30
		{10, 32, 7.6, 9.72},  // α = 1/32 — 9,72
		{10, 64, 8.3, 10.89}, // α = 1/64 — 10,89
		{10, 255, 9.0, 14.0}, // шире 64 подматрица не подбиралась (вариант 0)
	} {
		n := L * c.w
		x := make([]uint8, n)
		rho := make([]float64, n)
		for i := range x {
			x[i] = uint8(rng.Intn(2))
			rho[i] = 1
		}
		msg := make([]uint8, L)
		for i := range msg {
			msg[i] = uint8(rng.Intn(2))
		}
		y, cost, err := stcEmbed(x, rho, msg, c.h, c.w)
		if err != nil {
			t.Fatal(err)
		}
		changes := 0
		for i := range x {
			if x[i] != y[i] {
				changes++
			}
		}
		if float64(changes) != cost {
			t.Fatalf("стоимость %v != числу изменений %d", cost, changes)
		}
		e := float64(L) / float64(changes)
		t.Logf("h=%d w=%d: %.3f бит на изменение", c.h, c.w, e)
		if e < c.min || e > c.max {
			t.Fatalf("h=%d w=%d: эффективность %.3f вне [%.2f, %.2f]", c.h, c.w, e, c.min, c.max)
		}
	}
}

func TestSTCWet(t *testing.T) {
	// бесконечные стоимости не меняются, если есть другие варианты
	rng := rand.New(rand.NewSource(3))
	L, w := 500, 4
	n := L * w
	x := make([]uint8, n)
	rho := make([]float64, n)
	wet := make([]bool, n)
	for i := range x {
		x[i] = uint8(rng.Intn(2))
		rho[i] = 1
		if rng.Intn(4) == 0 {
			rho[i] = math.Inf(1)
			wet[i] = true
		}
	}
	msg := make([]uint8, L)
	for i := range msg {
		msg[i] = uint8(rng.Intn(2))
	}
	y, _, err := stcEmbed(x, rho, msg, 7, w)
	if err != nil {
		t.Fatal(err)
	}
	for i := range x {
		if wet[i] && x[i] != y[i] {
			t.Fatal("изменён недоступный элемент")
		}
	}
}

func TestPerm(t *testing.T) {
	for _, n := range []int{1, 2, 3, 17, 1000, 65537} {
		p := NewPerm(testRand(), n)
		seen := make([]bool, n)
		for j := 0; j < n; j++ {
			e := p.At(j)
			if e < 0 || e >= n || seen[e] {
				t.Fatalf("n=%d: не перестановка", n)
			}
			seen[e] = true
		}
	}
}
