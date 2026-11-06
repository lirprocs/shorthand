package stego

import (
	"fmt"
	"math/rand"
	"os"
	"strings"
	"sync"
	"testing"
)

// Поиск подматриц Ĥ: для каждого w перебираются варианты генератора и
// выбирается вариант с лучшей эффективностью на случайных данных.
// Запуск: STC_SEARCH=1 go test -run TestSearchHhat -timeout 2h ./internal/stego
func TestSearchHhat(t *testing.T) {
	if os.Getenv("STC_SEARCH") == "" {
		t.Skip("поиск подматриц запускается вручную")
	}
	const h, candidates, L = 10, 16, 2048
	var sb strings.Builder
	fmt.Fprintf(&sb, "var hhatVariant%d = [65]uint8{\n\t0, // w = 0 не используется", h)
	for w := 1; w <= 64; w++ {
		effs := make([]float64, candidates)
		var wg sync.WaitGroup
		for v := 0; v < candidates; v++ {
			wg.Add(1)
			go func(v int) {
				defer wg.Done()
				cols := hhatColumnsVariant(h, w, v)
				for s := int64(1); s <= 2; s++ {
					effs[v] += searchEff(h, w, cols, L, s)
				}
			}(v)
		}
		wg.Wait()
		best, bestV := -1.0, 0
		for v, e := range effs {
			if e > best {
				best, bestV = e, v
			}
		}
		if w%8 == 1 {
			sb.WriteString("\n\t")
		}
		fmt.Fprintf(&sb, "%d, ", bestV)
		t.Logf("w=%d: вариант %d, %.3f бит на изменение", w, bestV, best/2)
	}
	sb.WriteString("\n}\n")
	os.WriteFile(os.Getenv("STC_SEARCH"), []byte(sb.String()), 0o644)
}

func searchEff(h, w int, cols []uint32, L int, seed int64) float64 {
	rng := rand.New(rand.NewSource(seed))
	n := L * w
	x := make([]uint8, n)
	rho := make([]float64, n)
	for i := range x {
		x[i] = uint8(rng.Intn(2))
		rho[i] = 1
	}
	msg := make([]uint8, L)
	for i := range msg {
		msg[i] = uint8(rng.Intn(2))
	}
	y, _, _ := viterbi(x, rho, msg, h, w, cols)
	ch := 0
	for i := range x {
		if x[i] != y[i] {
			ch++
		}
	}
	return float64(L) / float64(ch)
}
