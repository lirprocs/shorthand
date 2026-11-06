// Package analysis — стегоанализ и метрики качества для экспериментов.
//
// Детекторы рассчитаны на замену младшего бита (LSB replacement) и
// оценивают долю элементов, несущих сообщение (бит на элемент):
//
//   - χ² (A. Westfeld, A. Pfitzmann, 1999) — выравнивание гистограммы в парах
//     значений (2k, 2k+1); результат — вероятность того, что пары выровнены;
//   - RS-анализ (J. Fridrich, M. Goljan, R. Du, 2001) — доли «регулярных» и
//     «сингулярных» групп пикселей при инвертировании младших битов;
//   - SPA (S. Dumitrescu, X. Wu, Z. Wang, 2003) — анализ пар соседних пикселей.
//
// Метрики: PSNR и SSIM (Z. Wang, A. Bovik и др., 2004), AUC — площадь под
// ROC-кривой (вероятность, что стегоизображение получит оценку выше пустого).
package analysis

import (
	"math"
	"sort"
)

// Plane — один канал изображения.
type Plane struct {
	W, H int
	Pix  []uint8
}

// Channels разбивает элементы контейнера (C каналов подряд) на плоскости.
func Channels(pix []uint8, w, h, c int) []Plane {
	out := make([]Plane, c)
	for ch := 0; ch < c; ch++ {
		p := Plane{W: w, H: h, Pix: make([]uint8, w*h)}
		for i := range p.Pix {
			p.Pix[i] = pix[i*c+ch]
		}
		out[ch] = p
	}
	return out
}

// ─── χ² ─────────────────────────────────────────────────────────────────────

// ChiSquare — вероятность (0…1), что гистограмма пар (2k, 2k+1) выровнена,
// как после полной замены младших битов. Пары с ожидаемой частотой меньше 5
// не учитываются.
func ChiSquare(planes []Plane) float64 {
	var hist [256]float64
	for _, p := range planes {
		for _, v := range p.Pix {
			hist[v]++
		}
	}
	chi, df := 0.0, -1
	for k := 0; k < 128; k++ {
		e := (hist[2*k] + hist[2*k+1]) / 2
		if e < 5 {
			continue
		}
		d := hist[2*k] - e
		chi += d * d / e
		df++
	}
	if df < 1 {
		return 0
	}
	return 1 - regGammaP(float64(df)/2, chi/2)
}

// regGammaP — регуляризованная неполная гамма-функция P(a, x).
func regGammaP(a, x float64) float64 {
	if x <= 0 {
		return 0
	}
	lg, _ := math.Lgamma(a)
	if x < a+1 {
		sum, term := 1/a, 1/a
		for n := 1; n < 1000; n++ {
			term *= x / (a + float64(n))
			sum += term
			if math.Abs(term) < math.Abs(sum)*1e-15 {
				break
			}
		}
		return sum * math.Exp(-x+a*math.Log(x)-lg)
	}
	// непрерывная дробь для Q(a, x)
	b := x + 1 - a
	c := 1 / 1e-300
	d := 1 / b
	h := d
	for i := 1; i < 1000; i++ {
		an := -float64(i) * (float64(i) - a)
		b += 2
		d = an*d + b
		if math.Abs(d) < 1e-300 {
			d = 1e-300
		}
		c = b + an/c
		if math.Abs(c) < 1e-300 {
			c = 1e-300
		}
		d = 1 / d
		del := d * c
		h *= del
		if math.Abs(del-1) < 1e-15 {
			break
		}
	}
	return 1 - math.Exp(-x+a*math.Log(x)-lg)*h
}

// ─── RS-анализ ──────────────────────────────────────────────────────────────

func flip1(x int) int { return x ^ 1 }

func flipNeg(x int) int { // F₋₁: 2k−1 ↔ 2k
	if x&1 == 0 {
		return x - 1
	}
	return x + 1
}

func smooth(g []int) int {
	s := 0
	for i := 1; i < len(g); i++ {
		d := g[i] - g[i-1]
		if d < 0 {
			d = -d
		}
		s += d
	}
	return s
}

// rsCounts — доли регулярных и сингулярных групп для маски M и −M.
func rsCounts(p Plane, invertAll bool) (rm, sm, rn, sn float64) {
	mask := [4]int{0, 1, 1, 0}
	var cnt [4]int
	total := 0
	g := make([]int, 4)
	gm := make([]int, 4)
	gn := make([]int, 4)
	for y := 0; y < p.H; y++ {
		row := p.Pix[y*p.W : (y+1)*p.W]
		for x := 0; x+4 <= p.W; x += 4 {
			for i := 0; i < 4; i++ {
				v := int(row[x+i])
				if invertAll {
					v ^= 1
				}
				g[i] = v
				gm[i], gn[i] = v, v
				if mask[i] == 1 {
					gm[i] = flip1(v)
					gn[i] = flipNeg(v)
				}
			}
			f0, fm, fn := smooth(g), smooth(gm), smooth(gn)
			switch {
			case fm > f0:
				cnt[0]++
			case fm < f0:
				cnt[1]++
			}
			switch {
			case fn > f0:
				cnt[2]++
			case fn < f0:
				cnt[3]++
			}
			total++
		}
	}
	t := float64(total)
	return float64(cnt[0]) / t, float64(cnt[1]) / t, float64(cnt[2]) / t, float64(cnt[3]) / t
}

// RS — оценка доли элементов с сообщением (бит на элемент) по RS-анализу,
// среднее по каналам. Для пустого изображения ≈ 0.
func RS(planes []Plane) float64 {
	sum := 0.0
	for _, p := range planes {
		rm0, sm0, rn0, sn0 := rsCounts(p, false)
		rm1, sm1, rn1, sn1 := rsCounts(p, true)
		d0, d1 := rm0-sm0, rm1-sm1
		dn0, dn1 := rn0-sn0, rn1-sn1
		a := 2 * (d1 + d0)
		b := dn0 - dn1 - d1 - 3*d0
		c := d0 - dn0
		z := smallerRoot(a, b, c)
		est := 0.0
		if !math.IsNaN(z) && z != 0.5 {
			est = z / (z - 0.5)
		}
		sum += est
	}
	return sum / float64(len(planes))
}

func smallerRoot(a, b, c float64) float64 {
	if math.Abs(a) < 1e-12 {
		if b == 0 {
			return math.NaN()
		}
		return -c / b
	}
	disc := b*b - 4*a*c
	if disc < 0 {
		disc = 0
	}
	sq := math.Sqrt(disc)
	r1, r2 := (-b+sq)/(2*a), (-b-sq)/(2*a)
	if math.Abs(r1) < math.Abs(r2) {
		return r1
	}
	return r2
}

// ─── SPA ────────────────────────────────────────────────────────────────────

// SPA — оценка доли элементов с сообщением по анализу пар соседних
// пикселей (по горизонтали и вертикали), среднее по каналам: меньший корень
// (W+Z)/2·p² + (2X − P)·p + Y − X = 0.
func SPA(planes []Plane) float64 {
	sum := 0.0
	for _, pl := range planes {
		var X, Y, Z, W, P float64
		pair := func(u, v int) {
			P++
			switch {
			case u == v:
				Z++
			case (v&1 == 0 && u < v) || (v&1 == 1 && u > v):
				X++
			default:
				Y++
				if u>>1 == v>>1 {
					W++
				}
			}
		}
		for y := 0; y < pl.H; y++ {
			for x := 0; x < pl.W; x++ {
				u := int(pl.Pix[y*pl.W+x])
				if x+1 < pl.W {
					pair(u, int(pl.Pix[y*pl.W+x+1]))
				}
				if y+1 < pl.H {
					pair(u, int(pl.Pix[(y+1)*pl.W+x]))
				}
			}
		}
		est := smallerRoot((W+Z)/2, 2*X-P, Y-X)
		if math.IsNaN(est) {
			est = 0
		}
		sum += est
	}
	return sum / float64(len(planes))
}

// ─── качество ───────────────────────────────────────────────────────────────

// PSNR между двумя наборами элементов (дБ); +Inf при совпадении.
func PSNR(a, b []uint8) float64 {
	var se float64
	for i := range a {
		d := float64(a[i]) - float64(b[i])
		se += d * d
	}
	if se == 0 {
		return math.Inf(1)
	}
	mse := se / float64(len(a))
	return 10 * math.Log10(255*255/mse)
}

// Luma — яркость Y = 0,299R + 0,587G + 0,114B (для C = 1 — сам канал).
func Luma(pix []uint8, w, h, c int) []float64 {
	out := make([]float64, w*h)
	for i := range out {
		if c == 1 {
			out[i] = float64(pix[i])
		} else {
			out[i] = 0.299*float64(pix[3*i]) + 0.587*float64(pix[3*i+1]) + 0.114*float64(pix[3*i+2])
		}
	}
	return out
}

// SSIM — средний индекс структурного сходства (окно — гауссово 11×11, σ = 1,5).
func SSIM(x, y []float64, w, h int) float64 {
	const c1, c2 = (0.01 * 255) * (0.01 * 255), (0.03 * 255) * (0.03 * 255)
	k := gaussKernel(11, 1.5)
	mul := func(a, b []float64) []float64 {
		r := make([]float64, len(a))
		for i := range a {
			r[i] = a[i] * b[i]
		}
		return r
	}
	mx, my := blur(x, w, h, k), blur(y, w, h, k)
	sxx, syy, sxy := blur(mul(x, x), w, h, k), blur(mul(y, y), w, h, k), blur(mul(x, y), w, h, k)
	r := len(k) / 2
	sum, n := 0.0, 0
	for yy := r; yy < h-r; yy++ {
		for xx := r; xx < w-r; xx++ {
			i := yy*w + xx
			vx := sxx[i] - mx[i]*mx[i]
			vy := syy[i] - my[i]*my[i]
			cxy := sxy[i] - mx[i]*my[i]
			sum += ((2*mx[i]*my[i] + c1) * (2*cxy + c2)) / ((mx[i]*mx[i] + my[i]*my[i] + c1) * (vx + vy + c2))
			n++
		}
	}
	if n == 0 {
		return 1
	}
	return sum / float64(n)
}

func gaussKernel(size int, sigma float64) []float64 {
	k := make([]float64, size)
	s := 0.0
	for i := range k {
		d := float64(i - size/2)
		k[i] = math.Exp(-d * d / (2 * sigma * sigma))
		s += k[i]
	}
	for i := range k {
		k[i] /= s
	}
	return k
}

func blur(src []float64, w, h int, k []float64) []float64 {
	r := len(k) / 2
	tmp := make([]float64, len(src))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			s := 0.0
			for j, kv := range k {
				xx := min(max(x+j-r, 0), w-1)
				s += kv * src[y*w+xx]
			}
			tmp[y*w+x] = s
		}
	}
	out := make([]float64, len(src))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			s := 0.0
			for j, kv := range k {
				yy := min(max(y+j-r, 0), h-1)
				s += kv * tmp[yy*w+x]
			}
			out[y*w+x] = s
		}
	}
	return out
}

// AUC — площадь под ROC-кривой: вероятность, что случайное стегоизображение
// получит оценку больше случайного пустого (0,5 — детектор угадывает,
// 1 — различает всегда). Статистика Манна–Уитни, ничьи — пополам.
func AUC(cover, stego []float64) float64 {
	type item struct {
		v     float64
		stego bool
	}
	all := make([]item, 0, len(cover)+len(stego))
	for _, v := range cover {
		all = append(all, item{v, false})
	}
	for _, v := range stego {
		all = append(all, item{v, true})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].v < all[j].v })
	// ранги с усреднением ничьих
	rankSum := 0.0
	for i := 0; i < len(all); {
		j := i
		for j < len(all) && all[j].v == all[i].v {
			j++
		}
		avg := float64(i+j+1) / 2 // ранги i+1…j
		for k := i; k < j; k++ {
			if all[k].stego {
				rankSum += avg
			}
		}
		i = j
	}
	ns, nc := float64(len(stego)), float64(len(cover))
	return (rankSum - ns*(ns+1)/2) / (ns * nc)
}
