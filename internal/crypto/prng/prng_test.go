package prng_test

import (
	"bytes"
	"encoding/hex"
	"os"
	"testing"

	"github.com/lirprocs/shorthand/internal/crypto/prng"
	"github.com/lirprocs/shorthand/internal/crypto/suite"
)

func key32() []byte {
	k := make([]byte, 32)
	for i := range k {
		k[i] = byte(i)
	}
	return k
}

// Поток генератора — часть формата: от него зависят позиции встраивания и
// знаки. Первые 32 байта потока каждого режима от ключа 00 01 … 1f закреплены;
// если значение изменилось, прежние изображения перестанут читаться.
//
// Показать текущие значения: SHORTHAND_PRINT_KAT=1 go test -run TestStreamFrozen -v ./internal/crypto/prng
var frozen = map[suite.Mode]string{
	suite.KuznyechikMGM: "e32e9891f76591aaeb61c8b05ac747b22a7cff131273114992fa1c79bf14d8b9",
	suite.AsconAEAD128:  "2e5f3403f4171471cc7934b51982cece8d6628435db70e89880f3be4e0b7b052",
	suite.MagmaMGM:      "405d88fc8e55a8451f5774ad6c8b2750b9391d3601482d808c71cf2371f31d40",
}

func TestStreamFrozen(t *testing.T) {
	for _, m := range suite.All {
		got := make([]byte, 32)
		m.NewRand(key32()).Read(got)
		if os.Getenv("SHORTHAND_PRINT_KAT") != "" {
			t.Logf("%v: %x", m, got)
			continue
		}
		if hex.EncodeToString(got) != frozen[m] {
			t.Fatalf("%v: поток генератора изменился:\n получено %x\n ожидалось %s", m, got, frozen[m])
		}
	}
}

// Read по частям, Uint64 и Bit читают один и тот же поток подряд, в том числе
// через границу внутреннего буфера (4096 байт).
func TestReadersShareStream(t *testing.T) {
	for _, m := range suite.All {
		whole := make([]byte, 3*4096+24)
		m.NewRand(key32()).Read(whole)

		r := m.NewRand(key32())
		var parts []byte
		for _, n := range []int{1, 7, 4090, 5, 4096, 4000, 109} {
			p := make([]byte, n)
			r.Read(p)
			parts = append(parts, p...)
		}
		if !bytes.Equal(parts, whole[:len(parts)]) {
			t.Fatalf("%v: чтение по частям расходится с чтением целиком", m)
		}

		r = m.NewRand(key32())
		skip := make([]byte, 4092)
		r.Read(skip)
		// осталось 4 байта буфера: Uint64 должен взять их и 4 байта следующего блока
		var want uint64
		for i := 7; i >= 0; i-- {
			want = want<<8 | uint64(whole[4092+i])
		}
		if got := r.Uint64(); got != want {
			t.Fatalf("%v: Uint64 на границе буфера: %x, ожидалось %x", m, got, want)
		}
		if got, want := r.Bit(), whole[4100]&1 == 1; got != want {
			t.Fatalf("%v: Bit после Uint64: %v, ожидалось %v", m, got, want)
		}
	}
}

// Uint64n: граница отбраковки. При n чуть больше 2^63 примерно половина
// чисел потока отбрасывается — результат всё равно должен лежать в [0, n).
func TestUint64nRange(t *testing.T) {
	r := prng.NewXOF(key32())
	for _, n := range []uint64{1, 2, 3, 1000003, 1<<63 + 1, ^uint64(0)} {
		for i := 0; i < 2000; i++ {
			if v := r.Uint64n(n); v >= n {
				t.Fatalf("Uint64n(%d) = %d", n, v)
			}
		}
	}
	// равномерность на n, не являющемся степенью двойки: 3 корзины
	var cnt [3]int
	for i := 0; i < 90000; i++ {
		cnt[r.Uint64n(3)]++
	}
	for _, c := range cnt {
		if c < 29400 || c > 30600 { // ±4,2σ
			t.Fatalf("Uint64n(3): корзины %v", cnt)
		}
	}
}
