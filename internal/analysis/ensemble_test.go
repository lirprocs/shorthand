package analysis

import (
	"math"
	"math/rand"
	"testing"
)

// synth — пары «пустое — стего» размерности dim: стего сдвинуто на shift
// по первым k признакам; у пары общий «контент» (как у изображения до и после
// встраивания), поэтому различие видно только в паре признаков.
func synth(n, dim, k int, shift float64, seed int64) (cover, stego [][]float32) {
	rng := rand.New(rand.NewSource(seed))
	for i := 0; i < n; i++ {
		c, s := make([]float32, dim), make([]float32, dim)
		for j := range c {
			base := rng.NormFloat64()
			c[j] = float32(base + 0.3*rng.NormFloat64())
			s[j] = float32(base + 0.3*rng.NormFloat64())
			if j < k {
				s[j] += float32(shift)
			}
		}
		cover, stego = append(cover, c), append(stego, s)
	}
	return
}

func TestEnsembleSeparates(t *testing.T) {
	c, s := synth(400, 300, 60, 0.4, 1)
	e, err := TrainEnsemble(c[:200], s[:200], EnsembleConfig{DSub: 100, Seed: 2, Workers: 4})
	if err != nil {
		t.Fatal(err)
	}
	r := TestEnsemble(e, c[200:], s[200:])
	t.Logf("различимые: P_E %.3f, AUC %.3f, OOB %.3f, L %d", r.PE, r.AUC, r.OOB, r.L)
	if r.PE > 0.2 || r.AUC < 0.85 || math.Abs(r.OOB-r.PE) > 0.1 || r.L < EnsembleMinL {
		t.Fatalf("%+v", r)
	}
}

func TestEnsembleNull(t *testing.T) {
	c, s := synth(400, 300, 0, 0, 3)
	e, err := TrainEnsemble(c[:200], s[:200], EnsembleConfig{DSub: 100, Seed: 4, Workers: 4})
	if err != nil {
		t.Fatal(err)
	}
	r := TestEnsemble(e, c[200:], s[200:])
	t.Logf("неразличимые: P_E %.3f, AUC %.3f, OOB %.3f", r.PE, r.AUC, r.OOB)
	if r.PE < 0.4 || math.Abs(r.AUC-0.5) > 0.1 {
		t.Fatalf("%+v", r)
	}
}

// Вырожденные признаки (постоянные, повторяющиеся) не ломают обучение: λ растёт.
func TestEnsembleDegenerate(t *testing.T) {
	c, s := synth(200, 50, 10, 0.5, 5)
	for i := range c {
		c[i][40], s[i][40] = 0, 0
		c[i][41], s[i][41] = c[i][0], s[i][0]
	}
	e, err := TrainEnsemble(c[:100], s[:100], EnsembleConfig{DSub: 50, Seed: 6, Workers: 2})
	if err != nil {
		t.Fatal(err)
	}
	if r := TestEnsemble(e, c[100:], s[100:]); r.PE > 0.3 {
		t.Fatalf("%+v", r)
	}
}

func TestCholesky(t *testing.T) {
	s := []float64{4, 2, 2, 3}
	l, ok := cholesky(s, 2, 0)
	if !ok {
		t.Fatal("не разложилась")
	}
	x := cholSolve(l, 2, []float64{2, 1}) // 4x+2y=2, 2x+3y=1 → x=0.5, y=0
	if math.Abs(x[0]-0.5) > 1e-12 || math.Abs(x[1]) > 1e-12 {
		t.Fatalf("%v", x)
	}
	if _, ok := cholesky([]float64{1, 1, 1, 1}, 2, 0); ok {
		t.Fatal("вырожденная матрица разложилась")
	}
}
