package stego

import (
	"errors"
	"math"
	"math/rand"
	"strings"
	"testing"

	"github.com/lirprocs/shorthand/internal/alphabet"
	"github.com/lirprocs/shorthand/internal/crypto/suite"
)

func randomBits(rng *rand.Rand, n int) []uint8 {
	b := make([]uint8, n)
	for i := range b {
		b[i] = uint8(rng.Intn(2))
	}
	return b
}

func TestHamElems(t *testing.T) {
	for _, c := range []struct{ bits, p, want int }{
		{8, 4, 30}, {720, 4, 2700}, {8, 1, 8}, {8, 2, 12}, {8, 8, 255},
		{9, 4, 31},  // две группы по 15 и хвост из 1 бита: 1 элемент
		{10, 3, 22}, // три группы по 7 и хвост из 1 бита
		{11, 3, 24}, // три группы по 7 и хвост из 2 бит: 3 элемента
		{16, 5, 94}, // три группы по 31 и хвост из 1 бита
		{0, 4, 0},
	} {
		if got := hamElems(c.bits, c.p); got != c.want {
			t.Errorf("hamElems(%d, %d) = %d, ожидалось %d", c.bits, c.p, got, c.want)
		}
	}
}

// Запись и чтение при любых p и длинах, в том числе не кратных p; на группу —
// не больше двух изменений.
func TestHamRoundTrip(t *testing.T) {
	rng := rand.New(rand.NewSource(11))
	for p := 1; p <= hamMaxP; p++ {
		for _, bits := range []int{1, 7, 8, 64, 333, 720} {
			n := hamElems(bits, p)
			x := randomBits(rng, n+5) // контейнер может быть длиннее нужного
			rho := make([]float64, n+5)
			for i := range rho {
				rho[i] = math.Exp(rng.NormFloat64() * 3)
			}
			msg := randomBits(rng, bits)
			y := hamEmbed(x, rho, msg, p)
			if len(y) != n {
				t.Fatalf("p = %d, %d бит: длина %d, ожидалось %d", p, bits, len(y), n)
			}
			got := hamExtract(y, bits, p)
			for i := range msg {
				if got[i] != msg[i] {
					t.Fatalf("p = %d, %d бит: бит %d не совпал", p, bits, i)
				}
			}
			group := 1<<p - 1
			for off := 0; off < n; off += group {
				ch := 0
				for i := off; i < min(off+group, n); i++ {
					if x[i] != y[i] {
						ch++
					}
				}
				if ch > 2 {
					t.Fatalf("p = %d: в группе %d изменений", p, ch)
				}
			}
		}
	}
}

// Выбор между одиночным изменением и парой: при равных стоимостях — всегда
// одно изменение (15/16 на группу из 15); дорогой нужный элемент обходится
// парой дешёвых.
func TestHamChoosesCheapest(t *testing.T) {
	rng := rand.New(rand.NewSource(12))
	const p, groups = 4, 4000
	n := hamElems(groups*p, p)
	x := randomBits(rng, n)
	msg := randomBits(rng, groups*p)
	rho := make([]float64, n)
	for i := range rho {
		rho[i] = 1
	}
	y := hamEmbed(x, rho, msg, p)
	changes := 0
	for i := range x {
		if x[i] != y[i] {
			changes++
		}
	}
	if perGroup := float64(changes) / groups; math.Abs(perGroup-15.0/16) > 0.02 {
		t.Fatalf("равные стоимости: %.3f изменения на группу, ожидалось 15/16", perGroup)
	}

	// в каждой группе один элемент «в ровной области». Если нужен именно он,
	// ту же разность даёт любая из семи пар остальных элементов, поэтому
	// дорогой элемент не меняется никогда, а изменений в такой группе два
	for g := 0; g < groups; g++ {
		rho[g*15+rng.Intn(15)] = 1e10
	}
	y = hamEmbed(x, rho, msg, p)
	expensive, pairs := 0, 0
	for g := 0; g < groups; g++ {
		ch := 0
		for i := g * 15; i < g*15+15; i++ {
			if x[i] != y[i] {
				ch++
				if rho[i] > 1 {
					expensive++
				}
			}
		}
		if ch == 2 {
			pairs++
		}
	}
	if expensive != 0 {
		t.Fatalf("изменено %d дорогих элементов", expensive)
	}
	// пара нужна, когда разность указывает на дорогой элемент: 1 группа из 16
	if frac := float64(pairs) / groups; math.Abs(frac-1.0/16) > 0.02 {
		t.Fatalf("пар %.3f на группу, ожидалось 1/16", frac)
	}
	got := hamExtract(y, len(msg), p)
	for i := range msg {
		if got[i] != msg[i] {
			t.Fatal("сообщение не совпало")
		}
	}

	// пара выбирается, только если она дешевле одиночного хотя бы вдвое
	x3, r3 := []uint8{0, 0, 0}, []float64{1.9, 1, 1} // p = 2: нужный элемент 1 стоит 1,9, пара (2, 3) — 2
	if y := hamEmbed(x3, r3, []uint8{0, 1}, 2); y[0] != 1 || y[1] != 0 || y[2] != 0 {
		t.Fatalf("пара дороже одиночного: %v", y)
	}
	r3[0] = 4.1 // пара (2, 3) стоит 2 — дешевле больше чем вдвое
	if y := hamEmbed(x3, r3, []uint8{0, 1}, 2); y[0] != 0 || y[1] != 1 || y[2] != 1 {
		t.Fatalf("пара дешевле одиночного больше чем вдвое: %v", y)
	}
}

// Порча элемента меняет только биты своей группы — те, что называет
// hamInfluence.
func TestHamInfluence(t *testing.T) {
	rng := rand.New(rand.NewSource(13))
	for _, p := range []int{1, 2, 3, 4, 5} {
		for _, bits := range []int{8, 64, 100, 101} {
			n := hamElems(bits, p)
			y := randomBits(rng, n)
			base := hamExtract(y, bits, p)
			for t0 := 0; t0 < n; t0++ {
				y[t0] ^= 1
				got := hamExtract(y, bits, p)
				y[t0] ^= 1
				first, last := hamInfluence(t0, bits, p)
				changed := 0
				for i := range got {
					if got[i] != base[i] {
						changed++
						if i < first || i > last {
							t.Fatalf("p = %d, %d бит: элемент %d изменил бит %d вне [%d, %d]", p, bits, t0, i, first, last)
						}
					}
				}
				if changed == 0 {
					t.Fatalf("p = %d: порча элемента %d не изменила сообщение", p, t0)
				}
			}
		}
	}
}

// randomText — несжимаемый текст заданной длины: случайные буквы и пробелы.
func randomText(rng *rand.Rand, n int) string {
	letters := []rune("абвгдежзиклмнопрстуфхцчшщыьэюя abcdefghijklmnopqrstuvwxyz0123456789.,!?")
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteRune(letters[rng.Intn(len(letters))])
	}
	return b.String()
}

// «Повышенная»: пока сообщение помещается, берётся код (15, 11); по мере
// заполнения изображения — более короткие коды, вплоть до прямой записи.
// Каждое сообщение должно извлекаться.
func TestHighLevelFallsBackToShorterCodes(t *testing.T) {
	cv := FromImage(testImage(120, 90, 21, false)) // 32 400 элементов
	rng := rand.New(rand.NewSource(14))
	seen := map[int]bool{}
	for n := 200; n <= 9000; n += 200 {
		text := randomText(rng, n)
		opt := Options{Mode: suite.KuznyechikMGM, Level: LevelHigh, Master: testMaster}
		rep, _, err := Plan(cv, text, opt)
		if err != nil {
			var ce *CapacityError
			if !errors.As(err, &ce) {
				t.Fatal(err)
			}
			break
		}
		if rep.Ham < 1 || rep.Ham > 4 || rep.H != 0 || rep.W != 0 {
			t.Fatalf("%d символов: h = %d, w = %d, ham = %d", n, rep.H, rep.W, rep.Ham)
		}
		if rep.Used > rep.Elements {
			t.Fatalf("занято %d из %d", rep.Used, rep.Elements)
		}
		// более длинный код не поместился бы
		if rep.Ham < 4 && styleHigh.elems(rep.Elements)+hamElems(rep.MsgBits, rep.Ham+1) <= rep.Elements {
			t.Fatalf("%d символов: выбран p = %d, хотя помещается p = %d", n, rep.Ham, rep.Ham+1)
		}
		if seen[rep.Ham] {
			continue
		}
		seen[rep.Ham] = true
		out, _, err := EmbedCover(cv, text, opt)
		if err != nil {
			t.Fatalf("p = %d: %v", rep.Ham, err)
		}
		got, err := ExtractCover(out, "", nil, testMaster)
		if err != nil || got.Text != alphabet.Normalize(text) {
			t.Fatalf("p = %d (%d символов): %v", rep.Ham, n, err)
		}
		if got.H != hamCode+rep.Ham || got.W != 1 || got.Version != formatVersion {
			t.Fatalf("p = %d: в заголовке h = %d, w = %d, версия %d", rep.Ham, got.H, got.W, got.Version)
		}
	}
	for p := 1; p <= 4; p++ {
		if !seen[p] {
			t.Errorf("код с p = %d ни разу не выбран", p)
		}
	}
}

// «Обычная»: короткое сообщение в большом изображении пишется с шириной 255,
// в маленьком — заголовок узкий (w = 8), и всё читается.
func TestNormalLevelWidths(t *testing.T) {
	big := FromImage(testImage(320, 240, 22, false))
	rep, _, err := Plan(big, "Привет!", Options{Mode: suite.AsconAEAD128, Master: testMaster})
	if err != nil || rep.W != maxW || rep.H != 10 || rep.Ham != 0 || rep.Coded != rep.Ciphertext {
		t.Fatalf("большое изображение: %+v, %v", rep, err)
	}
	if hw := styleNormal.coding(rep.Elements).w; hw != 64 {
		t.Fatalf("ширина заголовка %d", hw)
	}

	small := FromImage(testImage(40, 30, 23, false)) // 3600 элементов
	out, rep, err := EmbedCover(small, "Привет!", Options{Mode: suite.AsconAEAD128, Master: testMaster})
	if err != nil {
		t.Fatal(err)
	}
	if hw := styleNormal.coding(rep.Elements).w; hw != 8 {
		t.Fatalf("маленькое изображение: ширина заголовка %d", hw)
	}
	if rep.W < 1 || rep.W >= 64 || rep.Used > rep.Elements {
		t.Fatalf("маленькое изображение: w = %d, занято %d из %d", rep.W, rep.Used, rep.Elements)
	}
	if got, err := ExtractCover(out, "", nil, testMaster); err != nil || got.Text != "привет!" {
		t.Fatalf("маленькое изображение: %v", err)
	}
}

// На «обычной» у сообщения нет кода Рида–Соломона: порча одного элемента
// сообщения даёт ошибку имитовставки, а не неверный текст.
func TestNormalLevelDamageIsDetected(t *testing.T) {
	cv := FromImage(testImage(200, 150, 24, false))
	out, rep, err := EmbedCover(cv, longText, Options{Mode: suite.KuznyechikMGM, Master: testMaster})
	if err != nil {
		t.Fatal(err)
	}
	k := keys{master: testMaster}
	pos, ok := positions(k.perm(suite.KuznyechikMGM, out.Len()), out, rep.Used)
	if !ok {
		t.Fatal("позиции")
	}
	hElems := styleNormal.elems(rep.Elements)
	for _, t0 := range []int{hElems, hElems + 1000, rep.Used - 1} {
		bad := out.Clone()
		bad.Pix[pos[t0]] ^= 1
		res, err := ExtractCover(bad, "", nil, testMaster)
		if !errors.Is(err, ErrAuth) || res == nil || res.Text != "" {
			t.Fatalf("порча элемента %d: %v", t0, err)
		}
	}
	// порча одного элемента заголовка исправляется его кодом
	bad := out.Clone()
	bad.Pix[pos[5]] ^= 1
	if got, err := ExtractCover(bad, "", nil, testMaster); err != nil || got.Text != alphabet.Normalize(longText) {
		t.Fatalf("порча элемента заголовка: %v", err)
	}
}
