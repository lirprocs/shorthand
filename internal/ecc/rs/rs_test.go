package rs

import (
	"bytes"
	"math/rand"
	"testing"
)

func TestEncodeIsCodeword(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for _, r := range []int{2, 16, 32, 64, 128} {
		data := make([]byte, 255-r)
		rng.Read(data)
		cw := Encode(data, r)
		if _, bad := syndromes(cw, r); bad {
			t.Fatalf("r=%d: синдромы не нулевые", r)
		}
		if !bytes.Equal(cw[:len(data)], data) {
			t.Fatal("код не систематический")
		}
	}
}

// Известный пример: «hello world» с r = 10 (как в «Reed–Solomon codes for coders»,
// Wikiversity) — проверка порождающего многочлена и порядка коэффициентов.
func TestKnownParity(t *testing.T) {
	cw := Encode([]byte("hello world"), 10)
	want := []byte{0xed, 0x25, 0x54, 0xc4, 0xfd, 0xfd, 0x89, 0xf3, 0xa8, 0xaa}
	if !bytes.Equal(cw[11:], want) {
		t.Fatalf("проверочные байты %x, want %x", cw[11:], want)
	}
}

func corrupt(rng *rand.Rand, cw []byte, nErr, nEra int) (erasures []int) {
	perm := rng.Perm(len(cw))
	for _, p := range perm[:nErr] {
		cw[p] ^= byte(1 + rng.Intn(255))
	}
	for _, p := range perm[nErr : nErr+nEra] {
		if rng.Intn(2) == 0 { // стёртый байт может оказаться и верным
			cw[p] ^= byte(1 + rng.Intn(255))
		}
		erasures = append(erasures, p)
	}
	return
}

func TestErrorsAndErasures(t *testing.T) {
	rng := rand.New(rand.NewSource(2))
	for trial := 0; trial < 3000; trial++ {
		r := []int{4, 16, 32, 64, 128}[rng.Intn(5)]
		k := 1 + rng.Intn(255-r)
		data := make([]byte, k)
		rng.Read(data)
		cw := Encode(data, r)
		nEra := rng.Intn(r + 1)
		nErr := rng.Intn((r-nEra)/2 + 1)
		if nErr+nEra > len(cw) {
			continue
		}
		era := corrupt(rng, cw, nErr, nEra)
		if _, err := Decode(cw, r, era); err != nil {
			t.Fatalf("r=%d k=%d ошибок=%d стираний=%d: %v", r, k, nErr, nEra, err)
		}
		if !bytes.Equal(cw[:k], data) {
			t.Fatalf("r=%d k=%d ошибок=%d стираний=%d: исправлено неверно", r, k, nErr, nEra)
		}
	}
}

// За пределом 2ν + ρ ≤ r декодер должен в подавляющем большинстве случаев
// сообщить об ошибке (а не вернуть чужое слово); слово при отказе не меняется.
func TestBeyondCapacity(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	fails, miscorrect := 0, 0
	for trial := 0; trial < 500; trial++ {
		r := 32
		data := make([]byte, 100)
		rng.Read(data)
		cw := Encode(data, r)
		corrupt(rng, cw, r/2+1+rng.Intn(10), 0)
		before := append([]byte(nil), cw...)
		if _, err := Decode(cw, r, nil); err != nil {
			fails++
			if !bytes.Equal(cw, before) {
				t.Fatal("при отказе слово изменилось")
			}
		} else if !bytes.Equal(cw[:100], data) {
			miscorrect++
		}
	}
	if miscorrect > 5 {
		t.Fatalf("ложных исправлений %d из 500", miscorrect)
	}
	t.Logf("отказов %d, ложных исправлений %d из 500", fails, miscorrect)
}

func TestStream(t *testing.T) {
	rng := rand.New(rand.NewSource(4))
	for _, n := range []int{1, 50, 223, 224, 500, 3000} {
		for _, r := range []int{32, 64, 128} {
			data := make([]byte, n)
			rng.Read(data)
			coded := EncodeStream(data, r)
			if len(coded) != CodedLen(n, r) {
				t.Fatalf("CodedLen")
			}
			// испортить подряд идущий участок: перемежение раскидает его по блокам
			nb := len(layout(n, r))
			burst := nb * (r / 2) * 9 / 10
			if burst > len(coded) {
				burst = len(coded) / 3
			}
			start := rng.Intn(len(coded) - burst + 1)
			for p := start; p < start+burst; p++ {
				coded[p] ^= 0x5a
			}
			back, st, err := DecodeStream(coded, n, r, nil)
			if err != nil || !bytes.Equal(back, data) {
				t.Fatalf("n=%d r=%d пачка %d: %v", n, r, burst, err)
			}
			if st.Corrected != burst {
				t.Fatalf("исправлено %d из %d", st.Corrected, burst)
			}
		}
	}
}

// Стирания удваивают исправляющую способность: r стёртых байтов в блоке.
func TestStreamErasures(t *testing.T) {
	rng := rand.New(rand.NewSource(5))
	data := make([]byte, 400)
	rng.Read(data)
	r := 32
	coded := EncodeStream(data, r)
	erased := make([]bool, len(coded))
	cnt := len(layout(400, r)) * r // ровно r стираний на блок в среднем
	for _, p := range rng.Perm(len(coded))[:cnt*9/10] {
		coded[p] ^= 0xff
		erased[p] = true
	}
	if _, _, err := DecodeStream(coded, 400, r, nil); err == nil {
		t.Fatal("без стираний столько ошибок исправиться не должно")
	}
	back, _, err := DecodeStream(coded, 400, r, erased)
	if err != nil || !bytes.Equal(back, data) {
		t.Fatalf("со стираниями не исправилось: %v", err)
	}
}
