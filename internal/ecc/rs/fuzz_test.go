package rs

// Этап 1 «на потом» (ветка later/stage1): fuzz-тест декодера.
//
//	go test -run=^$ -fuzz=FuzzDecode -fuzztime=30s ./internal/ecc/rs

import (
	"bytes"
	"testing"
)

// Декодер на любом входе не паникует; если сообщает об успехе — слово
// действительно кодовое; если ошибок в пределах возможностей — исправляет.
func FuzzDecode(f *testing.F) {
	f.Add([]byte("hello world"), []byte{1, 2, 3}, uint8(16))
	f.Add(make([]byte, 40), []byte{}, uint8(8))
	f.Fuzz(func(t *testing.T, data, noise []byte, r uint8) {
		nsym := int(r%64) + 2
		if len(data) == 0 || len(data)+nsym > 255 {
			return
		}
		cw := Encode(data, nsym)
		bad := append([]byte(nil), cw...)
		for i, b := range noise {
			bad[(i*31)%len(bad)] ^= b
		}
		work := append([]byte(nil), bad...)
		if _, err := Decode(work, nsym, nil); err == nil {
			if _, nz := syndromes(work, nsym); nz {
				t.Fatal("успех, но слово не кодовое")
			}
		}
		// ошибок не больше nsym/2 — обязаны исправить
		errs := 0
		for i := range cw {
			if cw[i] != bad[i] {
				errs++
			}
		}
		if 2*errs <= nsym {
			work = append([]byte(nil), bad...)
			if _, err := Decode(work, nsym, nil); err != nil || !bytes.Equal(work, cw) {
				t.Fatalf("%d ошибок при r = %d не исправлено", errs, nsym)
			}
		}
	})
}
