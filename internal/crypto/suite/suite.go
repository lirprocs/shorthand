// Package suite связывает три режима шифрования программы: каждый режим
// целиком построен на одном примитиве — им же шифруется сообщение и им же
// вырабатываются позиции встраивания.
//
//	Режим           Шифрование                      Генератор позиций
//	Кузнечик-MGM    «Кузнечик» + MGM (ГОСТ)          «Кузнечик» в режиме CTR
//	Ascon-AEAD128   Ascon-AEAD128 (NIST SP 800-232)  Ascon-XOF128
//	Магма-MGM       «Магма» + MGM (ГОСТ)             «Магма» в режиме CTR
//
// MGM — библиотека github.com/lirprocs/MGM.
package suite

import (
	"crypto/cipher"
	"fmt"

	mgm "github.com/lirprocs/MGM"

	"github.com/lirprocs/shorthand/internal/crypto/ascon"
	"github.com/lirprocs/shorthand/internal/crypto/kuznyechik"
	"github.com/lirprocs/shorthand/internal/crypto/magma"
	"github.com/lirprocs/shorthand/internal/crypto/prng"
)

// Mode — режим шифрования; значение записывается в заголовок (2 бита).
type Mode uint8

const (
	KuznyechikMGM Mode = 0
	AsconAEAD128  Mode = 1
	MagmaMGM      Mode = 2
)

// All — все режимы в порядке перебора при извлечении.
var All = []Mode{KuznyechikMGM, AsconAEAD128, MagmaMGM}

func (m Mode) String() string {
	switch m {
	case KuznyechikMGM:
		return "Кузнечик-MGM"
	case AsconAEAD128:
		return "Ascon-AEAD128"
	case MagmaMGM:
		return "Магма-MGM"
	}
	return fmt.Sprintf("режим %d", uint8(m))
}

// ID — короткое имя для строк info в KDF (часть формата).
func (m Mode) ID() string {
	switch m {
	case KuznyechikMGM:
		return "kuznyechik-mgm"
	case AsconAEAD128:
		return "ascon-aead128"
	case MagmaMGM:
		return "magma-mgm"
	}
	return "unknown"
}

func (m Mode) Valid() bool { return m <= MagmaMGM }

// KeySize — длина ключа шифрования в байтах.
func (m Mode) KeySize() int {
	if m == AsconAEAD128 {
		return ascon.KeySize
	}
	return 32
}

// NonceSize — длина nonce в байтах (для MGM — длина блока; старший бит не используется).
func (m Mode) NonceSize() int {
	switch m {
	case AsconAEAD128:
		return ascon.NonceSize
	case MagmaMGM:
		return magma.BlockSize
	}
	return kuznyechik.BlockSize
}

// NewAEAD возвращает шифр с имитовставкой.
func (m Mode) NewAEAD(key []byte) (cipher.AEAD, error) {
	switch m {
	case KuznyechikMGM:
		b, err := kuznyechik.New(key)
		if err != nil {
			return nil, err
		}
		return mgm.New(b)
	case MagmaMGM:
		b, err := magma.New(key)
		if err != nil {
			return nil, err
		}
		return mgm.New(b)
	case AsconAEAD128:
		return ascon.NewAEAD128(key)
	}
	return nil, fmt.Errorf("suite: неизвестный режим %d", m)
}

// NewRand возвращает генератор режима от 32-байтного ключа.
func (m Mode) NewRand(key []byte) *prng.Rand {
	switch m {
	case KuznyechikMGM:
		b, err := kuznyechik.New(key)
		if err != nil {
			panic(err)
		}
		return prng.NewCTR(b)
	case MagmaMGM:
		b, err := magma.New(key)
		if err != nil {
			panic(err)
		}
		return prng.NewCTR(b)
	default:
		return prng.NewXOF(key)
	}
}
