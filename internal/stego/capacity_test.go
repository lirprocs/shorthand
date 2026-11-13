package stego

import (
	"errors"
	"image"
	"math/rand"
	"os"
	"strconv"
	"testing"

	"github.com/lirprocs/shorthand/internal/alphabet"
	"github.com/lirprocs/shorthand/internal/crypto/suite"
)

// Заполнение до предела: на случайных изображениях (от крошечных до средних,
// цветных, серых и с прозрачностью) берётся самый длинный текст, который
// помещается, — он должен извлекаться; текст на символ длиннее должен давать
// ошибку ёмкости ещё в Plan. Так проверяются границы выбора ширины STC, кода
// Хэмминга с короткими группами и ширины заголовка.
//
// Число случаев по умолчанию — 24; больше: SHORTHAND_CAPACITY_CASES=1000.
func TestFillToCapacity(t *testing.T) {
	cases := 24
	if v, err := strconv.Atoi(os.Getenv("SHORTHAND_CAPACITY_CASES")); err == nil && v > 0 {
		cases = v
	}
	rng := rand.New(rand.NewSource(31))
	letters := []rune("абвгдежзиклмнопрстуфхцчшщыьэюя abcdefghijklmnopqrstuvwxyz0123456789.,!?")
	text := func(n int) string {
		r := make([]rune, n)
		for i := range r {
			r[i] = letters[rng.Intn(len(letters))]
		}
		return string(r)
	}
	for c := 0; c < cases; c++ {
		w, h := 8+rng.Intn(120), 8+rng.Intn(90)
		gray := rng.Intn(4) == 0
		img := testImage(w, h, int64(900+c), gray).(*image.NRGBA)
		if !gray && rng.Intn(4) == 0 { // прозрачная полоса сверху
			for y := 0; y < h/3; y++ {
				for x := 0; x < w; x++ {
					img.Pix[img.PixOffset(x, y)+3] = 0
				}
			}
		}
		cv := FromImage(img)
		mode := suite.All[rng.Intn(len(suite.All))]
		lvl := Level(rng.Intn(3))
		opt := Options{Mode: mode, Level: lvl, Master: testMaster}
		// длинный несжимаемый текст; ищется самый длинный префикс, который помещается
		long := text(4000)
		runes := []rune(long)
		fits := func(n int) bool {
			_, _, err := Plan(cv, string(runes[:n]), opt)
			if err != nil && !errors.Is(err, ErrCapacity) {
				t.Fatalf("случай %d: %v", c, err)
			}
			return err == nil
		}
		if !fits(0) {
			continue // не помещается даже пустой текст: заголовок больше изображения
		}
		lo, hi := 0, len(runes)
		if fits(hi) {
			continue // в изображение помещается весь текст — предел не достигнут
		}
		for hi-lo > 1 {
			if mid := (lo + hi) / 2; fits(mid) {
				lo = mid
			} else {
				hi = mid
			}
		}
		// размер после сжатия не монотонен по длине текста: текст на символ
		// короче может сжаться на байт длиннее и не поместиться (на уровнях с
		// кодом RS байт может перейти границу блока), поэтому lo − 1 —
		// только если помещается
		for _, n := range []int{lo, max(lo-1, 0)} {
			if n != lo && !fits(n) {
				continue
			}
			s := string(runes[:n])
			out, rep, err := EmbedCover(cv, s, opt)
			if err != nil {
				t.Fatalf("случай %d (%d×%d, C=%d, %v, %v, %d симв.): встраивание: %v", c, w, h, cv.C, mode, lvl, n, err)
			}
			if rep.Used > rep.Elements {
				t.Fatalf("случай %d: занято %d из %d", c, rep.Used, rep.Elements)
			}
			got, err := ExtractCover(out, "", nil, testMaster)
			if err != nil || got.Text != alphabet.Normalize(s) {
				t.Fatalf("случай %d (%d×%d, C=%d, %v, %v, %d симв., h=%d w=%d ham=%d): извлечение: %v",
					c, w, h, cv.C, mode, lvl, n, rep.H, rep.W, rep.Ham, err)
			}
		}
		// на символ длиннее — ошибка ёмкости, а не сбой при встраивании
		if _, _, err := EmbedCover(cv, string(runes[:hi]), opt); !errors.Is(err, ErrCapacity) {
			t.Fatalf("случай %d: текст сверх предела: %v", c, err)
		}
	}
}
