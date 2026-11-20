package main

// Опыт на большом наборе: BOSSbase 1.01 (10 000 серых изображений 512×512,
// PGM) против признаков SRM (34 671) и классификатора-ансамбля
// (internal/analysis/ensemble.go). Признаки считает внешняя программа SRM
// авторов (DDE Lab) по спискам, которые пишет команда boss; ансамбль читает
// её двоичный вывод (srm_bin: N строк по D значений float32).
//
//	stegobench boss -data BOSSbase_1.01 -n 2000 -method v3 -rate 0.1 -out stego/v3_0.1
//	stegobench ensemble -cover cover.bin -stego v3_0.1.bin -name "v3 0.1" -out results

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lirprocs/shorthand/internal/analysis"
	"github.com/lirprocs/shorthand/internal/stego"
)

// ─── PGM ────────────────────────────────────────────────────────────────────

func readPGM(path string) (*stego.Cover, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	r := bufio.NewReader(bytes.NewReader(b))
	var fields []int
	magic := ""
	for len(fields) < 3 {
		tok, err := pgmToken(r)
		if err != nil {
			return nil, fmt.Errorf("%s: заголовок PGM: %v", path, err)
		}
		if magic == "" {
			magic = tok
			if magic != "P5" {
				return nil, fmt.Errorf("%s: не PGM P5", path)
			}
			continue
		}
		v, err := strconv.Atoi(tok)
		if err != nil {
			return nil, fmt.Errorf("%s: заголовок PGM: %v", path, err)
		}
		fields = append(fields, v)
	}
	w, h, maxv := fields[0], fields[1], fields[2]
	if maxv != 255 || w <= 0 || h <= 0 {
		return nil, fmt.Errorf("%s: нужен 8-битный PGM", path)
	}
	pix := make([]uint8, w*h)
	if _, err := io.ReadFull(r, pix); err != nil {
		return nil, fmt.Errorf("%s: данные PGM: %v", path, err)
	}
	return &stego.Cover{W: w, H: h, C: 1, Pix: pix}, nil
}

// pgmToken — следующее слово заголовка; после последнего поля съедается ровно
// один пробельный символ (как требует формат).
func pgmToken(r *bufio.Reader) (string, error) {
	var sb strings.Builder
	for {
		c, err := r.ReadByte()
		if err != nil {
			return "", err
		}
		switch {
		case c == '#':
			if _, err := r.ReadString('\n'); err != nil {
				return "", err
			}
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			if sb.Len() > 0 {
				return sb.String(), nil
			}
		default:
			sb.WriteByte(c)
		}
	}
}

func writePGM(path string, cv *stego.Cover) error {
	if cv.C != 1 {
		return errors.New("PGM — только серые изображения")
	}
	var b bytes.Buffer
	fmt.Fprintf(&b, "P5\n%d %d\n255\n", cv.W, cv.H)
	b.Write(cv.Pix)
	return os.WriteFile(path, b.Bytes(), 0o644)
}

// bossList — первые n изображений набора по номеру (1.pgm, 2.pgm, …).
func bossList(dir string, n int) ([]string, error) {
	files, _ := filepath.Glob(filepath.Join(dir, "*.pgm"))
	num := func(f string) int {
		v, _ := strconv.Atoi(strings.TrimSuffix(filepath.Base(f), ".pgm"))
		return v
	}
	sort.Slice(files, func(i, j int) bool { return num(files[i]) < num(files[j]) })
	if len(files) < n || len(files) == 0 {
		return nil, fmt.Errorf("в %s %d изображений PGM, нужно %d", dir, len(files), n)
	}
	if n <= 0 {
		n = len(files)
	}
	return files[:n], nil
}

// lsbReplace — замена младшего бита в случайных элементах (схема записки НИР,
// для серых изображений).
func lsbReplace(cv *stego.Cover, bits int, rng *rand.Rand) (*stego.Cover, int, error) {
	out := cv.Clone()
	changes := 0
	for _, e := range randomPositions(cv.Len(), bits, rng, nil) {
		nv := out.Pix[e]&^1 | uint8(rng.Intn(2))
		if nv != out.Pix[e] {
			changes++
		}
		out.Pix[e] = nv
	}
	return out, changes, nil
}

var bossMethods = map[string]func(*stego.Cover, int, *rand.Rand) (*stego.Cover, int, error){
	"lsbr": lsbReplace,
	"pm1":  embedMatching,
	"v3":   embedV3(stego.LevelNormal, 0, 0, 0),
	"v3h":  embedV3(stego.LevelHigh, 0, 0, 0),
	"v3m":  embedV3(stego.LevelMax, 0, 0, 0),
	"v5h":  embedV3(stego.LevelHigh, 0, 0, 4),
	"v5m":  embedV3(stego.LevelMax, 1, 1, 0),
}

// bossEmbed встраивает случайную нагрузку rate (бит на элемент) способом
// method в первые n изображений набора и пишет стегоизображения PGM и список
// list.txt (в том же порядке, что у пустых). method = cover — только список
// пустых изображений.
func bossEmbed(dir string, n int, method string, rate float64, out string) error {
	files, err := bossList(dir, n)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	list := make([]string, len(files))
	if method == "cover" {
		for i, f := range files {
			list[i], _ = filepath.Abs(f)
		}
		return os.WriteFile(filepath.Join(out, "list.txt"), []byte(strings.Join(list, "\n")+"\n"), 0o644)
	}
	embed, ok := bossMethods[method]
	if !ok {
		return fmt.Errorf("неизвестный способ %q", method)
	}
	t0 := time.Now()
	var changes, fails atomic.Int64
	var firstErr atomic.Value
	jobs := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < runtime.NumCPU(); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				cv, err := readPGM(files[i])
				if err == nil {
					// зерно — номер изображения и способ: опыт повторяется
					rng := rand.New(rand.NewSource(int64(i)*1000 + int64(len(method))*7 + int64(rate*1e6)))
					var st *stego.Cover
					var ch int
					if st, ch, err = embed(cv, int(rate*float64(cv.Len())), rng); err == nil {
						changes.Add(int64(ch))
						p := filepath.Join(out, filepath.Base(files[i]))
						if err = writePGM(p, st); err == nil {
							list[i], _ = filepath.Abs(p)
						}
					}
				}
				if err != nil {
					fails.Add(1)
					firstErr.CompareAndSwap(nil, err.Error())
				}
			}
		}()
	}
	for i := range files {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	if fails.Load() > 0 {
		return fmt.Errorf("%s %g: не встроилось %d из %d (%v)", method, rate, fails.Load(), len(files), firstErr.Load())
	}
	if err := os.WriteFile(filepath.Join(out, "list.txt"), []byte(strings.Join(list, "\n")+"\n"), 0o644); err != nil {
		return err
	}
	info := fmt.Sprintf("method=%s rate=%g images=%d changed=%.4f%% time=%s\n", method, rate, len(files),
		100*float64(changes.Load())/float64(len(files)*512*512), time.Since(t0).Round(time.Second))
	fmt.Print(info)
	return os.WriteFile(filepath.Join(out, "info.txt"), []byte(info), 0o644)
}

// ─── ансамбль ───────────────────────────────────────────────────────────────

// readFeatures — двоичный вывод srm_bin: N строк по dim значений float32 и
// имена изображений (файл .names рядом).
func readFeatures(bin string, dim int) ([][]float32, []string, error) {
	b, err := os.ReadFile(bin)
	if err != nil {
		return nil, nil, err
	}
	if len(b)%(4*dim) != 0 {
		return nil, nil, fmt.Errorf("%s: размер %d не кратен %d признакам", bin, len(b), dim)
	}
	n := len(b) / (4 * dim)
	rows := make([][]float32, n)
	for i := range rows {
		r := make([]float32, dim)
		for j := range r {
			r[j] = math.Float32frombits(binary.LittleEndian.Uint32(b[4*(i*dim+j):]))
		}
		rows[i] = r
	}
	nb, err := os.ReadFile(strings.TrimSuffix(bin, ".bin") + ".names")
	if err != nil {
		return nil, nil, err
	}
	names := strings.Fields(string(nb))
	if len(names) != n {
		return nil, nil, fmt.Errorf("%s: %d строк признаков и %d имён", bin, n, len(names))
	}
	for i := range names {
		names[i] = filepath.Base(names[i])
	}
	return rows, names, nil
}

// ensembleReport — ошибка ансамбля на splits случайных разбиениях пар пополам
// (обучение / проверка). dsub = 0 — подбор по ошибке вне мешка на первом
// разбиении из сетки dsubGrid.
func ensembleReport(coverBin, stegoBin string, dim, splits, dsub int, name, out string) error {
	c, cn, err := readFeatures(coverBin, dim)
	if err != nil {
		return err
	}
	s, sn, err := readFeatures(stegoBin, dim)
	if err != nil {
		return err
	}
	if len(c) != len(s) {
		return fmt.Errorf("пустых %d, стего %d", len(c), len(s))
	}
	for i := range cn {
		if cn[i] != sn[i] {
			return fmt.Errorf("пара %d: %s и %s", i, cn[i], sn[i])
		}
	}
	n := len(c)
	workers := runtime.NumCPU()
	split := func(seed int64) (trc, trs, tec, tes [][]float32) {
		p := rand.New(rand.NewSource(seed)).Perm(n)
		for k, i := range p {
			if k < n/2 {
				trc, trs = append(trc, c[i]), append(trs, s[i])
			} else {
				tec, tes = append(tec, c[i]), append(tes, s[i])
			}
		}
		return
	}
	t0 := time.Now()
	var log strings.Builder
	if dsub == 0 {
		trc, trs, _, _ := split(1)
		best := math.Inf(1)
		for _, d := range dsubGrid {
			if d > dim {
				continue
			}
			e, err := analysis.TrainEnsemble(trc, trs, analysis.EnsembleConfig{DSub: d, Seed: 100, Workers: workers})
			if err != nil {
				return err
			}
			fmt.Fprintf(&log, "dsub=%d: OOB %.4f (L=%d); ", d, e.OOB, e.L())
			fmt.Printf("%s: dsub %d — OOB %.4f, L %d (%s)\n", name, d, e.OOB, e.L(), time.Since(t0).Round(time.Second))
			if e.OOB < best {
				best, dsub = e.OOB, d
			}
		}
	}
	var pes, aucs []float64
	ls := 0
	for k := 0; k < splits; k++ {
		trc, trs, tec, tes := split(int64(k + 1))
		e, err := analysis.TrainEnsemble(trc, trs, analysis.EnsembleConfig{DSub: dsub, Seed: int64(200 + k), Workers: workers})
		if err != nil {
			return err
		}
		r := analysis.TestEnsemble(e, tec, tes)
		pes, aucs = append(pes, r.PE), append(aucs, r.AUC)
		ls += r.L
		fmt.Printf("%s: разбиение %d — P_E %.4f, AUC %.4f, OOB %.4f, L %d (%s)\n", name, k+1, r.PE, r.AUC, r.OOB, r.L, time.Since(t0).Round(time.Second))
	}
	mean := func(v []float64) float64 {
		s := 0.0
		for _, x := range v {
			s += x
		}
		return s / float64(len(v))
	}
	sd := func(v []float64) float64 {
		m, s := mean(v), 0.0
		for _, x := range v {
			s += (x - m) * (x - m)
		}
		return math.Sqrt(s / float64(max(len(v)-1, 1)))
	}
	line := fmt.Sprintf("%s\t%d\t%.4f\t%.4f\t%.4f\t%d\t%d\t%s\t%s\n", name, n, mean(pes), sd(pes), mean(aucs), dsub, ls/splits,
		time.Since(t0).Round(time.Second), strings.TrimSpace(log.String()))
	fmt.Print(line)
	f, err := os.OpenFile(filepath.Join(out, "ensemble.tsv"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(line)
	return err
}

// dsubGrid — сетка подбора d_sub. На 2000 изображениях выигрывали 250 и 500, 3000 —
// ни разу (EXPERIMENTS.md, раздел 10), поэтому сетка сдвинута вниз.
var dsubGrid = []int{100, 250, 500, 1000, 2000}

// hillSim — моделирование оптимального троичного встраивания ±1 с
// стоимостями HILL (как в работах по HILL и SRM [Li и др., 2014]): элемент
// меняется на +1 или −1 с вероятностями π± = e^(−λρ±) / (1 + e^(−λρ⁺) + e^(−λρ⁻)),
// λ подбирается так, чтобы сумма троичных энтропий равнялась нагрузке. Это не
// способ Shorthand (там двоичный STC со случайным знаком), а эталон для сверки
// цепочки опыта с опубликованными значениями.
func hillSim(cv *stego.Cover, bits int, rng *rand.Rand) (*stego.Cover, int, error) {
	rho := stego.HILLCosts(cv)
	n := len(rho)
	rp, rm := make([]float64, n), make([]float64, n)
	for i, r := range rho {
		rp[i], rm[i] = r, r
		switch cv.Pix[i] {
		case 255:
			rp[i] = math.Inf(1)
		case 0:
			rm[i] = math.Inf(1)
		}
	}
	probs := func(l float64, i int) (pp, pm float64) {
		ep, em := math.Exp(-l*rp[i]), math.Exp(-l*rm[i])
		z := 1 + ep + em
		return ep / z, em / z
	}
	ent := func(l float64) float64 {
		s := 0.0
		for i := 0; i < n; i++ {
			pp, pm := probs(l, i)
			p0 := 1 - pp - pm
			for _, p := range [3]float64{pp, pm, p0} {
				if p > 0 {
					s -= p * math.Log2(p)
				}
			}
		}
		return s
	}
	m := float64(bits)
	lo, hi := 0.0, 1e-3
	for ent(hi) > m {
		hi *= 2
		if hi > 1e12 {
			return nil, 0, errors.New("hillsim: нагрузка больше ёмкости")
		}
	}
	for it := 0; it < 50; it++ {
		mid := (lo + hi) / 2
		if ent(mid) > m {
			lo = mid
		} else {
			hi = mid
		}
	}
	out := cv.Clone()
	changes := 0
	for i := 0; i < n; i++ {
		pp, pm := probs(hi, i)
		u := rng.Float64()
		switch {
		case u < pp:
			out.Pix[i]++
			changes++
		case u < pp+pm:
			out.Pix[i]--
			changes++
		}
	}
	return out, changes, nil
}

func init() { bossMethods["hillsim"] = hillSim }
