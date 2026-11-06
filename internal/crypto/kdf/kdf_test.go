package kdf

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

// Обвязка HKDF проверяется примером 1 из RFC 5869 (SHA-256): та же функция
// deriveWith используется со «Стрибогом».
func TestHKDFRFC5869Case1(t *testing.T) {
	ikm, _ := hex.DecodeString("0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b0b")
	salt, _ := hex.DecodeString("000102030405060708090a0b0c")
	info, _ := hex.DecodeString("f0f1f2f3f4f5f6f7f8f9")
	okm := deriveWith(sha256.New, ikm, salt, string(info), 42)
	want := "3cb25f25faacd57a90434f64d0362f2a2d2d0a90cf1a5a4c5db02d56ecc4c5bf34007208d5b887185865"
	if hex.EncodeToString(okm) != want {
		t.Fatalf("HKDF = %x", okm)
	}
}

func TestDeriveSeparation(t *testing.T) {
	m := bytes.Repeat([]byte{7}, 32)
	a := Derive(m, nil, "a", 32)
	b := Derive(m, nil, "b", 32)
	c := Derive(m, []byte{1}, "a", 32)
	if bytes.Equal(a, b) || bytes.Equal(a, c) {
		t.Fatal("разные info/соль дали одинаковые ключи")
	}
	if !bytes.Equal(a, Derive(m, nil, "a", 32)) {
		t.Fatal("Derive недетерминирован")
	}
}

func TestMaster(t *testing.T) {
	if testing.Short() {
		t.Skip("Argon2 с 64 МиБ")
	}
	a, b := Master("пароль"), Master("пароль")
	if !bytes.Equal(a, b) || len(a) != MasterSize {
		t.Fatal("Master недетерминирован")
	}
	if bytes.Equal(a, Master("пароль1")) {
		t.Fatal("разные пароли — одинаковый ключ")
	}
}
