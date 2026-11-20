package stego

import (
	"errors"
	"math"
	"math/rand"
	"testing"

	"github.com/lirprocs/shorthand/internal/alphabet"
	"github.com/lirprocs/shorthand/internal/crypto/suite"
)

// Модель надёжности совпадает с прототипом, по которому её сверяли с опытом
// (docs/EXPERIMENTS.md, раздел 8): сообщение опыта устойчивости — 120 байт,
// изображение 960×600.
func TestModelMatchesPrototype(t *testing.T) {
	cv := noiseCover(960, 600, 3, 7) // без ровных мест, как фотография
	usable := cv.Len()
	for _, c := range []struct {
		name  string
		cd    coding
		rs    int
		class int
		t     Target
		want  float64
	}{
		// уровни версии 5
		{"повышенная, шум 0,5 %", coding{ham: 4}, 64, 1, Target{Noise: 0.005}, 0.923},
		{"повышенная, шум 0,45 %", coding{ham: 4}, 64, 1, Target{Noise: 0.0045}, 0.976},
		{"повышенная, закраска 1 %", coding{ham: 4}, 64, 1, Target{Paint: 0.01}, 0.994},
		{"повышенная, закраска 2 %", coding{ham: 4}, 64, 1, Target{Paint: 0.02}, 0.004},
		{"максимальная, шум 3 %", coding{h: 1, w: 1}, 128, 2, Target{Noise: 0.03}, 0.950},
		{"максимальная, закраска 7 %", coding{h: 1, w: 1}, 128, 2, Target{Paint: 0.07}, 0.983},
		// варианты опыта (ячейки таблицы раздела 2)
		{"RS 128, w = 4, шум 1 %", coding{h: 1, w: 4}, 128, 2, Target{Noise: 0.01}, 0.30},
		{"RS 128, w = 2, шум 2 %", coding{h: 1, w: 2}, 128, 2, Target{Noise: 0.02}, 0.29},
	} {
		got := reliability(cv, 120, c.t, c.cd, c.rs, c.class, usable)
		if math.Abs(got-c.want) > 0.006 {
			t.Errorf("%s: %.3f, прототип %.3f", c.name, got, c.want)
		}
	}
}

func TestHamBytePerElems(t *testing.T) {
	for p, want := range map[int]float64{1: 8, 2: 12, 4: 30} {
		if got := hamBytePerElems(p); got != want {
			t.Errorf("p = %d: %v, ожидалось %v", p, got, want)
		}
	}
	// p = 3: байты касаются 3, 4 и 3 групп по 7 элементов
	if got := hamBytePerElems(3); math.Abs(got-70.0/3) > 1e-9 {
		t.Errorf("p = 3: %v", got)
	}
}

// Ожидаемое искажение чётности и кода Хэмминга на равных стоимостях:
// чётность — ½ на бит; код Хэмминга без выгодных пар — 1 − 2⁻ᵖ на группу.
func TestExpectedDistortionUniform(t *testing.T) {
	rho := make([]float64, 1<<12)
	for i := range rho {
		rho[i] = 1
	}
	if d := expectedDistortion(rho, coding{h: 1, w: 1}, 100); math.Abs(d-50) > 1e-9 {
		t.Errorf("прямая запись: %v", d)
	}
	if d := expectedDistortion(rho, coding{h: 1, w: 3}, 100); math.Abs(d-50) > 1e-9 {
		t.Errorf("чётность трёх: %v", d)
	}
	if d := expectedDistortion(rho, coding{ham: 4}, 400); math.Abs(d-100*15.0/16) > 1e-9 {
		t.Errorf("код Хэмминга (15, 11): %v", d)
	}
}

// Выбор по модели на «повышенной», «максимальной» и своей цели: надёжность
// не ниже требуемой, код RS — наименьший такой, сообщение помещается и
// извлекается, в заголовке — выбранные параметры. Длинные сообщения доводят
// изображение до предела: тогда — ошибка вместимости.
func TestModelChoiceRoundTrip(t *testing.T) {
	cv := FromImage(testImage(160, 120, 31, false)) // 57 600 элементов
	rng := rand.New(rand.NewSource(15))
	custom := Target{Noise: 0.01, Paint: 0.03}
	for _, c := range []struct {
		lvl    Level
		target *Target
	}{{LevelHigh, nil}, {LevelMax, nil}, {LevelNormal, &custom}} {
		seen := 0
		for n := 30; n <= 4000; n = n * 2 {
			text := randomText(rng, n)
			opt := Options{Mode: suite.MagmaMGM, Level: c.lvl, Target: c.target, Master: testMaster}
			rep, _, err := Plan(cv, text, opt)
			if err != nil {
				var ce *CapacityError
				if !errors.As(err, &ce) || seen == 0 {
					t.Fatalf("%v, %d символов: %v", c.lvl, n, err)
				}
				break
			}
			seen++
			target := c.lvl.Target()
			if c.target != nil {
				target = custom
				if rep.Level != LevelCustom {
					t.Fatalf("своя цель записана как %v", rep.Level)
				}
			}
			if rep.Reliability < 1-modelFail || rep.Used > rep.Elements || rep.HeaderClass == 0 {
				t.Fatalf("%v, %d символов: %+v", c.lvl, n, rep)
			}
			if rep.RS >= 2 {
				if p := reliability(cv, rep.Ciphertext, target, rep.coding(), rep.RS-2, rep.HeaderClass, rep.Elements); p >= 1-modelFail {
					t.Fatalf("%v: код RS %d не наименьший (с %d — %.3f)", c.lvl, rep.RS, rep.RS-2, p)
				}
			}
			out, _, err := EmbedCover(cv, text, opt)
			if err != nil {
				t.Fatal(err)
			}
			got, err := ExtractCover(out, "", nil, testMaster)
			if err != nil || got.Text != alphabet.Normalize(text) {
				t.Fatalf("%v, %d символов: %v", c.lvl, n, err)
			}
			hf, wf := rep.coding().fields()
			if got.Version != 6 || got.RS != rep.RS || got.H != hf || got.W != wf || got.Level != rep.Level {
				t.Fatalf("%v: в заголовке %+v, выбрано %+v", c.lvl, got, rep)
			}
		}
		if seen < 2 {
			t.Errorf("%v: проверено сообщений: %d", c.lvl, seen)
		}
	}
}

// Сообщение, встроенное по цели, переживает повреждения этой цели: шум и
// закраску на пределе «повышенной».
func TestModelTargetSurvivesDamage(t *testing.T) {
	cv := FromImage(testImage(320, 240, 32, false))
	text := "Встреча переносится на четверг, в семь вечера, у главного входа."
	opt := Options{Mode: suite.KuznyechikMGM, Level: LevelHigh, Master: testMaster}
	ok, trials := 0, 20
	for i := 0; i < trials; i++ {
		opt.Nonce = []byte{byte(i), 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
		out, _, err := EmbedCover(cv, text, opt)
		if err != nil {
			t.Fatal(err)
		}
		rng := rand.New(rand.NewSource(int64(i)))
		// шум: ±1 в 0,5 % элементов
		for k := 0; k < out.Len()/200; k++ {
			e := rng.Intn(out.Len())
			if out.Pix[e] == 255 {
				out.Pix[e]--
			} else {
				out.Pix[e]++
			}
		}
		if got, err := ExtractCover(out, "", nil, testMaster); err == nil && got.Text == alphabet.Normalize(text) {
			ok++
		}
	}
	if ok < trials*9/10 {
		t.Fatalf("после шума 0,5 %% извлечено %d из %d", ok, trials)
	}
}

func TestTargetValidation(t *testing.T) {
	cv := FromImage(testImage(64, 48, 33, false))
	for _, tg := range []Target{{Noise: -0.1}, {Paint: 1}, {Noise: math.NaN()}, {Fail: 2}} {
		if _, _, err := Plan(cv, "текст", Options{Mode: suite.KuznyechikMGM, Target: &tg}); err == nil {
			t.Errorf("%+v: ошибки нет", tg)
		}
	}
	// повреждения, которых не выдерживает и самый сильный заголовок
	if _, _, err := Plan(cv, "текст", Options{Mode: suite.KuznyechikMGM, Target: &Target{Paint: 0.4}}); !errors.Is(err, ErrTarget) {
		t.Errorf("закраска 40 %%: %v", err)
	}
}
