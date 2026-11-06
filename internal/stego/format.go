package stego

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/lirprocs/shorthand/internal/compress"
	"github.com/lirprocs/shorthand/internal/crypto/suite"
)

// Формат v3.
//
// Порядок элементов задаёт ключевая перестановка (perm.go) с пропуском
// полностью прозрачных пикселей. Первые элементы — заголовок, следующие
// msgBits·w — сообщение.
//
// Заголовок (26 байт):
//
//	0–1   «SH»
//	2     версия = 3
//	3     биты 0–1 — режим шифрования, 2–3 — способ сжатия,
//	      4–5 — код избыточности RS, 6–7 — 0
//	4     h — высота STC
//	5     w — элементов контейнера на бит сообщения
//	6–9   L — длина шифротекста с имитовставкой, байт (big-endian)
//	10–25 nonce (для «Магмы» — 8 байт и нули)
//
// Заголовок записывается одним из двух способов (декодер пробует оба):
//
//   - компактный (уровень «обычная»): RS(42, 26), маска, STC с h = 10, w = 8 —
//     336 бит в 2688 элементах, около 50 изменений, как и у сообщения,
//     в текстурных местах;
//   - устойчивый («повышенная», «максимальная»): RS(90, 26), маска, каждый из
//     720 бит — прямым ±1 в трёх элементах, при чтении — большинство из трёх.
//     Около 1100 изменений, зато заголовок переживает повреждения, при
//     которых сообщение уже не восстановить, и декодер может сказать, что
//     сообщение есть, но испорчено.
//
// Маска — поток генератора режима: без пароля заголовок неотличим от шума.
//
// Сообщение: шифротекст||имитовставка → RS-поток (internal/ecc/rs) → биты
// (старший бит байта первым) → STC с параметрами (h, w) → ±1.
//
// Шифрование: AEAD выбранного режима, nonce случайный, дополнительные
// данные = 26 байт заголовка || имитозащищаемые данные пользователя.

const (
	headerLen     = 26
	formatVersion = 3
	maxW          = 64
)

// headerStyle — способ записи заголовка.
type headerStyle struct {
	parity int // проверочных байтов RS
	repeat int // повторов каждого бита (прямое ±1); 0 — STC
	h, w   int // параметры STC, если repeat = 0
}

var (
	styleCompact = headerStyle{parity: 16, h: 10, w: 8}
	styleRobust  = headerStyle{parity: 64, repeat: 3}
	headerStyles = []headerStyle{styleCompact, styleRobust}
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

func styleFor(l Level) headerStyle {
	if l == LevelNormal {
		return styleCompact
	}
	return styleRobust
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
	LevelMax:    {RS: 128, H: 1, MaxW: 2},
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
	Mode   suite.Mode
	Method compress.Method
	Level  Level
	H, W   int
	Length int // байт шифротекста с имитовставкой
	Nonce  [16]byte
}

func (h *Header) Marshal() []byte {
	b := make([]byte, headerLen)
	b[0], b[1] = magic[0], magic[1]
	b[2] = formatVersion
	b[3] = byte(h.Mode)&3 | (byte(h.Method)&3)<<2 | (byte(h.Level)&3)<<4
	b[4] = byte(h.H)
	b[5] = byte(h.W)
	binary.BigEndian.PutUint32(b[6:10], uint32(h.Length))
	copy(b[10:26], h.Nonce[:])
	return b
}

var errBadHeader = errors.New("заголовок не распознан")

func parseHeader(b []byte) (*Header, error) {
	if len(b) != headerLen || b[0] != magic[0] || b[1] != magic[1] || b[2] != formatVersion || b[3]>>6 != 0 {
		return nil, errBadHeader
	}
	h := &Header{
		Mode:   suite.Mode(b[3] & 3),
		Method: compress.Method(b[3] >> 2 & 3),
		Level:  Level(b[3] >> 4 & 3),
		H:      int(b[4]),
		W:      int(b[5]),
		Length: int(binary.BigEndian.Uint32(b[6:10])),
	}
	copy(h.Nonce[:], b[10:26])
	if !h.Mode.Valid() || h.Level > LevelMax || h.Method > compress.Deflate ||
		h.H < 1 || h.H > 12 || h.W < 1 || h.W > maxW || h.Length < 1 || h.Length > 1<<28 {
		return nil, errBadHeader
	}
	return h, nil
}

// Строки info для выработки ключей (часть формата).
func infoPerm(m suite.Mode) string   { return "shorthand/v3/positions/" + m.ID() }
func infoHeader(m suite.Mode) string { return "shorthand/v3/header-mask/" + m.ID() }
func infoAEAD(m suite.Mode) string   { return "shorthand/v3/aead/" + m.ID() }
func infoSigns(m suite.Mode) string  { return "shorthand/v3/signs/" + m.ID() }
