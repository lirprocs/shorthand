// Package precheck — проверка контейнера перед встраиванием и оценка риска.
//
// Этап 1 «на потом» (ветка later/stage1, в v3 не входит).
//
// Самая частая утечка — не алгоритм, а контейнер:
//
//   - PNG, полученный из JPEG (почти любое фото с телефона). У распакованного
//     JPEG коэффициенты ДКП блоков 8×8 лежат у кратных шага квантования;
//     любое изменение на ±1 нарушает эту «совместимость с JPEG», и
//     встраивание обнаруживается даже по нескольким пикселям (J. Fridrich,
//     M. Goljan, R. Du, «Steganalysis based on JPEG compatibility», 2001).
//     Лечение — уменьшить изображение вдвое (Downscale2): сетка 8×8 и шаги
//     квантования исчезают;
//   - картинки без текстуры (скриншоты, графика, ровный фон): адаптивному
//     встраиванию некуда прятать изменения;
//   - слишком большое сообщение относительно картинки.
package precheck

import (
	"fmt"
	"math"

	"github.com/lirprocs/shorthand/internal/stego"
)

// ─── следы JPEG ─────────────────────────────────────────────────────────────

// Коэффициенты ДКП (u, v), по которым ищется квантование: низкие частоты
// есть почти в каждом блоке, средние квантуются крупнее — при высоком
// качестве (90–95) шаг низких частот бывает 1, и тогда видны только средние.
// Качество 97–100 (шаг 1 почти везде) этим способом не распознаётся.
var probeCoefs = [][2]int{
	{0, 1}, {1, 0}, {1, 1}, {0, 2}, {2, 0},
	{2, 2}, {1, 3}, {3, 1}, {3, 3}, {2, 4}, {4, 2},
}

var dctCos [8][8]float64 // dctCos[u][x] = c(u)·cos((2x+1)uπ/16)

func init() {
	for u := 0; u < 8; u++ {
		c := math.Sqrt(2.0 / 8)
		if u == 0 {
			c = math.Sqrt(1.0 / 8)
		}
		for x := 0; x < 8; x++ {
			dctCos[u][x] = c * math.Cos(float64(2*x+1)*float64(u)*math.Pi/16)
		}
	}
}

// luma — яркость по формуле JPEG (Y из YCbCr, ITU-R BT.601).
func luma(c *stego.Cover) []float64 {
	n := c.W * c.H
	y := make([]float64, n)
	for i := 0; i < n; i++ {
		if c.C == 1 {
			y[i] = float64(c.Pix[i])
		} else {
			y[i] = 0.299*float64(c.Pix[3*i]) + 0.587*float64(c.Pix[3*i+1]) + 0.114*float64(c.Pix[3*i+2])
		}
	}
	return y
}

// coefs — значения коэффициентов probeCoefs для блоков 8×8 со сдвигом сетки
// off (каждый step-й блок по каждой оси); нули (|c| < 1,5) не берутся: ноль
// «кратен» любому шагу.
func coefs(y []float64, w, h int, off [2]int, step int) [][]float64 {
	vals := make([][]float64, len(probeCoefs))
	for y0 := off[1]; y0+8 <= h; y0 += 8 * step {
		for x0 := off[0]; x0+8 <= w; x0 += 8 * step {
			for k, uv := range probeCoefs {
				u, v := uv[0], uv[1] // u — по горизонтали, v — по вертикали
				s := 0.0
				for yy := 0; yy < 8; yy++ {
					row := y[(y0+yy)*w+x0:]
					cv := dctCos[v][yy]
					for xx := 0; xx < 8; xx++ {
						s += (row[xx] - 128) * dctCos[u][xx] * cv
					}
				}
				if math.Abs(s) >= 1.5 {
					vals[k] = append(vals[k], s)
				}
			}
		}
	}
	return vals
}

// charAbs — модуль эмпирической характеристической функции на частоте f:
// |среднее exp(2πi·f·c)|.
func charAbs(vals []float64, f float64) float64 {
	var re, im float64
	for _, c := range vals {
		s, co := math.Sincos(2 * math.Pi * f * c)
		re += co
		im += s
	}
	return math.Hypot(re, im) / float64(len(vals))
}

// combScore — насколько значения лежат у кратных шага: наибольшая по q ∈
// [2, 16] разность |φ(1/q)| − max(|φ(0,85/q)|, |φ(1,15/q)|). У квантованных
// значений φ имеет острый пик ровно на 1/q; у гладкого распределения (в том
// числе узкого, у ровных картинок) значения на соседних частотах те же, и
// разность около нуля.
func combScore(vals []float64) (best float64, q int) {
	if len(vals) < 100 {
		return 0, 0
	}
	noise := 3 / math.Sqrt(float64(len(vals)))
	for qq := 2; qq <= 16; qq++ {
		f := 1 / float64(qq)
		d := charAbs(vals, f) - math.Max(charAbs(vals, 0.85*f), charAbs(vals, 1.15*f)) - noise
		if d > best {
			best, q = d, qq
		}
	}
	return best, q
}

// JPEGScore — насколько изображение похоже на распакованный JPEG.
// Сначала по немногим блокам ищется сдвиг сетки 8×8 (из 64), затем на нём
// оценка уточняется по большему числу блоков. Оценка — combScore, наибольшая
// по пяти низкочастотным коэффициентам ДКП яркости. Возвращает оценку,
// найденный шаг квантования и сдвиг сетки.
func JPEGScore(c *stego.Cover) (score float64, q int, off [2]int) {
	y := luma(c)
	w, h := c.W, c.H
	nb := (w / 8) * (h / 8)
	if nb < 64 {
		return 0, 0, off
	}
	stepFor := func(blocks int) int { return max(1, int(math.Sqrt(float64(nb)/float64(blocks)))) }
	best := -1.0
	for oy := 0; oy < 8; oy++ {
		for ox := 0; ox < 8; ox++ {
			o := [2]int{ox, oy}
			for _, v := range coefs(y, w, h, o, stepFor(400)) {
				if s, _ := combScore(v); s > best {
					best, off = s, o
				}
			}
		}
	}
	for _, v := range coefs(y, w, h, off, stepFor(4000)) {
		if s, qq := combScore(v); s > score {
			score, q = s, qq
		}
	}
	return score, q, off
}

// JPEGThreshold — порог JPEGScore. Подобран командой «stegobench precheck»
// (07.11.2026): 15 обоев Windows, сохранённых из JPEG в PNG, — 0,149…0,518;
// они же, уменьшенные вдвое, — 0…0,033.
const JPEGThreshold = 0.08

// ─── текстура ───────────────────────────────────────────────────────────────

// TextureShare — доля пикселей с заметной текстурой: модуль остатка фильтра
// KB (как в HILL) по яркости не меньше 4. У детальных фотографий 0,3…0,9, у
// снимков с размытым фоном 0,05…0,2, у скриншотов и графики — около нуля.
func TextureShare(c *stego.Cover) float64 {
	y := luma(c)
	w, h := c.W, c.H
	if w < 3 || h < 3 {
		return 0
	}
	n, tex := 0, 0
	for yy := 1; yy < h-1; yy++ {
		for xx := 1; xx < w-1; xx++ {
			i := yy*w + xx
			r := -y[i-w-1] + 2*y[i-w] - y[i-w+1] + 2*y[i-1] - 4*y[i] + 2*y[i+1] - y[i+w-1] + 2*y[i+w] - y[i+w+1]
			if math.Abs(r) >= 4 {
				tex++
			}
			n++
		}
	}
	return float64(tex) / float64(n)
}

// TextureThreshold — ниже этой доли контейнер считается бедным текстурой.
const TextureThreshold = 0.15

// ─── уменьшение ─────────────────────────────────────────────────────────────

// Downscale2 уменьшает контейнер вдвое усреднением блоков 2×2 (с округлением):
// следы JPEG (сетка 8×8, шаги квантования) исчезают.
func Downscale2(c *stego.Cover) *stego.Cover {
	w, h := c.W/2, c.H/2
	out := &stego.Cover{W: w, H: h, C: c.C, Pix: make([]uint8, w*h*c.C)}
	if c.Alpha != nil {
		out.Alpha = make([]uint8, w*h)
	}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			p := [4]int{(2*y)*c.W + 2*x, (2*y)*c.W + 2*x + 1, (2*y+1)*c.W + 2*x, (2*y+1)*c.W + 2*x + 1}
			for ch := 0; ch < c.C; ch++ {
				s := 0
				for _, q := range p {
					s += int(c.Pix[q*c.C+ch])
				}
				out.Pix[(y*w+x)*c.C+ch] = uint8((s + 2) / 4)
			}
			if c.Alpha != nil {
				s := 0
				for _, q := range p {
					s += int(c.Alpha[q])
				}
				out.Alpha[y*w+x] = uint8((s + 2) / 4)
			}
		}
	}
	return out
}

// ─── итог ───────────────────────────────────────────────────────────────────

// Report — итог проверки контейнера.
type Report struct {
	JPEG         bool    // похоже на распакованный JPEG
	JPEGScore    float64 //
	JPEGQ        int     // найденный шаг квантования
	Texture      float64 // доля текстурных пикселей
	LowTexture   bool
	Warnings     []string // для человека, по-русски
	CanDownscale bool     // уменьшение вдвое поможет (следы JPEG) и картинка не слишком мала
}

// Check проверяет контейнер.
func Check(c *stego.Cover) *Report {
	r := &Report{}
	r.JPEGScore, r.JPEGQ, _ = JPEGScore(c)
	r.JPEG = r.JPEGScore >= JPEGThreshold
	r.Texture = TextureShare(c)
	r.LowTexture = r.Texture < TextureThreshold
	if r.JPEG {
		r.Warnings = append(r.Warnings, "изображение было сохранено в JPEG — изменения выдаст проверка совместимости с JPEG; уменьшите его вдвое («Подготовить»)")
		r.CanDownscale = c.W >= 256 && c.H >= 256
	}
	if r.LowTexture {
		r.Warnings = append(r.Warnings, fmt.Sprintf("мало текстуры (%.0f%% площади): скриншот, графика или ровный фон — возьмите фотографию", 100*r.Texture))
	}
	return r
}

// ─── риск обнаружения ───────────────────────────────────────────────────────

// Risk — уровень риска обнаружения.
type Risk int

const (
	RiskLow Risk = iota
	RiskMedium
	RiskHigh
)

func (r Risk) String() string {
	return [...]string{"низкий", "средний", "высокий"}[r]
}

// Пороги нагрузки (бит шифротекста на пригодный элемент) — по опытам
// docs/EXPERIMENTS.md (обучаемый детектор SPAM+ЛДФ, AUC): уровень «обычная»
// при 0,01 — 0,53, при 0,05 — 0,60, при 0,1 — 0,65; «повышенная» и
// «максимальная» заметнее (0,70+ уже при 0,005: неадаптивный заголовок
// и h = 1).
const (
	RateLow    = 0.02
	RateMedium = 0.1
)

// RiskOf оценивает риск по плану встраивания и проверке контейнера.
func RiskOf(rep *stego.Report, cr *Report) (Risk, float64) {
	rate := 8 * float64(rep.Ciphertext) / float64(rep.Elements)
	risk := RiskLow
	switch {
	case rate > RateMedium:
		risk = RiskHigh
	case rate > RateLow:
		risk = RiskMedium
	}
	if rep.Level != stego.LevelNormal && risk < RiskMedium {
		risk = RiskMedium
	}
	if cr != nil && cr.JPEG {
		risk = RiskHigh
	}
	if cr != nil && cr.LowTexture && risk < RiskHigh {
		risk++
	}
	return risk, rate
}
