package stego

import (
	"errors"
	"math"
	"math/rand/v2"
	"sync"
)

// Модель выбора параметров встраивания (заголовок версии 6).
//
// Пользователь задаёт не способ записи, а цель — какие повреждения сообщение
// должно пережить (Target): долю элементов, изменённых на ±1 (шум), и долю
// площади, закрашенной одним цветом (закраска), с вероятностью отказа не
// больше Fail. Программа решает задачу
//
//	θ* = arg min D̂(θ)  при  N(θ) ≤ n,  P_усп(θ, повреждение) ≥ 1 − Fail,
//
// где θ = (способ записи сообщения, проверочных байтов RS r, способ записи
// заголовка), D̂ — ожидаемое искажение по стоимостям HILL (мера
// обнаружимости), N — занятые элементы, n — пригодные.
//
// Надёжность. Порядок элементов задаёт ключевая перестановка, не связанная с
// геометрией изображения, поэтому любое повреждение задевает занятые элементы
// как случайную выборку: каждый повреждён независимо с вероятностью, равной
// доле повреждения. При шуме доли q младший бит меняется всегда: ошибка
// элемента — q. При закраске доли a элемент попадает в область с вероятностью
// a, младший бит заливки неверен с вероятностью ½; внутри области (кроме
// полосы в 2 пикселя у края, доля β) окрестность 5×5 одноцветна, и элемент
// отмечается стёртым (erasures в embed.go). Байт потока зависит от k
// элементов (k = 8w у чётности групп из w, (2ᵖ − 1)·ḡ у кода Хэмминга с p
// битами на группу, ḡ — сколько групп касается байт), отсюда вероятности
// ошибки и стирания байта; блок Рида–Соломона из n байт с r проверочными
// восстанавливается, если 2e + E ≤ r. Сообщение извлекается, если прочитан
// заголовок и восстановлены все блоки.
//
// Проверка модели по опыту устойчивости (docs/EXPERIMENTS.md, раздел 2):
// среднее расхождение — 1,6 процентного пункта на 80 ячейках; оценки
// искажения кода Хэмминга и чётности совпадают со встраиванием с точностью
// до 1 % (раздел 8).
//
// Канал без потерь (Target нулевой) — частный случай: модель даёт STC
// наибольшей ширины без кода RS («обычная»). STC с w > 1 при любом
// повреждении размножает ошибку на сотни элементов, поэтому при ненулевой
// цели перебираются только чётность групп и код Хэмминга.

// Target — какие повреждения сообщение должно пережить.
type Target struct {
	Noise float64 // доля элементов, изменённых на ±1 (лёгкая обработка)
	Paint float64 // доля площади, закрашенной одним цветом
	Fail  float64 // допустимая вероятность отказа; 0 — modelFail
}

const modelFail = 0.05

// Цели уровней: «обычная» — канал без потерь, «повышенная» и
// «максимальная» — пределы, которые эти уровни держали в версии 5
// (docs/EXPERIMENTS.md, раздел 2).
var levelTargets = [3]Target{
	LevelNormal: {},
	LevelHigh:   {Noise: 0.005, Paint: 0.01},
	LevelMax:    {Noise: 0.02, Paint: 0.05},
}

// Target — цель уровня.
func (l Level) Target() Target {
	if l <= LevelMax {
		return levelTargets[l]
	}
	return Target{}
}

func (t Target) lossless() bool { return t.Noise <= 0 && t.Paint <= 0 }

func (t Target) fail() float64 {
	if t.Fail > 0 {
		return t.Fail
	}
	return modelFail
}

func (t Target) valid() bool {
	ok := func(v float64) bool { return v >= 0 && v < 1 && !math.IsNaN(v) }
	return ok(t.Noise) && ok(t.Paint) && ok(t.Fail)
}

// ErrTarget — заданных повреждений не выдерживает даже самый сильный способ.
var ErrTarget = errors.New("заданных повреждений сообщение не переживёт ни при каком способе записи: уменьшите ожидаемые повреждения")

// ─── модель канала ───────────────────────────────────────────────────────────

// damage — одно повреждение модели канала.
type damage struct {
	paint bool
	frac  float64
	beta  float64 // закраска: доля области у края, где стирание не отмечается
	flat  float64 // закраска: доля элементов, ровных уже в контейнере (ложные стирания)
}

func (t Target) damages(cv *Cover) []damage {
	var ds []damage
	if t.Noise > 0 {
		ds = append(ds, damage{frac: t.Noise})
	}
	if t.Paint > 0 {
		ds = append(ds, damage{paint: true, frac: t.Paint, beta: paintBorder(t.Paint, cv.W, cv.H), flat: sampleOf(cv).flat})
	}
	return ds
}

// paintBorder — доля закрашенного прямоугольника (стороны 4 : 3) в полосе
// 2 пикселя у края.
func paintBorder(a float64, w, h int) float64 {
	area := a * float64(w*h)
	rw := math.Max(1, math.Sqrt(area*4/3))
	rh := math.Max(1, area/rw)
	return 1 - math.Max(rw-4, 0)*math.Max(rh-4, 0)/(rw*rh)
}

// plain — вероятность ошибки байта, зависящего от k элементов, без отметок
// стираний.
func (d damage) plain(k float64) float64 {
	q := d.frac
	if d.paint {
		q /= 2
	}
	return 1 - math.Pow(1-q, k)
}

// marked — вероятности стирания и ошибки байта при закраске с отметками.
// Стёртым отмечается и элемент, ровный уже в самом контейнере (небо,
// заливка): такие отметки лишние, но они расходуют проверочные байты.
func (d damage) marked(k float64) (pE, pe float64) {
	in := d.flat + (1-d.flat)*d.frac*(1-d.beta)
	wrong := (1 - d.flat) * d.frac * d.beta / 2
	pE = 1 - math.Pow(1-in, k)
	pe = math.Pow(1-in, k) - math.Pow(1-in-wrong, k)
	return pE, max(pe, 0)
}

var lfact = func() [256]float64 {
	var t [256]float64
	for i := range t {
		t[i], _ = math.Lgamma(float64(i + 1))
	}
	return t
}()

func lpow(p float64, k int) float64 {
	if k == 0 {
		return 0
	}
	if p <= 0 {
		return math.Inf(-1)
	}
	return float64(k) * math.Log(p)
}

// blockOK — вероятность восстановить блок из n ≤ 255 байт с r проверочными:
// стирание байта — pE, ошибка — pe, успех при 2e + E ≤ r.
func blockOK(n, r int, pE, pe float64) float64 {
	q0 := 1 - pE - pe
	s := 0.0
	for E := 0; E <= min(r, n); E++ {
		if pE == 0 && E > 0 {
			break
		}
		for e := 0; e <= (r-E)/2 && E+e <= n; e++ {
			if pe == 0 && e > 0 {
				break
			}
			l := lfact[n] - lfact[E] - lfact[e] - lfact[n-E-e] + lpow(pE, E) + lpow(pe, e) + lpow(q0, n-E-e)
			s += math.Exp(l)
		}
	}
	return min(s, 1)
}

// rsSizes — длины данных блоков кода RS (как rs.layout).
func rsSizes(n, r int) []int {
	k := 255 - r
	nb := max(1, (n+k-1)/k)
	sizes := make([]int, nb)
	for i := range sizes {
		sizes[i] = n / nb
		if i < n%nb {
			sizes[i]++
		}
	}
	return sizes
}

// streamOK — вероятность восстановить поток из n байт с r проверочными на
// блок; байт зависит от k элементов.
func streamOK(n, r int, k float64, d damage) float64 {
	pb := d.plain(k)
	if r == 0 {
		return math.Pow(1-pb, float64(n))
	}
	var pE, pe float64
	if d.paint {
		pE, pe = d.marked(k)
	}
	// у блоков не больше двух разных длин
	memo := map[int]float64{}
	p := 1.0
	for _, s := range rsSizes(n, r) {
		v, ok := memo[s]
		if !ok {
			v = blockOK(s+r, r, 0, pb)
			if d.paint {
				// декодер пробует и без отметок, и с отметками стираний
				v = max(v, blockOK(s+r, r, pE, pe))
			}
			memo[s] = v
		}
		p *= v
	}
	return p
}

// headerOK — вероятность прочитать заголовок способа class (без отметок
// стираний: readHeader их не использует).
func headerOK(class int, d damage, usable int) float64 {
	st := stylesV6[class]
	return blockOK(st.coded(), st.parity, 0, d.plain(bytePerElems(st.coding(usable))))
}

// hamBytePerElems — среднее число элементов, от которых зависит байт потока
// при коде Хэмминга с p битами на группу.
func hamBytePerElems(p int) float64 {
	tot := 0
	for b := 0; b < p; b++ { // 8b mod p повторяется с периодом p байт
		first, last := 8*b/p, (8*b+7)/p
		tot += last - first + 1
	}
	return float64(tot) / float64(p) * float64(int(1)<<p-1)
}

// ─── оценка искажения ────────────────────────────────────────────────────────

// sampleSize — элементов в выборке стоимостей для оценки искажения.
const sampleSize = 1 << 15

// sample — случайная выборка пригодных элементов контейнера: стоимости HILL
// и доля элементов с ровной окрестностью 5×5 (flat5). Выборка не зависит от
// ключа: элементы сообщения — случайная выборка из пригодных, поэтому
// ожидаемые искажение и доля ровных одинаковы для любой случайной выборки
// того же размера. Хранится выборка последнего контейнера: окно
// пересчитывает сводку при каждом изменении текста.
type sample struct {
	costs []float64
	flat  float64
}

var sampleCache struct {
	sync.Mutex
	cv *Cover
	n  int
	s  sample
}

func sampleOf(cv *Cover) sample {
	sampleCache.Lock()
	defer sampleCache.Unlock()
	if sampleCache.cv == cv && sampleCache.n == len(cv.Pix) {
		return sampleCache.s
	}
	rng := rand.New(rand.NewPCG(0x5348, 6))
	var elems []int
	if usableCount(cv) <= sampleSize {
		for e := 0; e < cv.Len(); e++ {
			if cv.Usable(e) {
				elems = append(elems, e)
			}
		}
		rng.Shuffle(len(elems), func(i, j int) { elems[i], elems[j] = elems[j], elems[i] })
	} else {
		for len(elems) < sampleSize {
			if e := rng.IntN(cv.Len()); cv.Usable(e) {
				elems = append(elems, e)
			}
		}
	}
	flat := 0
	for _, e := range elems {
		if flat5(cv, e) {
			flat++
		}
	}
	s := sample{costs: hillCosts(cv, elems), flat: float64(flat) / float64(max(len(elems), 1))}
	sampleCache.cv, sampleCache.n, sampleCache.s = cv, len(cv.Pix), s
	return s
}

// expectedDistortion — ожидаемое искажение записи bits бит способом cd по
// выборке стоимостей rho: среднее на группу × число групп.
//
//	чётность w:   ½ · min ρ по группе;
//	код Хэмминга: среднее по разностям синдромов d = 1…2ᵖ − 1 стоимости
//	              изменения элемента d или пары i ⊕ j = d по правилу hamEmbed,
//	              делённое на 2ᵖ (d = 0 — без изменений).
func expectedDistortion(rho []float64, cd coding, bits int) float64 {
	if cd.ham > 1 {
		n := 1<<cd.ham - 1
		groups := len(rho) / n
		if groups == 0 {
			return math.Inf(1)
		}
		sum := 0.0
		for g := 0; g < groups; g++ {
			r := rho[g*n : (g+1)*n]
			for d := 1; d <= n; d++ {
				best, cost := r[d-1], r[d-1]
				for i := 1; i <= n; i++ {
					j := i ^ d
					if j <= i {
						continue
					}
					if c := (r[i-1] + r[j-1]) * hamPairGain; c < best {
						best, cost = c, r[i-1]+r[j-1]
					}
				}
				sum += cost
			}
		}
		return sum / float64(groups) / float64(n+1) * float64(bits) / float64(cd.ham)
	}
	w := max(cd.w, 1)
	groups := len(rho) / w
	if groups == 0 {
		return math.Inf(1)
	}
	sum := 0.0
	for g := 0; g < groups; g++ {
		m := rho[g*w]
		for _, v := range rho[g*w+1 : (g+1)*w] {
			m = min(m, v)
		}
		sum += m / 2
	}
	return sum / float64(groups) * float64(bits)
}

// ─── выбор ───────────────────────────────────────────────────────────────────

// choice — решение задачи выбора.
type choice struct {
	cd          coding
	rs          int     // проверочных байтов RS на блок (0 — без кода)
	class       int     // способ записи заголовка версии 6 (stylesV6)
	reliability float64 // наименьшая по повреждениям цели P_усп (1 — канал без потерь)
	distortion  float64 // D̂ сообщения (без заголовка); 0 — не оценивалось
	changes     float64 // ожидаемое число изменений сообщения (expectedChanges)
}

// candidates — способы записи, перебираемые при ненулевой цели: чётность
// групп из 1…8 элементов и код Хэмминга с 2…8 битами на группу.
func candidates() []coding {
	var cs []coding
	for w := 1; w <= 8; w++ {
		cs = append(cs, coding{h: 1, w: w})
	}
	for p := 2; p <= hamMaxP; p++ {
		cs = append(cs, coding{ham: p})
	}
	return cs
}

// bytePerElems — от скольких элементов зависит байт потока: код Хэмминга —
// hamBytePerElems; чётность групп из w (STC h = 1) — 8w; STC высоты h —
// w·(h + 7): бит зависит от h соседних блоков решётки по w элементов.
func bytePerElems(cd coding) float64 {
	switch {
	case cd.ham > 1:
		return hamBytePerElems(cd.ham)
	case cd.h <= 1:
		return 8 * float64(max(cd.w, 1))
	}
	return float64(cd.w * (cd.h + 7))
}

// reliability — надёжность по модели для уже выбранных параметров: наименьшая
// по повреждениям цели t вероятность извлечь сообщение из n байт (1 — канал
// без потерь).
func reliability(cv *Cover, n int, t Target, cd coding, r, class, usable int) float64 {
	p := 1.0
	for _, d := range t.damages(cv) {
		p = min(p, headerOK(class, d, usable)*streamOK(n, r, bytePerElems(cd), d))
	}
	return p
}

// choose решает задачу выбора для сообщения из n байт (шифротекст с
// имитовставкой) в контейнере cv с usable пригодными элементами.
func choose(cv *Cover, n int, t Target, usable int) (choice, error) {
	if t.lossless() {
		hElems := stylesV6[0].elems(usable)
		bits := 8 * n
		w := min(maxW, max(usable-hElems, 0)/bits)
		if w < 1 {
			return choice{}, &CapacityError{Need: hElems + bits, Have: usable}
		}
		return choice{cd: coding{h: stcHeight(bits, w, 12), w: w}, reliability: 1}, nil
	}
	ds := t.damages(cv)
	need := 1 - t.fail()

	// заголовок: самый слабый (и дешёвый) способ, который выдерживает цель с
	// запасом: отказ заголовка — не больше половины допустимого
	class, hdr := 0, make([]float64, len(ds))
	for c := 1; c < len(stylesV6) && class == 0; c++ {
		ok := true
		for i, d := range ds {
			hdr[i] = headerOK(c, d, usable)
			ok = ok && hdr[i] >= 1-t.fail()/2
		}
		if ok {
			class = c
		}
	}
	if class == 0 {
		return choice{}, ErrTarget
	}
	hElems := stylesV6[class].elems(usable)

	rel := func(cd coding, r int) float64 {
		k, p := bytePerElems(cd), 1.0
		for i, d := range ds {
			p = min(p, hdr[i]*streamOK(n, r, k, d))
		}
		return p
	}
	rho := sampleOf(cv).costs
	var feasible []choice
	minNeed := -1
	for _, cd := range candidates() {
		// наименьшее чётное r с P_усп ≥ need; надёжность не убывает с ростом r
		if rel(cd, maxRS) < need {
			continue
		}
		lo, hi := 0, maxRS/2 // в половинах
		if rel(cd, 0) >= need {
			hi = 0
		}
		for lo < hi {
			mid := (lo + hi) / 2
			if rel(cd, 2*mid) >= need {
				hi = mid
			} else {
				lo = mid + 1
			}
		}
		r := 2 * hi
		bits := 8 * codedLen(n, r)
		used := hElems + cd.elems(bits)
		if used > usable {
			if minNeed < 0 || used < minNeed {
				minNeed = used
			}
			continue
		}
		feasible = append(feasible, choice{cd: cd, rs: r, class: class, reliability: rel(cd, r),
			distortion: expectedDistortion(rho, cd, bits), changes: expectedChanges(cd, bits)})
	}
	if len(feasible) == 0 {
		if minNeed >= 0 {
			return choice{}, &CapacityError{Need: minNeed, Have: usable}
		}
		return choice{}, ErrTarget
	}
	// D̂ оценено по выборке и точно примерно до 10 %: среди решений не дальше
	// distortionTol от наименьшего берётся то, что меняет меньше элементов
	// (без этого правила модель ради неразличимой разницы в D̂ раздувала код
	// RS в несколько раз; docs/EXPERIMENTS.md, раздел 8)
	dmin := math.Inf(1)
	for _, c := range feasible {
		dmin = min(dmin, c.distortion)
	}
	best := choice{changes: math.Inf(1)}
	for _, c := range feasible {
		if c.distortion <= dmin*(1+distortionTol) && (c.changes < best.changes || c.changes == best.changes && c.distortion < best.distortion) {
			best = c
		}
	}
	return best, nil
}

const distortionTol = 0.1

// expectedChanges — ожидаемое число изменённых элементов без учёта пар кода
// Хэмминга: чётность — ½ на бит, код Хэмминга — 1 − 2⁻ᵖ на группу.
func expectedChanges(cd coding, bits int) float64 {
	if cd.ham > 1 {
		return float64(bits) / float64(cd.ham) * (1 - math.Ldexp(1, -cd.ham))
	}
	return float64(bits) / 2
}

// maxRS — наибольшее число проверочных байтов на блок, которое перебирает
// модель (у блока остаётся хотя бы один байт данных).
const maxRS = 254

// FlatShare — доля пригодных элементов с одноцветной окрестностью 5×5 (по
// выборке модели): ровные места контейнера, где изменения заметнее всего и
// где программа при извлечении отмечает ложные стирания.
func FlatShare(c *Cover) float64 { return sampleOf(c).flat }
