// stegobench — эксперименты для НИР: стегоанализ, устойчивость, качество,
// скорость шифров, сжатие. Результаты — таблицы Markdown.
//
//	stegobench prepare -out data -src "C:/Windows/Web/Wallpaper/Theme1,..." [-scale 2] [-tile 256]
//	stegobench detect  -data data/tiles -out results
//	stegobench robust  -data data/full  -out results
//	stegobench ciphers -out results
//	stegobench compress -out results [-text файл.txt]
//	stegobench embed -in контейнер.png -out стего.png -method v3|v3h|v3m|pm1|report|v2 -rate 0.05
//	stegobench precheck -data папка1,папка2 -out results   (этап 1, на потом: проверка контейнеров)
package main

import (
	"bytes"
	"compress/gzip"
	"flag"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	mgm "github.com/lirprocs/MGM"

	"github.com/lirprocs/shorthand/internal/alphabet"
	"github.com/lirprocs/shorthand/internal/analysis"
	"github.com/lirprocs/shorthand/internal/compress"
	"github.com/lirprocs/shorthand/internal/crypto/suite"
	"github.com/lirprocs/shorthand/internal/imageio"
	"github.com/lirprocs/shorthand/internal/precheck"
	"github.com/lirprocs/shorthand/internal/stego"
)

var master = bytes.Repeat([]byte{0x42}, 32) // вместо Argon2 — ускоряет опыты

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "команда: prepare | detect | robust | ciphers | compress")
		os.Exit(2)
	}
	cmd, args := os.Args[1], os.Args[2:]
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	src := fs.String("src", "", "папки с фотографиями через запятую (prepare)")
	data := fs.String("data", "", "папка с контейнерами PNG")
	out := fs.String("out", "results", "папка для результатов")
	scale := fs.Int("scale", 2, "уменьшение фотографий (prepare)")
	tile := fs.Int("tile", 256, "размер фрагментов (prepare)")
	textFile := fs.String("text", "", "дополнительный текст (compress)")
	in := fs.String("in", "", "контейнер PNG/BMP (embed)")
	meth := fs.String("method", "v3", "способ встраивания (embed)")
	rate := fs.Float64("rate", 0.05, "нагрузка, бит на элемент (embed)")
	fs.Parse(args)
	if cmd != "embed" {
		os.MkdirAll(*out, 0o755)
	}
	var err error
	switch cmd {
	case "embed":
		err = embedFile(*in, *out, *meth, *rate)
	case "prepare":
		err = prepare(strings.Split(*src, ","), *out, *scale, *tile)
	case "detect":
		err = detect(*data, *out)
	case "precheck":
		err = precheckReport(strings.Split(*data, ","), *out)
	case "robust":
		err = robust(*data, *out)
	case "ciphers":
		err = ciphers(*out)
	case "compress":
		err = compressReport(*out, *textFile)
	default:
		err = fmt.Errorf("неизвестная команда %q", cmd)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "ошибка:", err)
		os.Exit(1)
	}
}

// ─── подготовка контейнеров ─────────────────────────────────────────────────

// prepare уменьшает фотографии в scale раз усреднением (это убирает следы
// JPEG-сжатия: детекторы иначе реагируют на них, а не на встраивание) и
// сохраняет их целиком (out/full) и фрагментами tile×tile (out/tiles).
func prepare(dirs []string, out string, scale, tile int) error {
	os.MkdirAll(filepath.Join(out, "full"), 0o755)
	os.MkdirAll(filepath.Join(out, "tiles"), 0o755)
	nFull, nTiles := 0, 0
	for _, dir := range dirs {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}
		files, _ := filepath.Glob(filepath.Join(dir, "*"))
		sort.Strings(files)
		for _, f := range files {
			ext := strings.ToLower(filepath.Ext(f))
			if ext != ".jpg" && ext != ".jpeg" && ext != ".png" {
				continue
			}
			fh, err := os.Open(f)
			if err != nil {
				continue
			}
			img, _, err := image.Decode(fh)
			fh.Close()
			if err != nil {
				continue
			}
			small := downscale(img, scale)
			b := small.Bounds()
			if b.Dx() < tile || b.Dy() < tile {
				continue
			}
			name := strings.TrimSuffix(filepath.Base(f), filepath.Ext(f))
			name = filepath.Base(dir) + "_" + name
			if err := imageio.Save(filepath.Join(out, "full", name+".png"), small, imageio.PNG); err != nil {
				return err
			}
			nFull++
			for y := 0; y+tile <= b.Dy(); y += tile {
				for x := 0; x+tile <= b.Dx(); x += tile {
					t := small.SubImage(image.Rect(x, y, x+tile, y+tile))
					p := filepath.Join(out, "tiles", fmt.Sprintf("%s_%d_%d.png", name, x, y))
					if err := imageio.Save(p, t, imageio.PNG); err != nil {
						return err
					}
					nTiles++
				}
			}
		}
	}
	fmt.Printf("подготовлено: %d изображений, %d фрагментов\n", nFull, nTiles)
	return nil
}

func downscale(img image.Image, k int) *image.NRGBA {
	b := img.Bounds()
	w, h := b.Dx()/k, b.Dy()/k
	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var r, g, bl uint32
			for dy := 0; dy < k; dy++ {
				for dx := 0; dx < k; dx++ {
					c := color.NRGBAModel.Convert(img.At(b.Min.X+x*k+dx, b.Min.Y+y*k+dy)).(color.NRGBA)
					r += uint32(c.R)
					g += uint32(c.G)
					bl += uint32(c.B)
				}
			}
			n := uint32(k * k)
			out.Set(x, y, color.NRGBA{uint8((r + n/2) / n), uint8((g + n/2) / n), uint8((bl + n/2) / n), 255})
		}
	}
	return out
}

func loadCovers(dir string) ([]*stego.Cover, []string, error) {
	files, _ := filepath.Glob(filepath.Join(dir, "*.png"))
	sort.Strings(files)
	var covers []*stego.Cover
	var names []string
	for _, f := range files {
		img, _, err := imageio.Load(f)
		if err != nil {
			return nil, nil, err
		}
		covers = append(covers, stego.FromImage(img))
		names = append(names, filepath.Base(f))
	}
	if len(covers) == 0 {
		return nil, nil, fmt.Errorf("в %s нет PNG", dir)
	}
	return covers, names, nil
}

// ─── способы встраивания для сравнения ──────────────────────────────────────

type method struct {
	name  string
	embed func(cv *stego.Cover, bits int, rng *rand.Rand) (*stego.Cover, int, error)
}

func randomPositions(n, k int, rng *rand.Rand, ok func(int) bool) []int {
	perm := rng.Perm(n)
	out := make([]int, 0, k)
	for _, p := range perm {
		if len(out) == k {
			break
		}
		if ok == nil || ok(p) {
			out = append(out, p)
		}
	}
	return out
}

// v2 — старая программа: байт сообщения в случайном пикселе, младшие биты
// R (3), G (2), B (3) заменяются битами байта.
func embedV2(cv *stego.Cover, bits int, rng *rand.Rand) (*stego.Cover, int, error) {
	out := cv.Clone()
	nb := bits / 8
	pix := randomPositions(cv.W*cv.H, nb, rng, nil)
	changes := 0
	for _, p := range pix {
		b := byte(rng.Intn(256))
		vals := [3]uint8{b >> 5 & 7, b >> 3 & 3, b & 7}
		masks := [3]uint8{0xf8, 0xfc, 0xf8}
		for c := 0; c < 3; c++ {
			e := p*3 + c
			nv := out.Pix[e]&masks[c] | vals[c]
			if nv != out.Pix[e] {
				changes++
			}
			out.Pix[e] = nv
		}
	}
	return out, changes, nil
}

// метод из записки: бит сообщения — младший бит R или B случайного пикселя (замена).
func embedReportLSB(cv *stego.Cover, bits int, rng *rand.Rand) (*stego.Cover, int, error) {
	out := cv.Clone()
	pix := randomPositions(cv.W*cv.H, bits, rng, nil)
	changes := 0
	for _, p := range pix {
		e := p*3 + 2*rng.Intn(2) // R или B
		nv := out.Pix[e]&^1 | uint8(rng.Intn(2))
		if nv != out.Pix[e] {
			changes++
		}
		out.Pix[e] = nv
	}
	return out, changes, nil
}

// ±1 в случайных элементах без STC и без учёта текстуры.
func embedMatching(cv *stego.Cover, bits int, rng *rand.Rand) (*stego.Cover, int, error) {
	out := cv.Clone()
	changes := 0
	for _, e := range randomPositions(cv.Len(), bits, rng, nil) {
		if rng.Intn(2) == 0 {
			continue // бит уже совпал
		}
		v := out.Pix[e]
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
		out.Pix[e] = v
		changes++
	}
	return out, changes, nil
}

func embedV3(level stego.Level, h, maxW int) func(*stego.Cover, int, *rand.Rand) (*stego.Cover, int, error) {
	return func(cv *stego.Cover, bits int, rng *rand.Rand) (*stego.Cover, int, error) {
		raw := make([]byte, max(1, bits/8-16)) // шифротекст ≈ bits/8 байт вместе с имитовставкой
		rng.Read(raw)
		nonce := make([]byte, 16)
		rng.Read(nonce)
		out, rep, err := stego.EmbedCover(cv, "", stego.Options{
			Mode: suite.KuznyechikMGM, Level: level, Master: master, Nonce: nonce, Raw: raw, H: h, MaxW: maxW,
		})
		if err != nil {
			return nil, 0, err
		}
		return out, rep.Changes + rep.HeaderChanges, nil
	}
}

// embedFile встраивает случайную нагрузку заданным способом и сохраняет
// результат — для рисунков (где именно меняются пиксели).
func embedFile(in, out, name string, rate float64) error {
	byName := map[string]func(*stego.Cover, int, *rand.Rand) (*stego.Cover, int, error){
		"v2": embedV2, "report": embedReportLSB, "pm1": embedMatching,
		"v3": embedV3(stego.LevelNormal, 0, 0), "v3h": embedV3(stego.LevelHigh, 0, 0), "v3m": embedV3(stego.LevelMax, 0, 0),
	}
	f, ok := byName[name]
	if !ok {
		return fmt.Errorf("неизвестный способ %q", name)
	}
	img, format, err := imageio.Load(in)
	if err != nil {
		return err
	}
	cv := stego.FromImage(img)
	st, changes, err := f(cv, int(rate*float64(cv.Len())), rand.New(rand.NewSource(1)))
	if err != nil {
		return err
	}
	fmt.Printf("%s: %d×%d, нагрузка %.3f бит на элемент, изменено %d элементов (%.3f%%)\n",
		name, cv.W, cv.H, rate, changes, 100*float64(changes)/float64(cv.Len()))
	return imageio.Save(out, st.Image(), format)
}

var methods = []method{
	{"v2: 3-2-3, замена", embedV2},
	{"записка: R/B, замена", embedReportLSB},
	{"±1 без STC", embedMatching},
	{"v3 обычная", embedV3(stego.LevelNormal, 0, 0)},
	{"v3 повышенная", embedV3(stego.LevelHigh, 0, 0)},
	{"v3 максимальная", embedV3(stego.LevelMax, 0, 0)},
}

// ─── стегоанализ и качество ─────────────────────────────────────────────────

type scores struct{ rs, spa, chi float64 }

func analyze(cv *stego.Cover) scores {
	pl := analysis.Channels(cv.Pix, cv.W, cv.H, cv.C)
	return scores{analysis.RS(pl), analysis.SPA(pl), analysis.ChiSquare(pl)}
}

type cell struct {
	st                  []scores
	changes, psnr, ssim []float64
	feat                [][]float64 // признаки стеговерсии i-го контейнера (nil — не встроилось)
	fails               int
}

func detect(dir, out string) error {
	covers, _, err := loadCovers(dir)
	if err != nil {
		return err
	}
	rates := []float64{0.005, 0.01, 0.02, 0.05, 0.1, 0.2}
	fmt.Printf("стегоанализ: %d контейнеров, %d способов, %d нагрузок\n", len(covers), len(methods), len(rates))
	t0 := time.Now()
	coverSc := make([]scores, len(covers))
	coverFeat := make([][]float64, len(covers))
	res := make([][]cell, len(methods))
	for m := range res {
		res[m] = make([]cell, len(rates))
		for r := range res[m] {
			res[m][r].feat = make([][]float64, len(covers))
		}
	}
	var mu sync.Mutex
	jobs := make(chan int)
	var wg sync.WaitGroup
	for wk := 0; wk < max(1, runtime.NumCPU()/2); wk++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				cv := covers[i]
				cs := analyze(cv)
				cf := analysis.Features(analysis.Channels(cv.Pix, cv.W, cv.H, cv.C))
				lc := analysis.Luma(cv.Pix, cv.W, cv.H, cv.C)
				mu.Lock()
				coverSc[i], coverFeat[i] = cs, cf
				mu.Unlock()
				for m, me := range methods {
					for r, rate := range rates {
						rng := rand.New(rand.NewSource(int64(i*1000 + m*10 + r)))
						bits := int(rate * float64(cv.Len()))
						st, ch, err := me.embed(cv, bits, rng)
						if err != nil {
							mu.Lock()
							res[m][r].fails++
							mu.Unlock()
							continue
						}
						sc := analyze(st)
						sf := analysis.Features(analysis.Channels(st.Pix, st.W, st.H, st.C))
						ps := analysis.PSNR(cv.Pix, st.Pix)
						ss := analysis.SSIM(lc, analysis.Luma(st.Pix, st.W, st.H, st.C), cv.W, cv.H)
						mu.Lock()
						c := &res[m][r]
						c.st = append(c.st, sc)
						c.changes = append(c.changes, float64(ch)/float64(cv.Len()))
						c.psnr = append(c.psnr, ps)
						c.ssim = append(c.ssim, ss)
						c.feat[i] = sf
						mu.Unlock()
					}
				}
			}
		}()
	}
	for i := range covers {
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	var b strings.Builder
	fmt.Fprintf(&b, "# Стегоанализ и качество\n\n")
	fmt.Fprintf(&b, "Контейнеров: %d (цветные фрагменты %d×%d), время %s.\n\n", len(covers), covers[0].W, covers[0].H, time.Since(t0).Round(time.Second))
	fmt.Fprintf(&b, "Нагрузка — бит сообщения на элемент (канал пикселя). Для v3 это шифротекст с имитовставкой; код Рида–Соломона и заголовок добавляются сверху.\n\n")
	fmt.Fprintf(&b, "AUC — вероятность, что детектор поставит стегоизображению оценку выше, чем пустому: 0,5 — не отличает, 1,0 — отличает всегда.\n\n")
	cr, cs, cc := column(coverSc, func(s scores) float64 { return s.rs }), column(coverSc, func(s scores) float64 { return s.spa }), column(coverSc, func(s scores) float64 { return s.chi })
	fmt.Fprintf(&b, "Пустые контейнеры: RS %.4f, SPA %.4f, χ² %.3f (средние оценки).\n\n", mean(cr), mean(cs), mean(cc))
	fmt.Fprintf(&b, "SPAM+ЛДФ — обучаемый детектор (100 признаков совместной встречаемости остатков, дискриминант Фишера, 10-кратная перекрёстная проверка); видит и ±1.\n\n")
	fmt.Fprintf(&b, "| Способ | Нагрузка | Изменено, %% | RS: оценка / AUC | SPA: оценка / AUC | χ²: AUC | SPAM+ЛДФ: AUC | PSNR, дБ | SSIM |\n")
	fmt.Fprintf(&b, "|---|---|---|---|---|---|---|---|---|\n")
	for m, me := range methods {
		for r, rate := range rates {
			c := res[m][r]
			if len(c.st) == 0 {
				fmt.Fprintf(&b, "| %s | %.3f | не помещается (%d) | | | | | | |\n", me.name, rate, c.fails)
				continue
			}
			sr := column(c.st, func(s scores) float64 { return s.rs })
			ss := column(c.st, func(s scores) float64 { return s.spa })
			sc := column(c.st, func(s scores) float64 { return s.chi })
			var fc, fs [][]float64
			for i, f := range c.feat {
				if f != nil {
					fc, fs = append(fc, coverFeat[i]), append(fs, f)
				}
			}
			fmt.Fprintf(&b, "| %s | %.3f | %.3f | %.4f / %.2f | %.4f / %.2f | %.2f | %.2f | %.1f | %.5f |\n",
				me.name, rate, 100*mean(c.changes), mean(sr), analysis.AUC(cr, sr), mean(ss), analysis.AUC(cs, ss),
				analysis.AUC(cc, sc), analysis.CVAUC(fc, fs, 10, 1), mean(c.psnr), mean(c.ssim))
		}
	}
	return writeReport(out, "detect.md", b.String())
}

func column(s []scores, f func(scores) float64) []float64 {
	out := make([]float64, len(s))
	for i, v := range s {
		out[i] = f(v)
	}
	return out
}

func mean(v []float64) float64 {
	if len(v) == 0 {
		return math.NaN()
	}
	s := 0.0
	for _, x := range v {
		s += x
	}
	return s / float64(len(v))
}

func writeReport(dir, name, text string) error {
	fmt.Print(text)
	return os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644)
}

// ─── устойчивость ───────────────────────────────────────────────────────────

// robustText — сообщение для опытов; в корпус контекстной модели не входит.
const robustText = "Груз прибудет в порт не раньше субботы: шторм задержал паром на двое суток. " +
	"Склад на Лесной закрыт на ремонт, поэтому разгружаемся у второго причала. " +
	"Накладные и сертификаты лежат в синей папке, ключ от сейфа — у Марины. Созвонимся вечером, после девяти."

type variant struct {
	name    string
	level   stego.Level
	h, maxW int
}

type damage struct {
	name  string
	apply func(cv *stego.Cover, rng *rand.Rand) *stego.Cover
}

// paint закрашивает прямоугольник площадью frac однородным цветом.
func paint(frac float64) func(*stego.Cover, *rand.Rand) *stego.Cover {
	return func(cv *stego.Cover, rng *rand.Rand) *stego.Cover {
		out := cv.Clone()
		area := frac * float64(cv.W*cv.H)
		w := int(math.Sqrt(area * 4 / 3))
		h := int(area) / max(w, 1)
		w, h = min(w, cv.W), min(h, cv.H)
		x0, y0 := rng.Intn(cv.W-w+1), rng.Intn(cv.H-h+1)
		var fill [3]uint8
		for c := 0; c < cv.C; c++ {
			fill[c] = cv.Pix[((y0+h/2)*cv.W+x0+w/2)*cv.C+c]
		}
		for y := y0; y < y0+h; y++ {
			for x := x0; x < x0+w; x++ {
				for c := 0; c < cv.C; c++ {
					out.Pix[(y*cv.W+x)*cv.C+c] = fill[c]
				}
			}
		}
		return out
	}
}

// noise меняет на ±1 долю frac случайных элементов (лёгкая обработка).
func noise(frac float64) func(*stego.Cover, *rand.Rand) *stego.Cover {
	return func(cv *stego.Cover, rng *rand.Rand) *stego.Cover {
		out := cv.Clone()
		k := int(frac * float64(cv.Len()))
		for i := 0; i < k; i++ {
			e := rng.Intn(cv.Len())
			v := out.Pix[e]
			if v == 255 || (v > 0 && rng.Intn(2) == 0) {
				v--
			} else {
				v++
			}
			out.Pix[e] = v
		}
		return out
	}
}

func robust(dir, out string) error {
	covers, _, err := loadCovers(dir)
	if err != nil {
		return err
	}
	variants := []variant{
		{"обычная: RS 32, h=10, w≤64", stego.LevelNormal, 0, 0},
		{"RS 32, h=1, w≤8", stego.LevelNormal, 1, 8},
		{"RS 64, h=1, w≤8", stego.LevelHigh, 1, 8},
		{"повышенная: RS 64, h=1, w≤4", stego.LevelHigh, 0, 0},
		{"RS 64, h=4, w≤4", stego.LevelHigh, 4, 4},
		{"RS 128, h=1, w≤4", stego.LevelMax, 1, 4},
		{"RS 128, h=1, w≤2", stego.LevelMax, 1, 2},
		{"максимальная: RS 128, h=1, w=1", stego.LevelMax, 0, 0},
	}
	damages := []damage{{"без повреждений", func(c *stego.Cover, _ *rand.Rand) *stego.Cover { return c }}}
	for _, f := range []float64{0.005, 0.01, 0.02, 0.05, 0.1, 0.2} {
		damages = append(damages, damage{fmt.Sprintf("закрашено %g%%", 100*f), paint(f)})
	}
	for _, f := range []float64{0.001, 0.005, 0.01, 0.02, 0.05} {
		damages = append(damages, damage{fmt.Sprintf("±1 в %g%% элементов", 100*f), noise(f)})
	}
	const trials = 3
	type stat struct{ ok, header, n int }
	res := make([][]stat, len(variants))
	changes := make([][]float64, len(variants))
	for v := range res {
		res[v] = make([]stat, len(damages))
	}
	fmt.Printf("устойчивость: %d изображений, %d вариантов, %d повреждений × %d\n", len(covers), len(variants), len(damages), trials)
	t0 := time.Now()
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, max(1, runtime.NumCPU()/2))
	for i, cv := range covers {
		for v, va := range variants {
			wg.Add(1)
			sem <- struct{}{}
			go func(i, v int, cv *stego.Cover, va variant) {
				defer wg.Done()
				defer func() { <-sem }()
				nonce := make([]byte, 16)
				nonce[1] = byte(i)
				st, rep, err := stego.EmbedCover(cv, robustText, stego.Options{
					Mode: suite.KuznyechikMGM, Level: va.level, Master: master, Nonce: nonce, H: va.h, MaxW: va.maxW,
				})
				if err != nil {
					fmt.Fprintln(os.Stderr, va.name, err)
					return
				}
				mu.Lock()
				changes[v] = append(changes[v], float64(rep.Changes+rep.HeaderChanges))
				mu.Unlock()
				want := alphabet.Normalize(robustText)
				for d, dm := range damages {
					for t := 0; t < trials; t++ {
						rng := rand.New(rand.NewSource(int64(i*100000 + v*1000 + d*10 + t)))
						got, err := stego.ExtractCover(dm.apply(st, rng), "", nil, master)
						mu.Lock()
						s := &res[v][d]
						s.n++
						if err == nil && got.Text == want {
							s.ok++
						}
						if err == nil || got != nil {
							s.header++
						}
						mu.Unlock()
					}
				}
			}(i, v, cv, va)
		}
	}
	wg.Wait()

	var b strings.Builder
	fmt.Fprintf(&b, "# Устойчивость к повреждениям\n\n")
	fmt.Fprintf(&b, "Изображений: %d (%d×%d), сообщение %d символов, по %d опыта на повреждение, время %s.\n\n",
		len(covers), covers[0].W, covers[0].H, len([]rune(robustText)), trials, time.Since(t0).Round(time.Second))
	fmt.Fprintf(&b, "В ячейке: доля успешных извлечений, %%; в скобках — доля случаев, когда заголовок прочитан (программа знает, что сообщение есть).\n\n")
	b.WriteString("| Повреждение |")
	for _, va := range variants {
		b.WriteString(" " + va.name + " |")
	}
	b.WriteString("\n|---|")
	for range variants {
		b.WriteString("---|")
	}
	b.WriteString("\n| изменено элементов (среднее) |")
	for v := range variants {
		fmt.Fprintf(&b, " %.0f |", mean(changes[v]))
	}
	b.WriteString("\n")
	for d, dm := range damages {
		fmt.Fprintf(&b, "| %s |", dm.name)
		for v := range variants {
			s := res[v][d]
			fmt.Fprintf(&b, " %.0f (%.0f) |", 100*float64(s.ok)/float64(max(s.n, 1)), 100*float64(s.header)/float64(max(s.n, 1)))
		}
		b.WriteString("\n")
	}
	return writeReport(out, "robust.md", b.String())
}

// ─── шифры ──────────────────────────────────────────────────────────────────

func ciphers(out string) error {
	var b strings.Builder
	fmt.Fprintf(&b, "# Скорость шифров (Seal: шифрование и имитовставка)\n\n")
	fmt.Fprintf(&b, "Процессор: %d логических ядер, %s/%s, один поток.\n\n", runtime.NumCPU(), runtime.GOOS, runtime.GOARCH)
	sizes := []int{64, 1024, 16384}
	b.WriteString("| Режим | Ключ, бит | Nonce, байт | Имитовставка, байт |")
	for _, s := range sizes {
		fmt.Fprintf(&b, " %d Б, МБ/с |", s)
	}
	b.WriteString("\n|---|---|---|---|")
	for range sizes {
		b.WriteString("---|")
	}
	b.WriteString("\n")
	for _, m := range suite.All {
		a, _ := m.NewAEAD(make([]byte, m.KeySize()))
		fmt.Fprintf(&b, "| %s | %d | %d | %d |", m, 8*m.KeySize(), m.NonceSize(), a.Overhead())
		for _, s := range sizes {
			buf := make([]byte, s)
			nonce := make([]byte, m.NonceSize())
			r := testing.Benchmark(func(tb *testing.B) {
				tb.SetBytes(int64(s))
				for i := 0; i < tb.N; i++ {
					a.Seal(nil, nonce, buf, nil)
				}
			})
			fmt.Fprintf(&b, " %.1f |", float64(s)*float64(r.N)/r.T.Seconds()/1e6)
		}
		b.WriteString("\n")
	}
	// прежний интерфейс библиотеки MGM (ключ «Кузнечика» разворачивается на каждый блок)
	var key [32]byte
	var nonce [16]byte
	fmt.Fprintf(&b, "| Кузнечик-MGM, прежний Encrypt | 256 | 16 | 16 |")
	for _, s := range sizes {
		buf := make([]byte, s)
		if s > 1024 {
			b.WriteString(" — |")
			continue
		}
		r := testing.Benchmark(func(tb *testing.B) {
			tb.SetBytes(int64(s))
			for i := 0; i < tb.N; i++ {
				mgm.Encrypt(nil, buf, key, nonce)
			}
		})
		fmt.Fprintf(&b, " %.2f |", float64(s)*float64(r.N)/r.T.Seconds()/1e6)
	}
	b.WriteString("\n")
	return writeReport(out, "ciphers.md", b.String())
}

// ─── сжатие ─────────────────────────────────────────────────────────────────

var sampleTexts = []string{
	"Привет, мой друг!",
	"Встреча завтра в 18:00 у метро Бауманская, принеси документы.",
	"Код подтверждения: 482913. Никому его не сообщайте.",
	"Сегодня я долго думал о том, что ты сказал вчера. Наверное, ты прав: нам стоит поговорить спокойно, без спешки, " +
		"и решить, как быть дальше. Позвони мне, когда освободишься, я весь вечер буду дома.",
	robustText,
}

func compressReport(out, extra string) error {
	texts := append([]string(nil), sampleTexts...)
	if extra != "" {
		b, err := os.ReadFile(extra)
		if err != nil {
			return err
		}
		texts = append(texts, string(b))
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Сжатие\n\n| Символов | UTF-8, байт | gzip (UTF-8), байт | 7 бит | контекстная модель | deflate | выбрано | бит на символ |\n|---|---|---|---|---|---|---|---|\n")
	for _, t := range texts {
		sym := alphabet.Encode(t)
		var gz bytes.Buffer
		zw, _ := gzip.NewWriterLevel(&gz, gzip.BestCompression)
		zw.Write([]byte(t))
		zw.Close()
		p := len(compress.CompressWith(compress.Packed7, sym))
		c := len(compress.CompressWith(compress.Context, sym))
		d := len(compress.CompressWith(compress.Deflate, sym))
		m, best := compress.Compress(sym)
		n := len([]rune(t))
		fmt.Fprintf(&b, "| %d | %d | %d | %d | %d | %d | %s | %.2f |\n", n, len(t), gz.Len(), p, c, d, m, 8*float64(len(best))/float64(n))
	}
	return writeReport(out, "compress.md", b.String())
}

// ─── проверка контейнеров (этап 1, на потом) ────────────────────────────────

// precheckReport — оценки следов JPEG и текстуры для всех PNG/BMP из папок:
// по ним подобраны пороги precheck.JPEGThreshold и TextureThreshold.
func precheckReport(dirs []string, out string) error {
	var b strings.Builder
	fmt.Fprintf(&b, "# Проверка контейнеров\n\nПорог следов JPEG — %.2f, порог текстуры — %.2f.\n\n", precheck.JPEGThreshold, precheck.TextureThreshold)
	fmt.Fprintf(&b, "| Папка | Файл | Размер | Следы JPEG | Шаг q | Текстура | Время, мс | Вывод |\n|---|---|---|---|---|---|---|---|\n")
	for _, dir := range dirs {
		files, _ := filepath.Glob(filepath.Join(strings.TrimSpace(dir), "*"))
		sort.Strings(files)
		for _, f := range files {
			if _, err := imageio.FormatOf(f); err != nil {
				continue
			}
			img, _, err := imageio.Load(f)
			if err != nil {
				return err
			}
			cv := stego.FromImage(img)
			t0 := time.Now()
			r := precheck.Check(cv)
			ms := time.Since(t0).Milliseconds()
			verdict := "годится"
			switch {
			case r.JPEG && r.LowTexture:
				verdict = "JPEG, мало текстуры"
			case r.JPEG:
				verdict = "был JPEG"
			case r.LowTexture:
				verdict = "мало текстуры"
			}
			fmt.Fprintf(&b, "| %s | %s | %d×%d | %.3f | %d | %.2f | %d | %s |\n",
				filepath.Base(dir), filepath.Base(f), cv.W, cv.H, r.JPEGScore, r.JPEGQ, r.Texture, ms, verdict)
		}
	}
	return writeReport(out, "precheck.md", b.String())
}
