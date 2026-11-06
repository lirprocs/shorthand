package streebog

import (
	"bytes"
	"crypto/hmac"
	"encoding/hex"
	"strings"
	"testing"
)

// rev переводит число из записи RFC (старший байт первым) в строку байтов.
func rev(t *testing.T, s string) []byte {
	b, err := hex.DecodeString(strings.ReplaceAll(s, " ", ""))
	if err != nil {
		t.Fatal(err)
	}
	for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}
	return b
}

// Примеры из ГОСТ Р 34.11-2012, приложение А (RFC 6986, раздел 10).
var examples = []struct {
	name, m, h512, h256 string
}{
	{
		name: "M1",
		m: "32313039383736353433323130393837 36353433323130393837363534333231" +
			"30393837363534333231303938373635 343332313039383736353433323130",
		h512: "486f64c1917879417fef082b3381a4e2 11c324f074654c38823a7b76f830ad00" +
			"fa1fbae42b1285c0352f227524bc9ab1 6254288dd6863dccd5b9f54a1ad0541b",
		h256: "00557be5e584fd52a449b16b0251d05d 27f94ab76cbaa6da890b59d8ef1e159d",
	},
	{
		name: "M2",
		m: "fbe2e5f0eee3c820fbeafaebef20fffb f0e1e0f0f520e0ed20e8ece0ebe5f0f2" +
			"f120fff0eeec20f120faf2fee5e2202c e8f6f3ede220e8e6eee1e8f0f2d1202c" +
			"e8f0f2e5e220e5d1",
		h512: "28fbc9bada033b1460642bdcddb90c3f b3e56c497ccd0f62b8a2ad4935e85f03" +
			"7613966de4ee00531ae60f3b5a47f8da e06915d5f2f194996fcabf2622e6881e",
		h256: "508f7e553c06501d749a66fc28c6cac0 b005746d97537fa85d9e40904efed29d",
	},
}

func TestRFC6986(t *testing.T) {
	for _, e := range examples {
		m := rev(t, e.m)
		if e.name == "M1" && string(m) != "012345678901234567890123456789012345678901234567890123456789012" {
			t.Fatalf("неверно прочитан M1: %q", m)
		}
		for _, c := range []struct {
			h    func() []byte
			want string
		}{
			{func() []byte { d := New512(); d.Write(m); return d.Sum(nil) }, e.h512},
			{func() []byte { d := New256(); d.Write(m); return d.Sum(nil) }, e.h256},
		} {
			if got, want := c.h(), rev(t, c.want); !bytes.Equal(got, want) {
				t.Fatalf("%s: got %x, want %x", e.name, got, want)
			}
		}
	}
}

// Запись по кускам и повторный Sum дают тот же результат; проверяются
// длины на границах блока (63, 64, 65, 128 байт).
func TestChunkingAndBoundaries(t *testing.T) {
	for _, n := range []int{0, 1, 63, 64, 65, 127, 128, 129, 1000} {
		data := bytes.Repeat([]byte{0xA5}, n)
		d1 := New256()
		d1.Write(data)
		want := d1.Sum(nil)
		if again := d1.Sum(nil); !bytes.Equal(again, want) {
			t.Fatal("Sum изменил состояние")
		}
		d2 := New256()
		for i := 0; i < n; i += 7 {
			end := i + 7
			if end > n {
				end = n
			}
			d2.Write(data[i:end])
		}
		if got := d2.Sum(nil); !bytes.Equal(got, want) {
			t.Fatalf("n=%d: результат зависит от разбиения", n)
		}
	}
}

// HMAC_GOSTR3411_2012_256 — пример из Р 50.1.113-2016 (RFC 7836, приложение B).
func TestHMAC256(t *testing.T) {
	key, _ := hex.DecodeString("000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f")
	data, _ := hex.DecodeString("0126bdb87800af214341456563780100")
	m := hmac.New(New256, key)
	m.Write(data)
	got := hex.EncodeToString(m.Sum(nil))
	if got != "a1aa5f7de402d7b3d323f2991c8d4534013137010a83754fd0af6d7cd4922ed9" {
		t.Fatalf("HMAC = %s", got)
	}
}

func BenchmarkStreebog256(b *testing.B) {
	d := New256()
	buf := make([]byte, 4096)
	b.SetBytes(int64(len(buf)))
	for i := 0; i < b.N; i++ {
		d.Write(buf)
	}
}
