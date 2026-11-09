package stego

import (
	"crypto/rand"
	"errors"
	"fmt"
	"image"

	"github.com/lirprocs/shorthand/internal/alphabet"
	"github.com/lirprocs/shorthand/internal/compress"
	"github.com/lirprocs/shorthand/internal/crypto/kdf"
	"github.com/lirprocs/shorthand/internal/crypto/prng"
	"github.com/lirprocs/shorthand/internal/crypto/suite"
	"github.com/lirprocs/shorthand/internal/ecc/rs"
)

// Options — параметры встраивания.
type Options struct {
	Mode     suite.Mode
	Level    Level
	Password string
	AAD      []byte // имитозащищаемые данные (не встраиваются, нужны при извлечении)

	// Для экспериментов (cmd/stegobench), в программе не используются:
	// фиксированный nonce, заранее выработанный master (чтобы не считать
	// Argon2 в каждом опыте), готовая полезная нагрузка вместо текста и
	// другие параметры STC вместо параметров уровня.
	Nonce  []byte
	Master []byte
	Raw    []byte
	H      int
	MaxW   int
}

// Report — что и как встроено.
type Report struct {
	Mode          suite.Mode
	Level         Level
	Method        compress.Method
	Chars         int // символов текста
	Compressed    int // байт после сжатия
	Ciphertext    int // байт шифротекста с имитовставкой
	Coded         int // байт после кода Рида–Соломона
	MsgBits       int // бит сообщения для STC
	H, W          int
	Elements      int // элементов контейнера, пригодных для встраивания
	Used          int // из них занято (заголовок + сообщение)
	Changes       int // изменено элементов сообщением
	HeaderChanges int // изменено элементов заголовком
}

var (
	ErrCapacity = errors.New("сообщение не помещается в изображение")
	ErrNotFound = errors.New("сообщение не найдено: неверный пароль, в изображении нет сообщения или оно сильно повреждено")
	ErrDamaged  = errors.New("сообщение найдено, но повреждено сильнее, чем можно исправить")
	ErrAuth     = errors.New("имитовставка не сошлась: неверные имитозащищаемые данные или сообщение изменено")
)

// CapacityError — подробности, когда сообщение не помещается.
type CapacityError struct {
	Need, Have int // элементов
}

func (e *CapacityError) Error() string {
	return fmt.Sprintf("%v: нужно %d элементов, в изображении %d (возьмите изображение крупнее, сократите текст или снизьте уровень устойчивости)",
		ErrCapacity, e.Need, e.Have)
}

func (e *CapacityError) Unwrap() error { return ErrCapacity }

func usableCount(c *Cover) int {
	if c.Alpha == nil {
		return c.Len()
	}
	n := 0
	for _, a := range c.Alpha {
		if a != 0 {
			n += c.C
		}
	}
	return n
}

// Plan считает размеры без встраивания (быстро, без Argon2).
func Plan(c *Cover, text string, opt Options) (*Report, []byte, error) {
	if !opt.Mode.Valid() || opt.Level > LevelMax {
		return nil, nil, errors.New("неверный режим или уровень")
	}
	var method compress.Method
	var comp []byte
	if opt.Raw != nil {
		comp = opt.Raw
	} else {
		method, comp = compress.Compress(alphabet.Encode(text))
	}
	aead, _ := opt.Mode.NewAEAD(make([]byte, opt.Mode.KeySize()))
	lp := opt.Level.Params()
	if opt.H > 0 {
		lp.H = opt.H
	}
	if opt.MaxW > 0 {
		lp.MaxW = min(opt.MaxW, maxW)
	}
	r := &Report{
		Mode: opt.Mode, Level: opt.Level, Method: method,
		Chars:      len([]rune(text)),
		Compressed: len(comp),
		Ciphertext: len(comp) + aead.Overhead(),
		H:          lp.H,
		Elements:   usableCount(c),
	}
	r.Coded = rs.CodedLen(r.Ciphertext, lp.RS)
	r.MsgBits = 8 * r.Coded
	hElems := styleFor(opt.Level).elems()
	avail := r.Elements - hElems
	r.W = min(lp.MaxW, max(avail, 0)/r.MsgBits)
	if r.W < 1 {
		return r, nil, &CapacityError{Need: hElems + r.MsgBits, Have: r.Elements}
	}
	r.Used = hElems + r.MsgBits*r.W
	return r, comp, nil
}

// positions — первые count пригодных элементов в порядке перестановки.
// Если столько элементов в изображении нет, память под них не запрашивается.
func positions(p *Perm, c *Cover, count int) ([]int, bool) {
	n := c.Len()
	if count < 0 || count > n {
		return nil, false
	}
	out := make([]int, 0, count)
	for j := 0; j < n && len(out) < count; j++ {
		e := p.At(j)
		if c.Usable(e) {
			out = append(out, e)
		}
	}
	return out, len(out) == count
}

func toBits(b []byte) []uint8 {
	out := make([]uint8, 8*len(b))
	for i, v := range b {
		for k := 0; k < 8; k++ {
			out[8*i+k] = v >> (7 - k) & 1
		}
	}
	return out
}

func fromBits(bits []uint8) []byte {
	out := make([]byte, len(bits)/8)
	for i := range out {
		var v byte
		for k := 0; k < 8; k++ {
			v = v<<1 | bits[8*i+k]&1
		}
		out[i] = v
	}
	return out
}

type keys struct {
	master []byte
}

func (k keys) perm(m suite.Mode, n int) *Perm {
	return NewPerm(m.NewRand(kdf.Derive(k.master, nil, infoPerm(m), 32)), n)
}

// change меняет элемент на ±1 так, чтобы младший бит стал bit.
func change(c *Cover, e int, bit uint8, signs *prng.Rand) bool {
	v := c.Pix[e]
	if v&1 == bit {
		return false
	}
	switch {
	case v == 0:
		v = 1
	case v == 255:
		v = 254
	case signs.Bit():
		v++
	default:
		v--
	}
	c.Pix[e] = v
	return true
}

// Embed встраивает текст и возвращает новое изображение.
func Embed(img image.Image, text string, opt Options) (image.Image, *Report, error) {
	out, rep, err := EmbedCover(FromImage(img), text, opt)
	if err != nil {
		return nil, rep, err
	}
	return out.Image(), rep, nil
}

// EmbedCover — встраивание в контейнер (исходный не меняется).
func EmbedCover(cv *Cover, text string, opt Options) (*Cover, *Report, error) {
	rep, comp, err := Plan(cv, text, opt)
	if err != nil {
		return nil, rep, err
	}
	mode, lp := opt.Mode, opt.Level.Params()
	k := keys{master: opt.Master}
	if k.master == nil {
		k.master = kdf.Master(opt.Password)
	}

	// заголовок и шифрование. R — 16 случайных байт сообщения: соль ключей и
	// основа nonce шифра; случайны все 16 при любом режиме, чтобы в
	// заголовке не было постоянных битов
	var nonce [saltLen]byte
	if opt.Nonce != nil {
		copy(nonce[:], opt.Nonce)
	} else if _, err := rand.Read(nonce[:]); err != nil {
		return nil, rep, err
	}
	hdr := Header{Version: formatVersion, Mode: mode, Method: rep.Method, Level: opt.Level, H: rep.H, W: rep.W, Length: rep.Ciphertext, Nonce: nonce}
	hb := hdr.Marshal()
	aead, err := mode.NewAEAD(kdf.Derive(k.master, nonce[:], infoAEAD(mode), mode.KeySize()))
	if err != nil {
		return nil, rep, err
	}
	ct := aead.Seal(nil, hdr.aeadNonce(), comp, append(append([]byte(nil), hb...), opt.AAD...))
	coded := rs.EncodeStream(ct, lp.RS)
	msg := toBits(coded)

	// позиции
	perm := k.perm(mode, cv.Len())
	pos, ok := positions(perm, cv, rep.Used)
	if !ok {
		return nil, rep, &CapacityError{Need: rep.Used, Have: rep.Elements}
	}
	out := cv.Clone()
	signs := mode.NewRand(kdf.Derive(k.master, nonce[:], infoSigns(mode), 32))

	// стоимости HILL исходного изображения для всех занятых элементов
	rho := hillCosts(cv, pos)
	apply := func(p []int, y []uint8) (n int) {
		for i, e := range p {
			if y[i] != cv.Pix[e]&1 && change(out, e, y[i], signs) {
				n++
			}
		}
		return n
	}

	// заголовок: поля под маской от R, код RS, STC — как у сообщения
	style := styleFor(opt.Level)
	masked := append([]byte(nil), hb...)
	maskFields(masked, k.master, mode)
	hbits := toBits(rs.Encode(masked, style.parity))
	hpos := pos[:style.elems()]
	hy, _, err := stcEmbed(lsbs(cv, hpos), rho[:len(hpos)], hbits, style.h, style.w)
	if err != nil {
		return nil, rep, err
	}
	rep.HeaderChanges = apply(hpos, hy)

	// сообщение
	pay := pos[len(hpos):]
	y, _, err := stcEmbed(lsbs(cv, pay), rho[len(hpos):], msg, rep.H, rep.W)
	if err != nil {
		return nil, rep, err
	}
	rep.Changes = apply(pay, y)
	return out, rep, nil
}

func lsbs(c *Cover, pos []int) []uint8 {
	x := make([]uint8, len(pos))
	for i, e := range pos {
		x[i] = c.Pix[e] & 1
	}
	return x
}

// readHeader пытается прочитать заголовок способом style. Возвращает поля и
// 26 байт заголовка без маски — дополнительные данные шифра.
func readHeader(cv *Cover, p *Perm, style headerStyle, k keys, mode suite.Mode) (*Header, []byte, bool) {
	pos, ok := positions(p, cv, style.elems())
	if !ok {
		return nil, nil, false
	}
	nb := style.bits()
	var bits []uint8
	if style.repeat > 0 { // версия 3: большинство из повторов
		bits = make([]uint8, nb)
		for b := range bits {
			votes := 0
			for r := 0; r < style.repeat; r++ {
				votes += int(cv.Pix[pos[r*nb+b]] & 1)
			}
			if 2*votes > style.repeat {
				bits[b] = 1
			}
		}
	} else {
		bits = stcExtract(lsbs(cv, pos), nb, style.h, style.w)
	}
	hc := fromBits(bits)
	if style.version == 3 {
		for i, m := range k.headerMaskV3(mode)[:len(hc)] {
			hc[i] ^= m
		}
	}
	if _, err := rs.Decode(hc, style.parity, nil); err != nil {
		return nil, nil, false
	}
	hb := hc[:headerLen]
	var (
		h    *Header
		err  error
		want headerStyle
	)
	if style.version == 3 {
		if h, err = parseHeaderV3(hb); err == nil {
			want = styleForV3(h.Level)
		}
	} else {
		maskFields(hb, k.master, mode)
		if h, err = parseHeader(hb); err == nil {
			want = styleFor(h.Level)
		}
	}
	if err != nil || want != style || h.Mode != mode {
		return nil, nil, false
	}
	return h, hb, true
}

// Extracted — результат извлечения.
type Extracted struct {
	Text         string
	Mode         suite.Mode
	Level        Level
	Method       compress.Method
	H, W         int
	Corrected    int  // исправлено байтов кодом Рида–Соломона
	UsedErasures bool // понадобились отметки стираний
}

// Extract извлекает текст из изображения.
func Extract(img image.Image, password string, aad []byte) (*Extracted, error) {
	return ExtractCover(FromImage(img), password, aad, nil)
}

// ExtractCover — извлечение из контейнера; master — для экспериментов (nil — из пароля).
func ExtractCover(cv *Cover, password string, aad, master []byte) (*Extracted, error) {
	k := keys{master: master}
	if k.master == nil {
		k.master = kdf.Master(password)
	}
	var (
		hdr   *Header
		hb    []byte
		perm  *Perm
		style headerStyle
	)
search:
	for _, mode := range suite.All {
		p := k.perm(mode, cv.Len())
		for _, st := range headerStyles {
			if h, b, ok := readHeader(cv, p, st, k, mode); ok {
				hdr, hb, perm, style = h, b, p, st
				break search
			}
		}
	}
	if hdr == nil {
		return nil, ErrNotFound
	}

	mode := hdr.Mode
	parity := rsParity(uint8(hdr.Level))
	coded := rs.CodedLen(hdr.Length, parity)
	msgBits := 8 * coded
	need := style.elems() + msgBits*hdr.W
	// длина из заголовка может быть любой (до 2²⁸ байт): сначала сверить с
	// размером изображения, потом выделять память
	if need > usableCount(cv) {
		return &Extracted{Mode: mode, Level: hdr.Level, Method: hdr.Method, H: hdr.H, W: hdr.W}, ErrDamaged
	}
	pos, ok := positions(perm, cv, need)
	if !ok {
		return nil, ErrDamaged
	}
	pay := pos[style.elems():]
	stream := fromBits(stcExtract(lsbs(cv, pay), msgBits, hdr.H, hdr.W))

	aead, err := mode.NewAEAD(kdf.Derive(k.master, hdr.Nonce[:], infoAEAD(mode), mode.KeySize()))
	if err != nil {
		return nil, err
	}
	ad := append(append([]byte(nil), hb...), aad...)
	res := &Extracted{Mode: mode, Level: hdr.Level, Method: hdr.Method, H: hdr.H, W: hdr.W}

	open := func(erased []bool) ([]byte, int, error) {
		ct, st, err := rs.DecodeStream(stream, hdr.Length, parity, erased)
		if err != nil {
			return nil, 0, ErrDamaged
		}
		pt, err := aead.Open(nil, hdr.aeadNonce(), ct, ad)
		if err != nil {
			if st.Corrected == 0 {
				return nil, 0, ErrAuth
			}
			return nil, st.Corrected, ErrDamaged
		}
		return pt, st.Corrected, nil
	}
	comp, corrected, err := open(nil)
	if err != nil && !errors.Is(err, ErrAuth) {
		if c2, n2, err2 := open(erasures(cv, pay, msgBits, hdr.H, hdr.W)); err2 == nil {
			comp, corrected, err = c2, n2, nil
			res.UsedErasures = true
		}
	}
	if err != nil {
		return res, err
	}
	res.Corrected = corrected
	sym, err := compress.Decompress(hdr.Method, comp)
	if err != nil {
		return res, ErrDamaged
	}
	text, err := alphabet.Decode(sym)
	if err != nil {
		return res, ErrDamaged
	}
	res.Text = text
	return res, nil
}

// erasures отмечает байты потока, которые зависят от подозрительных
// элементов: элемент подозрителен, если окрестность 5×5 в его канале
// одноцветна (так выглядят закрашенные области; в естественных фотографиях
// такие места редки, а HILL их почти не меняет).
func erasures(cv *Cover, pay []int, msgBits, h, w int) []bool {
	er := make([]bool, msgBits/8)
	for t, e := range pay {
		if !flat5(cv, e) {
			continue
		}
		first, last := stcInfluence(t, msgBits, h, w)
		for b := first / 8; b <= last/8; b++ {
			er[b] = true
		}
	}
	return er
}

func flat5(c *Cover, e int) bool {
	ch, p := e%c.C, e/c.C
	x0, y0 := p%c.W, p/c.W
	v := c.Pix[e]
	for y := max(0, y0-2); y <= min(c.H-1, y0+2); y++ {
		for x := max(0, x0-2); x <= min(c.W-1, x0+2); x++ {
			if c.Pix[(y*c.W+x)*c.C+ch] != v {
				return false
			}
		}
	}
	return true
}

// UsableCount — число элементов, пригодных для встраивания.
func UsableCount(c *Cover) int { return usableCount(c) }
