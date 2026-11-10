package stego

import (
	"bytes"
	"errors"
	"fmt"
	"math/rand"
	"runtime"
	"testing"

	"github.com/lirprocs/shorthand/internal/crypto/suite"
	"github.com/lirprocs/shorthand/internal/ecc/rs"
)

// noiseCover — контейнер из независимого шума: у разных seed нет ничего общего.
func noiseCover(w, h, c int, seed int64) *Cover {
	r := rand.New(rand.NewSource(seed))
	cv := &Cover{W: w, H: h, C: c, Pix: make([]uint8, w*h*c)}
	for i := range cv.Pix {
		cv.Pix[i] = uint8(40 + r.Intn(170))
	}
	return cv
}

// writeHeader записывает в контейнер заголовок версии 4 с любыми полями —
// так, как это сделал бы тот, кто знает ключ.
func writeHeader(t *testing.T, cv *Cover, master []byte, hdr Header) {
	t.Helper()
	k := keys{master: master}
	style := styleFor(hdr.Level)
	pos, ok := positions(k.perm(hdr.Mode, cv.Len()), cv, style.elems())
	if !ok {
		t.Fatal("контейнер мал для заголовка")
	}
	b := hdr.Marshal()
	maskFields(b, master, hdr.Mode)
	y, _, err := stcEmbed(lsbs(cv, pos), hillCosts(cv, pos), toBits(rs.Encode(b, style.parity)), style.h, style.w)
	if err != nil {
		t.Fatal(err)
	}
	for i, e := range pos {
		cv.Pix[e] = cv.Pix[e]&^1 | y[i]
	}
}

func TestHeaderRoundTrip(t *testing.T) {
	want := Header{Version: formatVersion, Mode: suite.MagmaMGM, Method: 1, Level: LevelHigh, H: 1, W: 4, Length: 123456}
	for i := range want.Nonce {
		want.Nonce[i] = byte(0xf0 + i)
	}
	b := want.Marshal()
	plain := append([]byte(nil), b...)
	maskFields(b, testMaster, want.Mode)
	if !bytes.Equal(b[:saltLen], plain[:saltLen]) || bytes.Equal(b[saltLen:], plain[saltLen:]) {
		t.Fatal("маска должна закрывать поля и не трогать R")
	}
	maskFields(b, testMaster, want.Mode)
	got, err := parseHeader(b)
	if err != nil || *got != want {
		t.Fatalf("%v: %+v", err, got)
	}
	// nonce шифра: старший бит обнулён, в заголовке — нет
	if n := got.aeadNonce(); len(n) != 8 || n[0] != 0x70 || got.Nonce[0] != 0xf0 {
		t.Fatalf("nonce %x", n)
	}
}

// Заголовок с верным ключом, но с длиной, которой в изображении быть не
// может: извлечение должно ответить «повреждено», не запрашивая память под
// несуществующие элементы (раньше — гигабайты, при длине 2²⁸ — падение).
func TestCraftedLengthDoesNotAllocate(t *testing.T) {
	for _, lvl := range []Level{LevelNormal, LevelHigh, LevelMax} {
		cv := noiseCover(200, 150, 3, int64(lvl))
		lp := lvl.Params()
		writeHeader(t, cv, testMaster, Header{Mode: suite.KuznyechikMGM, Level: lvl, H: lp.H, W: lp.MaxW, Length: 1 << 28})
		var a, b runtime.MemStats
		runtime.ReadMemStats(&a)
		res, err := ExtractCover(cv, "", nil, testMaster)
		runtime.ReadMemStats(&b)
		if !errors.Is(err, ErrDamaged) || res == nil || res.Level != lvl {
			t.Fatalf("%v: %v", lvl, err)
		}
		if mib := (b.TotalAlloc - a.TotalAlloc) >> 20; mib > 64 {
			t.Fatalf("%v: запрошено %d МиБ", lvl, mib)
		}
	}
}

// constantLSB — сколько элементов имеют одинаковый младший бит у всех
// изображений; equalPairs — сколько пар элементов имеют одинаковые младшие
// биты во всех изображениях (так выдаёт себя повтор бита в нескольких местах).
func constantLSB(imgs []*Cover) (constant, equalPairs int) {
	seen := map[uint64]int{}
	for e := range imgs[0].Pix {
		var v uint64
		for _, im := range imgs {
			v = v<<1 | uint64(im.Pix[e]&1)
		}
		if v == 0 || v == 1<<len(imgs)-1 {
			constant++
		}
		equalPairs += seen[v]
		seen[v]++
	}
	return
}

// Атака без ключа на заголовок: много изображений одного размера с одним
// паролем. Ни одинаковых младших битов, ни всегда равных пар быть не должно —
// как у пустых контейнеров. (Заголовок версии 3 давал 225 постоянных элементов
// на уровнях «повышенная» и «максимальная».)
func TestHeaderLeavesNoCommonBits(t *testing.T) {
	const n = 40 // случайно: 90000 / 2³⁹ постоянных, C(90000, 2) / 2⁴⁰ пар — нули
	covers := make([]*Cover, n)
	for i := range covers {
		covers[i] = noiseCover(200, 150, 3, int64(100+i))
	}
	if c, p := constantLSB(covers); c != 0 || p != 0 {
		t.Fatalf("пустые контейнеры: %d постоянных, %d пар", c, p)
	}
	for _, mode := range suite.All {
		for _, lvl := range []Level{LevelNormal, LevelHigh, LevelMax} {
			st := make([]*Cover, n)
			for i := range st {
				out, _, err := EmbedCover(covers[i], fmt.Sprintf("сообщение номер %d", i), Options{Mode: mode, Level: lvl, Master: testMaster})
				if err != nil {
					t.Fatal(err)
				}
				st[i] = out
			}
			if c, p := constantLSB(st); c != 0 || p != 0 {
				t.Errorf("%v, %v: %d элементов с постоянным младшим битом, %d всегда равных пар", mode, lvl, c, p)
			}
		}
	}
}

// Заголовок не должен ставить изменения в ровную заливку, пока есть выбор.
// Левая половина — заливка, правая — текстура.
func TestHeaderAvoidsFlatAreas(t *testing.T) {
	for _, lvl := range []Level{LevelNormal, LevelHigh} {
		cv := noiseCover(600, 400, 3, 3)
		for y := 0; y < cv.H; y++ {
			for x := 0; x < cv.W/2; x++ {
				for c := 0; c < 3; c++ {
					cv.Pix[(y*cv.W+x)*3+c] = 255
				}
			}
		}
		out, rep, err := EmbedCover(cv, "коротко", Options{Mode: suite.KuznyechikMGM, Level: lvl, Master: testMaster})
		if err != nil {
			t.Fatal(err)
		}
		flat := 0
		for e := range cv.Pix {
			if cv.Pix[e] != out.Pix[e] && (e/3)%cv.W < cv.W/2-8 {
				flat++
			}
		}
		total := rep.Changes + rep.HeaderChanges
		t.Logf("%v: изменений %d (заголовок %d), из них в заливке %d", lvl, total, rep.HeaderChanges, flat)
		// при w = 4 все четыре элемента группы лежат в заливке с вероятностью
		// 1/16, и половину таких групп приходится менять: около 3 % изменений
		if lvl == LevelNormal && flat != 0 || flat*10 > total {
			t.Errorf("%v: в заливке %d изменений из %d", lvl, flat, total)
		}
	}
}
