package analysis

import (
	"math"
	"math/rand"
)

// Обучаемый детектор, который видит не только замену, но и ±1:
// признаки — совместная встречаемость соседних остатков (упрощённый SPAM,
// T. Pevný, P. Bas, J. Fridrich, 2010), классификатор — линейный
// дискриминант Фишера с регуляризацией, качество — AUC на перекрёстной
// проверке (изображение и его стеговерсия всегда в одной части).
//
// Остатки: первого порядка d = x[i+1] − x[i] и второго порядка
// r = x[i−1] − 2x[i] + x[i+1], усечённые до [−T, T], T = 2. Для пар соседних
// остатков по горизонтали и вертикали считается гистограмма 5×5 —
// 4 · 25 = 100 признаков, среднее по каналам.

const spamT = 2

// Features — вектор признаков изображения.
func Features(planes []Plane) []float64 {
	const n = 2*spamT + 1
	f := make([]float64, 4*n*n)
	trunc := func(v int) int {
		if v > spamT {
			return spamT
		}
		if v < -spamT {
			return -spamT
		}
		return v
	}
	for _, p := range planes {
		var hist [4][n * n]float64
		var cnt [4]float64
		at := func(x, y int) int { return int(p.Pix[y*p.W+x]) }
		for y := 0; y < p.H; y++ {
			for x := 0; x < p.W; x++ {
				// горизонталь
				if x+2 < p.W {
					d1 := trunc(at(x+1, y) - at(x, y))
					d2 := trunc(at(x+2, y) - at(x+1, y))
					hist[0][(d1+spamT)*n+d2+spamT]++
					cnt[0]++
				}
				if x+3 < p.W {
					r1 := trunc(at(x, y) - 2*at(x+1, y) + at(x+2, y))
					r2 := trunc(at(x+1, y) - 2*at(x+2, y) + at(x+3, y))
					hist[1][(r1+spamT)*n+r2+spamT]++
					cnt[1]++
				}
				// вертикаль
				if y+2 < p.H {
					d1 := trunc(at(x, y+1) - at(x, y))
					d2 := trunc(at(x, y+2) - at(x, y+1))
					hist[2][(d1+spamT)*n+d2+spamT]++
					cnt[2]++
				}
				if y+3 < p.H {
					r1 := trunc(at(x, y) - 2*at(x, y+1) + at(x, y+2))
					r2 := trunc(at(x, y+1) - 2*at(x, y+2) + at(x, y+3))
					hist[3][(r1+spamT)*n+r2+spamT]++
					cnt[3]++
				}
			}
		}
		for g := 0; g < 4; g++ {
			for k := 0; k < n*n; k++ {
				f[g*n*n+k] += hist[g][k] / math.Max(cnt[g], 1) / float64(len(planes))
			}
		}
	}
	return f
}

// fld обучает дискриминант Фишера: w = (Σ₀ + Σ₁ + λI)⁻¹ (μ₁ − μ₀).
func fld(x0, x1 [][]float64) []float64 {
	d := len(x0[0])
	mu := func(x [][]float64) []float64 {
		m := make([]float64, d)
		for _, v := range x {
			for i := range m {
				m[i] += v[i] / float64(len(x))
			}
		}
		return m
	}
	m0, m1 := mu(x0), mu(x1)
	s := make([][]float64, d)
	for i := range s {
		s[i] = make([]float64, d)
	}
	acc := func(x [][]float64, m []float64) {
		for _, v := range x {
			for i := 0; i < d; i++ {
				di := v[i] - m[i]
				for j := 0; j < d; j++ {
					s[i][j] += di * (v[j] - m[j])
				}
			}
		}
	}
	acc(x0, m0)
	acc(x1, m1)
	tr := 0.0
	for i := 0; i < d; i++ {
		tr += s[i][i]
	}
	lambda := 1e-3*tr/float64(d) + 1e-12
	for i := 0; i < d; i++ {
		s[i][i] += lambda
	}
	b := make([]float64, d)
	for i := range b {
		b[i] = m1[i] - m0[i]
	}
	return solve(s, b)
}

// solve — метод Гаусса с выбором главного элемента.
func solve(a [][]float64, b []float64) []float64 {
	n := len(b)
	for col := 0; col < n; col++ {
		p := col
		for r := col + 1; r < n; r++ {
			if math.Abs(a[r][col]) > math.Abs(a[p][col]) {
				p = r
			}
		}
		a[col], a[p] = a[p], a[col]
		b[col], b[p] = b[p], b[col]
		if a[col][col] == 0 {
			continue
		}
		for r := col + 1; r < n; r++ {
			f := a[r][col] / a[col][col]
			for c := col; c < n; c++ {
				a[r][c] -= f * a[col][c]
			}
			b[r] -= f * b[col]
		}
	}
	x := make([]float64, n)
	for r := n - 1; r >= 0; r-- {
		s := b[r]
		for c := r + 1; c < n; c++ {
			s -= a[r][c] * x[c]
		}
		if a[r][r] != 0 {
			x[r] = s / a[r][r]
		}
	}
	return x
}

func dot(a, b []float64) float64 {
	s := 0.0
	for i := range a {
		s += a[i] * b[i]
	}
	return s
}

// CVAUC — AUC дискриминанта на k-кратной перекрёстной проверке.
// cover[i] и stego[i] — одно и то же изображение до и после встраивания.
func CVAUC(cover, stego [][]float64, k int, seed int64) float64 {
	n := len(cover)
	idx := rand.New(rand.NewSource(seed)).Perm(n)
	sc := make([]float64, n)
	ss := make([]float64, n)
	for f := 0; f < k; f++ {
		var tr0, tr1 [][]float64
		var test []int
		for j, i := range idx {
			if j%k == f {
				test = append(test, i)
			} else {
				tr0 = append(tr0, cover[i])
				tr1 = append(tr1, stego[i])
			}
		}
		w := fld(tr0, tr1)
		for _, i := range test {
			sc[i] = dot(w, cover[i])
			ss[i] = dot(w, stego[i])
		}
	}
	return AUC(sc, ss)
}
