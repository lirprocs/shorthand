package stego

import "sync"

// Функция стоимости HILL (B. Li, M. Wang, J. Huang, X. Li, 2014):
//
//	ρ = ( 1 / ( |X ⊛ KB| ⊛ L₁ ) ) ⊛ L₂
//
// KB — фильтр высоких частот 3×3 (остаток предсказания по соседям),
// L₁ — усреднение 3×3, L₂ — усреднение 15×15. В текстурных областях и на
// краях остаток большой и стоимость изменения мала; в ровных областях (небо,
// стена) стоимость огромна, и STC их почти не трогает. Каналы цветного
// изображения обрабатываются независимо. Граница — зеркальное отражение.

var kb = [3][3]float32{{-1, 2, -1}, {2, -4, 2}, {-1, 2, -1}}

const (
	hillEps     = 1e-10
	hillMaxCost = 1e10
)

// reflect — индекс с симметричным отражением за краями (как 'symmetric').
func reflect(i, n int) int {
	for i < 0 || i >= n {
		if i < 0 {
			i = -i - 1
		}
		if i >= n {
			i = 2*n - i - 1
		}
	}
	return i
}

// hillPlane — стоимости для канала ch контейнера (W·H значений).
func hillPlane(c *Cover, ch int) []float32 {
	w, h := c.W, c.H
	at := func(x, y int) float32 { return float32(c.Pix[(y*w+x)*c.C+ch]) }

	// 1. |X ⊛ KB|
	res := make([]float32, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var s float32
			for dy := -1; dy <= 1; dy++ {
				yy := reflect(y+dy, h)
				for dx := -1; dx <= 1; dx++ {
					s += kb[dy+1][dx+1] * at(reflect(x+dx, w), yy)
				}
			}
			if s < 0 {
				s = -s
			}
			res[y*w+x] = s
		}
	}
	// 2. ⊛ L₁ и обращение
	inv := boxMean(res, w, h, 1)
	for i, v := range inv {
		r := 1 / (v + hillEps)
		if r > hillMaxCost {
			r = hillMaxCost
		}
		inv[i] = r
	}
	// 3. ⊛ L₂
	return boxMean(inv, w, h, 7)
}

// boxMean — среднее по окну (2r+1)×(2r+1), разделимо: по строкам, затем по столбцам.
func boxMean(src []float32, w, h, r int) []float32 {
	tmp := make([]float32, w*h)
	k := float64(2*r + 1)
	for y := 0; y < h; y++ {
		row := src[y*w : (y+1)*w]
		for x := 0; x < w; x++ {
			var s float64
			for d := -r; d <= r; d++ {
				s += float64(row[reflect(x+d, w)])
			}
			tmp[y*w+x] = float32(s / k)
		}
	}
	out := make([]float32, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var s float64
			for d := -r; d <= r; d++ {
				s += float64(tmp[reflect(y+d, h)*w+x])
			}
			out[y*w+x] = float32(s / k)
		}
	}
	return out
}

// hillCosts — стоимости изменения элементов elems.
func hillCosts(c *Cover, elems []int) []float64 {
	costs := make([]float64, len(elems))
	var wg sync.WaitGroup
	for ch := 0; ch < c.C; ch++ {
		wg.Add(1)
		go func(ch int) {
			defer wg.Done()
			plane := hillPlane(c, ch)
			for i, e := range elems {
				if e%c.C == ch {
					costs[i] = float64(plane[e/c.C])
				}
			}
		}(ch)
	}
	wg.Wait()
	return costs
}
