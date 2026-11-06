package ascon

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"math/rand"
	"os"
	"strings"
	"testing"
)

// KAT-векторы взяты из эталонной реализации ascon/ascon-c (ветка main,
// коммит 446347f, январь 2026; соответствует NIST SP 800-232):
//   crypto_aead/asconaead128/LWC_AEAD_KAT_128_128.txt — все 1089 векторов;
//   crypto_hash/asconxof128/LWC_XOF_KAT_128_512.txt  — первые 200 векторов.

func readKAT(t *testing.T, path string) []map[string]string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []map[string]string
	cur := map[string]string{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			if len(cur) > 0 {
				out = append(out, cur)
				cur = map[string]string{}
			}
			continue
		}
		k, v, ok := strings.Cut(line, " = ")
		if !ok {
			k, v = strings.TrimSuffix(line, " ="), ""
		}
		cur[k] = v
	}
	if len(cur) > 0 {
		out = append(out, cur)
	}
	return out
}

func h(t *testing.T, s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestAEAD128KAT(t *testing.T) {
	vs := readKAT(t, "testdata/LWC_AEAD_KAT_128_128.txt")
	if len(vs) != 1089 {
		t.Fatalf("прочитано %d векторов", len(vs))
	}
	for _, v := range vs {
		a, err := NewAEAD128(h(t, v["Key"]))
		if err != nil {
			t.Fatal(err)
		}
		nonce, pt, ad, want := h(t, v["Nonce"]), h(t, v["PT"]), h(t, v["AD"]), h(t, v["CT"])
		got := a.Seal(nil, nonce, pt, ad)
		if !bytes.Equal(got, want) {
			t.Fatalf("Count %s: Seal\n got %X\nwant %X", v["Count"], got, want)
		}
		back, err := a.Open(nil, nonce, want, ad)
		if err != nil || !bytes.Equal(back, pt) {
			t.Fatalf("Count %s: Open err=%v", v["Count"], err)
		}
		if len(want) > 0 {
			bad := append([]byte(nil), want...)
			bad[len(bad)-1] ^= 0x80
			if _, err := a.Open(nil, nonce, bad, ad); err == nil {
				t.Fatalf("Count %s: искажение не обнаружено", v["Count"])
			}
		}
	}
}

func TestXOF128KAT(t *testing.T) {
	vs := readKAT(t, "testdata/LWC_XOF_KAT_128_512_first200.txt")
	for _, v := range vs {
		msg, want := h(t, v["Msg"]), h(t, v["MD"])
		x := NewXOF128()
		x.Write(msg)
		got := make([]byte, len(want))
		x.Read(got)
		if !bytes.Equal(got, want) {
			t.Fatalf("Count %s:\n got %X\nwant %X", v["Count"], got, want)
		}
	}
}

// Запись и чтение кусками любой длины дают тот же поток, что и целиком.
func TestXOFChunking(t *testing.T) {
	rng := rand.New(rand.NewSource(5))
	msg := make([]byte, 300)
	rng.Read(msg)
	ref := NewXOF128()
	ref.Write(msg)
	want := make([]byte, 1000)
	ref.Read(want)
	for trial := 0; trial < 50; trial++ {
		x := NewXOF128()
		for p := msg; len(p) > 0; {
			k := rng.Intn(20)
			if k > len(p) {
				k = len(p)
			}
			x.Write(p[:k])
			p = p[k:]
		}
		got := make([]byte, 0, 1000)
		for len(got) < 1000 {
			k := rng.Intn(30) + 1
			if len(got)+k > 1000 {
				k = 1000 - len(got)
			}
			buf := make([]byte, k)
			x.Read(buf)
			got = append(got, buf...)
		}
		if !bytes.Equal(got, want) {
			t.Fatal("поток зависит от разбиения на куски")
		}
	}
}

func BenchmarkAEAD128(b *testing.B) {
	a, _ := NewAEAD128(make([]byte, 16))
	nonce := make([]byte, 16)
	buf := make([]byte, 4096)
	b.SetBytes(int64(len(buf)))
	for i := 0; i < b.N; i++ {
		a.Seal(buf[:0], nonce, buf, nil)
	}
}

func BenchmarkXOF128(b *testing.B) {
	x := NewXOF128()
	x.Write([]byte("ключ"))
	buf := make([]byte, 4096)
	b.SetBytes(int64(len(buf)))
	for i := 0; i < b.N; i++ {
		x.Read(buf)
	}
}
