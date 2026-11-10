package stego

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/lirprocs/shorthand/internal/compress"
	"github.com/lirprocs/shorthand/internal/crypto/kdf"
	"github.com/lirprocs/shorthand/internal/crypto/suite"
)

// Формат v3, заголовок версии 4.
//
// Порядок элементов задаёт ключевая перестановка (perm.go) с пропуском
// полностью прозрачных пикселей. Первые элементы — заголовок, следующие
// msgBits·w — сообщение.
//
// Заголовок (26 байт):
//
//	0–15  R — 16 случайных байт, свои у каждого сообщения
//	16–17 «SH»
//	18    версия = 4
//	19    биты 0–1 — режим шифрования, 2–3 — способ сжатия,
//	      4–5 — уровень (код избыточности RS), 6–7 — 0
//	20    h — высота STC
//	21    w — элементов контейнера на бит сообщения
//	22–25 L — длина шифротекста с имитовставкой, байт (big-endian)
//
// Байты 16–25 записываются под маской HKDF(master, соль = R). Поэтому в
// заголовке нет ни одного бита, одинакового у двух сообщений: R случаен,
// остальное зависит от R. (В версии 3 маска зависела только от пароля, и
// постоянные поля давали одинаковые младшие биты в одних и тех же элементах
// у всех изображений с одним паролем.)
//
// 26 байт защищаются кодом Рида–Соломона и записываются через STC, как и
// сообщение, — изменяются самые дешёвые по HILL элементы:
//
//	уровень        код         STC            элементов  изменений
//	обычная        RS(42, 26)  h = 10, w = 8  2688       ≈ 50–90
//	повышенная     RS(90, 26)  h = 1,  w = 4  2880       ≈ 360
//	максимальная   RS(90, 26)  h = 1,  w = 1  720        ≈ 360
//
// Декодер пробует все способы. Параметры заголовка повторяют параметры
// сообщения своего уровня (h = 1: бит зависит только от своих w элементов),
// а код у него сильнее — RS(90, 26) исправляет 35 % байтов, — поэтому
// заголовок переживает повреждения, при которых сообщение уже не
// восстановить, и декодер может сказать: сообщение есть, но испорчено.
//
// Сообщение: шифротекст||имитовставка → RS-поток (internal/ecc/rs) → биты
// (старший бит байта первым) → STC с параметрами (h, w) → ±1.
//
// Шифрование: AEAD выбранного режима. Ключ — HKDF(master, соль = R); nonce —
// первые байты R (для MGM старший бит обнуляется: nonce — n − 1 бит);
// дополнительные данные = 26 байт заголовка без маски || имитозащищаемые
// данные пользователя.
//
// Заголовок версии 3 (ветка v3 до коммита 8e03055) только читается; его
// устройство — в compat.go.

const (
	headerLen     = 26
	formatVersion = 4
	maxW          = 64
	saltLen       = 16 // R
)

// headerStyle — способ записи заголовка.
type headerStyle struct {
	version int // версия заголовка: 4 или 3 (только чтение)
	parity  int // проверочных байтов RS
	repeat  int // версия 3: повторов каждого бита прямым ±1; 0 — STC
	h, w    int // параметры STC, если repeat = 0
}

var (
	styleCompact = headerStyle{version: 4, parity: 16, h: 10, w: 8}
	styleHigh    = headerStyle{version: 4, parity: 64, h: 1, w: 4}
	styleMax     = headerStyle{version: 4, parity: 64, h: 1, w: 1}

	// headerStyles — порядок перебора при извлечении.
	headerStyles = []headerStyle{styleCompact, styleHigh, styleMax, styleCompactV3, styleRobustV3}
)

func (s headerStyle) coded() int { return headerLen + s.parity }
func (s headerStyle) bits() int  { return 8 * s.coded() }

// elems — сколько элементов контейнера занимает заголовок.
func (s headerStyle) elems() int {
	if s.repeat > 0 {
		return s.bits() * s.repeat
	}
	return s.bits() * s.w
}

// styleFor — способ записи заголовка для уровня.
func styleFor(l Level) headerStyle {
	switch l {
	case LevelNormal:
		return styleCompact
	case LevelHigh:
		return styleHigh
	}
	return styleMax
}

var magic = [2]byte{'S', 'H'}

// Level — уровень устойчивости к повреждениям.
type Level uint8

const (
	LevelNormal Level = 0 // «обычная»: наибольшая скрытность
	LevelHigh   Level = 1 // «повышенная»
	LevelMax    Level = 2 // «максимальная»
)

// LevelParams — параметры уровня: проверочных байтов RS на блок 255,
// высота STC и наибольшее w.
type LevelParams struct {
	RS   int
	H    int
	MaxW int
}

// levelParams — значения подобраны экспериментом (cmd/stegobench, раздел
// «устойчивость» в docs/DESIGN.md). Код RS в заголовке — индекс уровня;
// h и w записываются явно, поэтому их можно менять без потери совместимости.
var levelParams = [3]LevelParams{
	LevelNormal: {RS: 32, H: 10, MaxW: maxW},
	LevelHigh:   {RS: 64, H: 1, MaxW: 4},
	LevelMax:    {RS: 128, H: 1, MaxW: 1},
}

func (l Level) Params() LevelParams { return levelParams[l] }

func (l Level) String() string {
	switch l {
	case LevelNormal:
		return "обычная"
	case LevelHigh:
		return "повышенная"
	case LevelMax:
		return "максимальная"
	}
	return fmt.Sprintf("уровень %d", uint8(l))
}

// rsParity — проверочных байтов по коду уровня из заголовка.
func rsParity(code uint8) int { return [4]int{32, 64, 128, 0}[code&3] }

// Header — поля заголовка.
type Header struct {
	Version int // 4; 3 — у изображений прежней версии
	Mode    suite.Mode
	Method  compress.Method
	Level   Level
	H, W    int
	Length  int // байт шифротекста с имитовставкой
	// Nonce — R: соль ключей сообщения и основа nonce шифра. В версии 3 —
	// сам nonce (для «Магмы» — 8 байт и нули).
	Nonce [saltLen]byte
}

// Marshal — 26 байт заголовка версии 4 без маски (они же — дополнительные
// данные шифра).
func (h *Header) Marshal() []byte {
	b := make([]byte, headerLen)
	copy(b[:saltLen], h.Nonce[:])
	f := b[saltLen:]
	f[0], f[1] = magic[0], magic[1]
	f[2] = formatVersion
	f[3] = byte(h.Mode)&3 | (byte(h.Method)&3)<<2 | (byte(h.Level)&3)<<4
	f[4] = byte(h.H)
	f[5] = byte(h.W)
	binary.BigEndian.PutUint32(f[6:10], uint32(h.Length))
	return b
}

// aeadNonce — nonce шифра: первые байты R; для MGM nonce — n − 1 бит,
// поэтому старший бит обнуляется (в заголовке он остаётся случайным).
func (h *Header) aeadNonce() []byte {
	n := append([]byte(nil), h.Nonce[:h.Mode.NonceSize()]...)
	n[0] &= 0x7f
	return n
}

// maskFields накладывает (или снимает) маску на поля заголовка версии 4:
// байты 16–25 складываются по XOR с HKDF(master, соль = R).
func maskFields(b, master []byte, m suite.Mode) {
	mask := kdf.Derive(master, b[:saltLen], infoFields(m), headerLen-saltLen)
	for i, v := range mask {
		b[saltLen+i] ^= v
	}
}

var errBadHeader = errors.New("заголовок не распознан")

// parseHeader разбирает 26 байт заголовка версии 4 без маски.
func parseHeader(b []byte) (*Header, error) {
	if len(b) != headerLen {
		return nil, errBadHeader
	}
	f := b[saltLen:]
	if f[0] != magic[0] || f[1] != magic[1] || f[2] != formatVersion {
		return nil, errBadHeader
	}
	h := &Header{Version: formatVersion}
	copy(h.Nonce[:], b[:saltLen])
	return h, h.setFields(f[3], f[4], f[5], f[6:10])
}

// setFields заполняет и проверяет поля, общие для версий 3 и 4.
func (h *Header) setFields(flags, hh, w byte, length []byte) error {
	if flags>>6 != 0 {
		return errBadHeader
	}
	h.Mode = suite.Mode(flags & 3)
	h.Method = compress.Method(flags >> 2 & 3)
	h.Level = Level(flags >> 4 & 3)
	h.H, h.W = int(hh), int(w)
	h.Length = int(binary.BigEndian.Uint32(length))
	if !h.Mode.Valid() || h.Level > LevelMax || h.Method > compress.Deflate ||
		h.H < 1 || h.H > 12 || h.W < 1 || h.W > maxW || h.Length < 1 || h.Length > 1<<28 {
		return errBadHeader
	}
	return nil
}

// Строки info для выработки ключей (часть формата; «v3» в строках — имя
// формата, оно общее для заголовков версий 3 и 4).
func infoPerm(m suite.Mode) string   { return "shorthand/v3/positions/" + m.ID() }
func infoHeader(m suite.Mode) string { return "shorthand/v3/header-mask/" + m.ID() } // только версия 3
func infoFields(m suite.Mode) string { return "shorthand/v4/header-fields/" + m.ID() }
func infoAEAD(m suite.Mode) string   { return "shorthand/v3/aead/" + m.ID() }
func infoSigns(m suite.Mode) string  { return "shorthand/v3/signs/" + m.ID() }
