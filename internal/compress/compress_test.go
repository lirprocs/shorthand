package compress

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"math/rand"
	"strings"
	"testing"

	"github.com/lirprocs/shorthand/internal/alphabet"
)

// Тестовые тексты не входят в корпус.
var samples = []string{
	"Привет, мой друг!",
	"Встреча завтра в 18:00 у метро Бауманская, принеси документы.",
	"Код подтверждения: 482913. Никому его не сообщайте.",
	"Сегодня я долго думал о том, что ты сказал вчера. Наверное, ты прав: нам стоит поговорить спокойно, без спешки, " +
		"и решить, как быть дальше. Позвони мне, когда освободишься, я весь вечер буду дома.",
	strings.Repeat("Отчёт по проекту: выполнено три этапа из пяти, сроки соблюдаются, бюджет не превышен. ", 4) +
		"Следующая проверка — в конце месяца. Ответственный — Петров А. В.",
	"The package was delivered on time. Спасибо, всё получили! 😀",
	"",
}

func TestRoundTripAllMethods(t *testing.T) {
	for _, s := range samples {
		sym := alphabet.Encode(s)
		for _, m := range []Method{Packed7, Context, Deflate} {
			data := CompressWith(m, sym)
			back, err := Decompress(m, data)
			if err != nil || !bytes.Equal(back, sym) {
				t.Fatalf("%v, %q: не восстановилось (err %v)", m, s, err)
			}
		}
		m, data := Compress(sym)
		back, err := Decompress(m, data)
		if err != nil || !bytes.Equal(back, sym) {
			t.Fatalf("Compress/%v: не восстановилось", m)
		}
	}
}

func TestRandomSymbols(t *testing.T) {
	rng := rand.New(rand.NewSource(9))
	for i := 0; i < 200; i++ {
		n := rng.Intn(400)
		sym := make([]uint8, 0, n+4)
		for len(sym) < n {
			if rng.Intn(20) == 0 {
				// ESC и три «сырые» части кода Unicode — среди них бывают 0 и резервные коды
				sym = append(sym, alphabet.ESC, uint8(rng.Intn(128)), uint8(rng.Intn(128)), uint8(rng.Intn(128)))
				continue
			}
			sym = append(sym, uint8(2+rng.Intn(117)))
		}
		for _, m := range []Method{Packed7, Context, Deflate} {
			back, err := Decompress(m, CompressWith(m, sym))
			if err != nil || !bytes.Equal(back, sym) {
				t.Fatalf("%v: случайная последовательность длины %d не восстановилась", m, n)
			}
		}
	}
}

func TestCorruptDoesNotHang(t *testing.T) {
	rng := rand.New(rand.NewSource(10))
	for i := 0; i < 300; i++ {
		data := make([]byte, rng.Intn(60))
		rng.Read(data)
		for _, m := range []Method{Packed7, Context, Deflate} {
			Decompress(m, data) // ошибка или мусор — но не зависание и не паника
		}
	}
}

// Корпус — часть формата: его изменение ломает чтение старых сообщений.
func TestCorpusFrozen(t *testing.T) {
	sum := sha256.Sum256([]byte(strings.ReplaceAll(corpus, "\r\n", "\n")))
	const want = "c72415695982fac421d3a01b10aeb4a99429afacb23496b3246154d77eb2388c"
	if got := hex.EncodeToString(sum[:]); got != want {
		t.Fatalf("корпус изменён: sha256 = %s", got)
	}
}

func TestReport(t *testing.T) {
	for _, s := range samples {
		if s == "" {
			continue
		}
		sym := alphabet.Encode(s)
		utf := len(s)
		p := len(CompressWith(Packed7, sym))
		c := len(CompressWith(Context, sym))
		d := len(CompressWith(Deflate, sym))
		m, best := Compress(sym)
		t.Logf("%4d симв. UTF-8 %4d | 7 бит %4d | контекст %4d (%.2f бит/симв) | deflate %4d → %v, %d байт",
			len([]rune(s)), utf, p, c, float64(8*c)/float64(len([]rune(s))), d, m, len(best))
	}
}
