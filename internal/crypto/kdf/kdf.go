// Package kdf вырабатывает ключи из пароля.
//
//	master = Argon2id(пароль, соль приложения, t = 3, m = 64 МиБ, p = 4) — 32 байта
//	ключ   = HKDF(HMAC-Стрибог-256, master, соль, info, длина)
//
// Argon2id (RFC 9106) делает перебор паролей дорогим: на каждую попытку нужно
// 64 МиБ памяти. HKDF (RFC 5869) с HMAC на «Стрибоге» (Р 50.1.113-2016)
// разводит из одного master независимые ключи для разных целей: позиции,
// маска заголовка, шифрование — по строке info.
//
// Соль постоянная: декодер должен найти данные в изображении до того, как
// прочитает что-либо из него, поэтому соль нельзя хранить внутри. Следствие —
// можно заранее посчитать таблицу для частых паролей сразу для всех
// пользователей программы; при 64 МиБ на попытку это дорого, но пароль всё
// равно должен быть длинным.
package kdf

import (
	"crypto/hkdf"
	"hash"

	"golang.org/x/crypto/argon2"

	"github.com/lirprocs/shorthand/internal/crypto/streebog"
)

// Параметры Argon2id — часть формата v3: при изменении старые изображения
// перестанут читаться.
const (
	ArgonTime    = 3
	ArgonMemory  = 64 * 1024 // КиБ
	ArgonThreads = 4
	MasterSize   = 32
)

// AppSalt — соль приложения (часть формата v3).
const AppSalt = "Shorthand v3 / стеганография текста / ИУ10"

// Master вырабатывает главный ключ из пароля.
func Master(password string) []byte {
	return argon2.IDKey([]byte(password), []byte(AppSalt), ArgonTime, ArgonMemory, ArgonThreads, MasterSize)
}

// Derive вырабатывает ключ длиной n байт для цели info.
func Derive(master, salt []byte, info string, n int) []byte {
	return deriveWith(streebog.New256, master, salt, info, n)
}

func deriveWith(h func() hash.Hash, secret, salt []byte, info string, n int) []byte {
	k, err := hkdf.Key(h, secret, salt, info, n)
	if err != nil {
		panic("kdf: " + err.Error()) // только при n > 255·32 — ошибка программы
	}
	return k
}
