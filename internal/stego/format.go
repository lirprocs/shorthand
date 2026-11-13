package stego

import (
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/lirprocs/shorthand/internal/compress"
	"github.com/lirprocs/shorthand/internal/crypto/kdf"
	"github.com/lirprocs/shorthand/internal/crypto/suite"
	"github.com/lirprocs/shorthand/internal/ecc/rs"
)

// Формат v3, заголовок версии 5.
//
// Порядок элементов задаёт ключевая перестановка (perm.go) с пропуском
// полностью прозрачных пикселей. Первые элементы — заголовок, следующие —
// сообщение.
//
// Заголовок (26 байт):
//
//	0–15  R — 16 случайных байт, свои у каждого сообщения
//	16–17 «SH»
//	18    версия = 5
//	19    биты 0–1 — режим шифрования, 2–3 — способ сжатия,
//	      4–5 — уровень устойчивости, 6–7 — 0
//	20    h: 1…12 — высота STC; 32 + p — код Хэмминга с p битами на группу
//	21    w — элементов на бит сообщения в STC (1…255); у кода Хэмминга — 1
//	22–25 L — длина шифротекста с имитовставкой, байт (big-endian)
//
// Байты 16–25 записываются под маской HKDF(master, соль = R). Поэтому в
// заголовке нет ни одного бита, одинакового у двух сообщений: R случаен,
// остальное зависит от R.
//
// 26 байт защищаются кодом Рида–Соломона и записываются тем же способом, что
// и сообщение своего уровня, — изменяются самые дешёвые по HILL элементы:
//
//	уровень        код заголовка  запись заголовка          код сообщения  запись сообщения
//	обычная        RS(34, 26)     STC h = 10, w = 8…64      нет            STC h = 10, w ≤ 255
//	повышенная     RS(90, 26)     код Хэмминга (15, 11)     RS 64          код Хэмминга, p ≤ 4
//	максимальная   RS(90, 26)     h = 1, w = 1              RS 128         h = 1, w = 1
//
// Ширина STC заголовка «обычной» зависит от размера изображения
// (headerWidth): чем шире, тем меньше изменений, но заголовок не должен
// занимать больше восьмой части пригодных элементов. Декодер знает число
// пригодных элементов и вычисляет ту же ширину.
//
// На «обычной» сообщение не защищено кодом Рида–Соломона: STC с h = 10
// размножает любую порчу элемента на десять бит, и код почти ничего не
// исправлял (docs/EXPERIMENTS.md, раздел 2), а стоил до половины изменений на
// коротком сообщении (раздел 7). Канал «обычной» — без потерь; целостность
// проверяет имитовставка.
//
// На «повышенной» p — наибольшее из 4, 3, 2, 1, при котором сообщение
// помещается (p = 1 — прямая запись бита в элемент).
//
// Декодер пробует все способы записи заголовка, в том числе прежние.
//
// Сообщение: шифротекст||имитовставка → RS-поток (internal/ecc/rs), если код
// есть → биты (старший бит байта первым) → STC или код Хэмминга → ±1.
//
// Шифрование: AEAD выбранного режима. Ключ — HKDF(master, соль = R); nonce —
// первые байты R (старший бит обнуляется: у MGM nonce — n − 1 бит);
// дополнительные данные = 26 байт заголовка без маски || имитозащищаемые
// данные пользователя.
//
// Заголовки версий 3 и 4 только читаются; их устройство — в compat.go.

const (
	headerLen     = 26
	formatVersion = 5
	maxW          = 255 // наибольшая ширина STC; в версиях 3 и 4 — maxWOld
	saltLen       = 16  // R
)

// headerStyle — способ записи заголовка.
type headerStyle struct {
	version int  // версия заголовка: 5; 4 и 3 — только чтение
	parity  int  // проверочных байтов RS
	repeat  int  // версия 3: повторов каждого бита прямым ±1
	h, w    int  // параметры STC
	wide    bool // ширина STC — по размеру изображения (headerWidth), w не используется
	ham     int  // код Хэмминга с ham битами на группу вместо STC
}

var (
	styleNormal = headerStyle{version: 5, parity: 8, h: 10, wide: true}
	styleHigh   = headerStyle{version: 5, parity: 64, ham: 4}
	styleMax    = headerStyle{version: 5, parity: 64, h: 1, w: 1}

	// headerStyles — порядок перебора при извлечении.
	headerStyles = []headerStyle{
		styleNormal, styleHigh, styleMax,
		styleNormalV4, styleHighV4, styleMaxV4,
		styleCompactV3, styleRobustV3,
	}
)

func (s headerStyle) coded() int { return headerLen + s.parity }
func (s headerStyle) bits() int  { return 8 * s.coded() }

// coding — способ записи заголовка в изображение с usable пригодными
// элементами (для версии 3 с повтором не применяется).
func (s headerStyle) coding(usable int) coding {
	switch {
	case s.ham > 0:
		return coding{ham: s.ham}
	case s.wide:
		return coding{h: s.h, w: headerWidth(usable, s.bits())}
	}
	return coding{h: s.h, w: s.w}
}

// elems — сколько элементов контейнера занимает заголовок.
func (s headerStyle) elems(usable int) int {
	if s.repeat > 0 {
		return s.bits() * s.repeat
	}
	return s.coding(usable).elems(s.bits())
}

// headerWidth — ширина STC заголовка «обычной»: от 8 до 64, заголовок занимает
// не больше восьмой части пригодных элементов. Часть формата.
func headerWidth(usable, bits int) int {
	return max(8, min(64, usable/(8*bits)))
}

// styleFor — способ записи заголовка для версии и уровня.
func styleFor(version int, l Level) headerStyle {
	switch version {
	case 3:
		return styleForV3(l)
	case 4:
		return styleForV4(l)
	}
	switch l {
	case LevelNormal:
		return styleNormal
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

// LevelParams — параметры уровня: проверочных байтов RS на блок 255 (0 — без
// кода) и способ записи сообщения: код Хэмминга с числом бит на группу до Ham
// либо, если Ham = 0, STC высоты H с шириной до MaxW. У «повышенной» H и MaxW —
// параметры прежнего способа (чётность группы), они нужны только стенду.
type LevelParams struct {
	RS   int
	H    int
	MaxW int
	Ham  int
}

// levelParams — значения подобраны экспериментом (cmd/stegobench,
// docs/EXPERIMENTS.md, разделы 2 и 7). h и w записываются в заголовок явно,
// поэтому их можно менять без потери совместимости; код RS задан версией
// заголовка и уровнем (msgParity).
var levelParams = [3]LevelParams{
	LevelNormal: {RS: 0, H: 10, MaxW: maxW},
	LevelHigh:   {RS: 64, H: 1, MaxW: 4, Ham: 4},
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

// msgParity — проверочных байтов RS у сообщения: в версии 5 «обычная» кода не
// имеет, в версиях 3 и 4 — 32 байта.
func msgParity(version int, l Level) int {
	if version >= 5 {
		return levelParams[l].RS
	}
	return [3]int{32, 64, 128}[l]
}

// codedLen и encodeStream — код сообщения; parity = 0 — без кода.
func codedLen(n, parity int) int {
	if parity == 0 {
		return n
	}
	return rs.CodedLen(n, parity)
}

func encodeStream(data []byte, parity int) []byte {
	if parity == 0 {
		return data
	}
	return rs.EncodeStream(data, parity)
}

// Header — поля заголовка.
type Header struct {
	Version int // 5; 4 и 3 — у изображений прежних версий
	Mode    suite.Mode
	Method  compress.Method
	Level   Level
	H, W    int // поля h и w: способ записи сообщения (codingOf)
	Length  int // байт шифротекста с имитовставкой
	// Nonce — R: соль ключей сообщения и основа nonce шифра. В версии 3 —
	// сам nonce (для «Магмы» — 8 байт и нули).
	Nonce [saltLen]byte
}

// Marshal — 26 байт заголовка версий 4 и 5 без маски (они же —
// дополнительные данные шифра). Version = 0 — нынешняя версия.
func (h *Header) Marshal() []byte {
	v := h.Version
	if v == 0 {
		v = formatVersion
	}
	b := make([]byte, headerLen)
	copy(b[:saltLen], h.Nonce[:])
	f := b[saltLen:]
	f[0], f[1] = magic[0], magic[1]
	f[2] = byte(v)
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

// maskFields накладывает (или снимает) маску на поля заголовка версий 4 и 5:
// байты 16–25 складываются по XOR с HKDF(master, соль = R).
func maskFields(b, master []byte, m suite.Mode) {
	mask := kdf.Derive(master, b[:saltLen], infoFields(m), headerLen-saltLen)
	for i, v := range mask {
		b[saltLen+i] ^= v
	}
}

var errBadHeader = errors.New("заголовок не распознан")

// parseHeader разбирает 26 байт заголовка версии 4 или 5 без маски.
func parseHeader(b []byte) (*Header, error) {
	if len(b) != headerLen {
		return nil, errBadHeader
	}
	f := b[saltLen:]
	if f[0] != magic[0] || f[1] != magic[1] || f[2] != 4 && f[2] != formatVersion {
		return nil, errBadHeader
	}
	h := &Header{Version: int(f[2])}
	copy(h.Nonce[:], b[:saltLen])
	return h, h.setFields(f[3], f[4], f[5], f[6:10])
}

// setFields заполняет и проверяет поля, общие для всех версий; Version уже
// задана.
func (h *Header) setFields(flags, hh, w byte, length []byte) error {
	if flags>>6 != 0 {
		return errBadHeader
	}
	h.Mode = suite.Mode(flags & 3)
	h.Method = compress.Method(flags >> 2 & 3)
	h.Level = Level(flags >> 4 & 3)
	h.H, h.W = int(hh), int(w)
	h.Length = int(binary.BigEndian.Uint32(length))
	wLimit := maxWOld
	if h.Version >= 5 {
		wLimit = maxW
	}
	stc := h.H >= 1 && h.H <= 12 && h.W >= 1 && h.W <= wLimit
	ham := h.Version >= 5 && h.H > hamCode && h.H <= hamCode+hamMaxP && h.W == 1
	if !h.Mode.Valid() || h.Level > LevelMax || h.Method > compress.Deflate ||
		!stc && !ham || h.Length < 1 || h.Length > 1<<28 {
		return errBadHeader
	}
	return nil
}

// Строки info для выработки ключей (часть формата; «v3» в строках — имя
// формата, оно общее для всех версий заголовка).
func infoPerm(m suite.Mode) string   { return "shorthand/v3/positions/" + m.ID() }
func infoHeader(m suite.Mode) string { return "shorthand/v3/header-mask/" + m.ID() } // только версия 3
func infoFields(m suite.Mode) string { return "shorthand/v4/header-fields/" + m.ID() }
func infoAEAD(m suite.Mode) string   { return "shorthand/v3/aead/" + m.ID() }
func infoSigns(m suite.Mode) string  { return "shorthand/v3/signs/" + m.ID() }
