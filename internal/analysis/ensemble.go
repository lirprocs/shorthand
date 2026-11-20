package analysis

import (
	"errors"
	"math"
	"math/rand"
	"sort"
	"sync"
)

// Классификатор-ансамбль для стегоанализа в пространствах большой
// размерности (J. Kodovský, J. Fridrich, V. Holub, «Ensemble Classifiers for
// Steganalysis of Digital Media», IEEE TIFS 7(2), 2012) — стандартная пара к
// признакам SRM. Реализация по статье (авторская — на MATLAB):
//
//   - L базовых классификаторов — линейный дискриминант Фишера (FLD), каждый
//     на случайном подпространстве из DSub признаков и на бутстреп-выборке
//     пар «пустое — стего» (пары не разрываются);
//   - FLD: w = (S_w + λI)⁻¹(μ_s − μ_c), λ начинается с 10⁻¹⁰·tr(S_w)/d и
//     растёт в 10 раз, пока разложение Холецкого не удастся; порог — тот, что
//     даёт наименьшую ошибку ½(P_FA + P_MD) на своей обучающей выборке;
//   - решение ансамбля — большинство голосов;
//   - ошибка «вне мешка» (OOB): каждое обучающее изображение голосуется
//     только классификаторами, в чьей бутстреп-выборке его не было; по ней
//     выбираются DSub и число классификаторов L.
//
// Отличия от авторской программы: DSub ищется по сетке, а не одномерным
// поиском; L растёт, пока OOB-ошибка за последние 10 классификаторов не
// изменится меньше чем на 0,002 (не меньше EnsembleMinL, не больше
// EnsembleMaxL).

// EnsembleConfig — параметры обучения.
type EnsembleConfig struct {
	DSub    int   // размерность подпространства
	Seed    int64 // зерно (подпространства, бутстреп)
	Workers int   // параллельных классификаторов
}

const (
	EnsembleMinL = 30
	EnsembleMaxL = 99
)

type ensLearner struct {
	idx []int
	w   []float64
	b   float64
}

func (f *ensLearner) score(x []float32) float64 {
	s := 0.0
	for k, i := range f.idx {
		s += f.w[k] * float64(x[i])
	}
	return s
}

// Ensemble — обученный ансамбль.
type Ensemble struct {
	learners []ensLearner
	DSub     int
	OOB      float64 // ошибка вне мешка ½(P_FA + P_MD)
}

// Votes — доля классификаторов, отнёсших x к стего.
func (e *Ensemble) Votes(x []float32) float64 {
	n := 0
	for i := range e.learners {
		if e.learners[i].score(x) > e.learners[i].b {
			n++
		}
	}
	return float64(n) / float64(len(e.learners))
}

// L — число базовых классификаторов.
func (e *Ensemble) L() int { return len(e.learners) }

// TrainEnsemble обучает ансамбль на парах cover[i] — stego[i].
func TrainEnsemble(cover, stego [][]float32, cfg EnsembleConfig) (*Ensemble, error) {
	n := len(cover)
	if n == 0 || n != len(stego) {
		return nil, errors.New("ансамбль: нужны пары пустых и стегоизображений")
	}
	dim := len(cover[0])
	d := min(cfg.DSub, dim)
	workers := max(cfg.Workers, 1)
	rng := rand.New(rand.NewSource(cfg.Seed))

	type job struct {
		idx  []int
		boot []int
	}
	// голоса вне мешка: [i][0] — пустое, [i][1] — стего; за и против «стего»
	var oobStego, oobTotal [][2]int
	oobStego = make([][2]int, n)
	oobTotal = make([][2]int, n)
	e := &Ensemble{DSub: d}
	history := []float64{}
	for len(e.learners) < EnsembleMaxL {
		batch := make([]job, min(workers, EnsembleMaxL-len(e.learners)))
		for k := range batch {
			batch[k] = job{idx: rng.Perm(dim)[:d], boot: make([]int, n)}
			for i := range batch[k].boot {
				batch[k].boot[i] = rng.Intn(n)
			}
			sort.Ints(batch[k].idx)
		}
		res := make([]ensLearner, len(batch))
		errs := make([]error, len(batch))
		var wg sync.WaitGroup
		for k := range batch {
			wg.Add(1)
			go func(k int) {
				defer wg.Done()
				res[k], errs[k] = trainFLD(cover, stego, batch[k].idx, batch[k].boot)
			}(k)
		}
		wg.Wait()
		for k := range batch {
			if errs[k] != nil {
				return nil, errs[k]
			}
			f := res[k]
			e.learners = append(e.learners, f)
			in := make([]bool, n)
			for _, i := range batch[k].boot {
				in[i] = true
			}
			for i := 0; i < n; i++ {
				if in[i] {
					continue
				}
				for c, x := range [2][]float32{cover[i], stego[i]} {
					oobTotal[i][c]++
					if f.score(x) > f.b {
						oobStego[i][c]++
					}
				}
			}
			history = append(history, oobError(oobStego, oobTotal))
		}
		L := len(history)
		if L >= EnsembleMinL && math.Abs(history[L-1]-history[L-11]) < 0.002 {
			break
		}
	}
	e.OOB = history[len(history)-1]
	return e, nil
}

func oobError(st, tot [][2]int) float64 {
	var fa, md, nc, ns float64
	for i := range st {
		if tot[i][0] > 0 {
			nc++
			if 2*st[i][0] > tot[i][0] {
				fa++
			}
		}
		if tot[i][1] > 0 {
			ns++
			if 2*st[i][1] <= tot[i][1] {
				md++
			}
		}
	}
	if nc == 0 || ns == 0 {
		return 0.5
	}
	return (fa/nc + md/ns) / 2
}

// trainFLD — дискриминант Фишера на признаках idx по бутстреп-выборке пар boot.
func trainFLD(cover, stego [][]float32, idx, boot []int) (ensLearner, error) {
	d, m := len(idx), len(boot)
	// столбцы — центрированные признаки: a[j] — значения j-го признака у всех
	// 2m образцов (сначала пустые, потом стего), чтобы S_w = A·Aᵀ считалась
	// скалярными произведениями непрерывных строк
	a := make([][]float64, d)
	mc, ms := make([]float64, d), make([]float64, d)
	for j, f := range idx {
		row := make([]float64, 2*m)
		for t, i := range boot {
			row[t] = float64(cover[i][f])
			row[m+t] = float64(stego[i][f])
			mc[j] += row[t]
			ms[j] += row[m+t]
		}
		mc[j] /= float64(m)
		ms[j] /= float64(m)
		for t := 0; t < m; t++ {
			row[t] -= mc[j]
			row[m+t] -= ms[j]
		}
		a[j] = row
	}
	sw := make([]float64, d*d)
	tr := 0.0
	for i := 0; i < d; i++ {
		ai := a[i]
		for j := 0; j <= i; j++ {
			v := dot4(ai, a[j])
			sw[i*d+j] = v
			sw[j*d+i] = v
		}
		tr += sw[i*d+i]
	}
	diff := make([]float64, d)
	for j := range diff {
		diff[j] = ms[j] - mc[j]
	}
	lambda := 1e-10 * tr / float64(d)
	if lambda == 0 {
		lambda = 1e-10
	}
	var w []float64
	for try := 0; try < 30; try++ {
		if l, ok := cholesky(sw, d, lambda); ok {
			w = cholSolve(l, d, diff)
			break
		}
		lambda *= 10
	}
	if w == nil {
		return ensLearner{}, errors.New("ансамбль: S_w не удалось обратить")
	}
	f := ensLearner{idx: idx, w: w}
	// порог — наименьшая ошибка ½(P_FA + P_MD) на обучающей выборке
	type pt struct {
		v     float64
		stego bool
	}
	pts := make([]pt, 0, 2*m)
	for _, i := range boot {
		pts = append(pts, pt{f.score(cover[i]), false}, pt{f.score(stego[i]), true})
	}
	sort.Slice(pts, func(i, j int) bool { return pts[i].v < pts[j].v })
	// порог ниже всех: все — стего: P_FA = 1, P_MD = 0
	fa, md := float64(m), 0.0
	best, bestB := 0.5*(fa+md)/float64(m), pts[0].v-1
	for k := 0; k < len(pts); k++ {
		if pts[k].stego {
			md++
		} else {
			fa--
		}
		if k+1 < len(pts) && pts[k+1].v == pts[k].v {
			continue
		}
		if e := 0.5 * (fa + md) / float64(m); e < best {
			best = e
			if k+1 < len(pts) {
				bestB = (pts[k].v + pts[k+1].v) / 2
			} else {
				bestB = pts[k].v + 1
			}
		}
	}
	f.b = bestB
	return f, nil
}

func dot4(x, y []float64) float64 {
	var s0, s1, s2, s3 float64
	n := len(x)
	i := 0
	for ; i+4 <= n; i += 4 {
		s0 += x[i] * y[i]
		s1 += x[i+1] * y[i+1]
		s2 += x[i+2] * y[i+2]
		s3 += x[i+3] * y[i+3]
	}
	for ; i < n; i++ {
		s0 += x[i] * y[i]
	}
	return s0 + s1 + s2 + s3
}

// cholesky — L·Lᵀ = S + λI (нижняя треугольная, по строкам); false — матрица не
// положительно определена (или почти вырождена) при этом λ.
func cholesky(s []float64, d int, lambda float64) ([]float64, bool) {
	l := make([]float64, d*d)
	maxDiag := 0.0
	for i := 0; i < d; i++ {
		maxDiag = math.Max(maxDiag, s[i*d+i]+lambda)
	}
	for i := 0; i < d; i++ {
		li := l[i*d : i*d+d]
		for j := 0; j <= i; j++ {
			lj := l[j*d : j*d+d]
			v := s[i*d+j]
			if i == j {
				v += lambda
			}
			for k := 0; k < j; k++ {
				v -= li[k] * lj[k]
			}
			if i == j {
				if v <= 1e-14*maxDiag {
					return nil, false
				}
				li[i] = math.Sqrt(v)
			} else {
				li[j] = v / lj[j]
			}
		}
	}
	return l, true
}

func cholSolve(l []float64, d int, b []float64) []float64 {
	y := make([]float64, d)
	for i := 0; i < d; i++ {
		v := b[i]
		for k := 0; k < i; k++ {
			v -= l[i*d+k] * y[k]
		}
		y[i] = v / l[i*d+i]
	}
	x := make([]float64, d)
	for i := d - 1; i >= 0; i-- {
		v := y[i]
		for k := i + 1; k < d; k++ {
			v -= l[k*d+i] * x[k]
		}
		x[i] = v / l[i*d+i]
	}
	return x
}

// EnsembleResult — итог проверки на одном разбиении.
type EnsembleResult struct {
	PE   float64 // ½(P_FA + P_MD) на проверочной половине
	AUC  float64 // по доле голосов
	OOB  float64
	DSub int
	L    int
}

// TestEnsemble — ошибка ансамбля на проверочных парах.
func TestEnsemble(e *Ensemble, cover, stego [][]float32) EnsembleResult {
	sc, ss := make([]float64, len(cover)), make([]float64, len(stego))
	var fa, md float64
	for i := range cover {
		sc[i] = e.Votes(cover[i])
		if sc[i] > 0.5 {
			fa++
		}
		ss[i] = e.Votes(stego[i])
		if ss[i] <= 0.5 {
			md++
		}
	}
	return EnsembleResult{
		PE:   (fa/float64(len(cover)) + md/float64(len(stego))) / 2,
		AUC:  AUC(sc, ss),
		OOB:  e.OOB,
		DSub: e.DSub,
		L:    e.L(),
	}
}
