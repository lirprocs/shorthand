package imageio

// Этап 1 «на потом» (ветка later/stage1, в v3 не входит): «чистый файл».
//
// Картинка с сообщением должна выглядеть как пересохранённый оригинал, а не
// как продукт другой программы. Поэтому при записи повторяются свойства
// исходного файла: тип цвета и разрядность PNG, его служебные блоки
// (цветовой профиль, гамма, разрешение, текст, EXIF, дата), разрядность и
// разрешение BMP. Что повторить нельзя (палитра, 16 бит, чересстрочность),
// перечисляет Meta.Changes — программа предупреждает об этом.

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	"io"
	"os"
	"time"

	"golang.org/x/image/bmp"
)

// Chunk — служебный блок PNG.
type Chunk struct {
	Type       string
	Data       []byte
	BeforeIDAT bool // стоял до данных изображения
}

// Meta — свойства исходного файла.
type Meta struct {
	Format  Format
	ModTime time.Time

	// PNG
	ColorType  uint8 // 0 серый, 2 RGB, 3 палитра, 4 серый+альфа, 6 RGBA
	BitDepth   uint8
	Interlaced bool
	Chunks     []Chunk // служебные (ancillary) блоки в исходном порядке

	// BMP
	BMPBits       int
	BMPHeaderSize int
	XPPM, YPPM    int32
	TopDown       bool
}

var pngSig = []byte("\x89PNG\r\n\x1a\n")

// Inspect читает свойства файла.
func Inspect(path string) (*Meta, error) {
	f, err := FormatOf(path)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	m := &Meta{Format: f, ModTime: st.ModTime()}
	switch f {
	case PNG:
		err = m.parsePNG(data)
	case BMP:
		err = m.parseBMP(data)
	}
	if err != nil {
		return nil, err
	}
	return m, nil
}

func (m *Meta) parsePNG(d []byte) error {
	if !bytes.HasPrefix(d, pngSig) {
		return errors.New("не PNG")
	}
	p := len(pngSig)
	seenIDAT := false
	for p+12 <= len(d) {
		n := int(binary.BigEndian.Uint32(d[p:]))
		typ := string(d[p+4 : p+8])
		if n < 0 || p+12+n > len(d) {
			return errors.New("повреждённый PNG")
		}
		data := d[p+8 : p+8+n]
		switch {
		case typ == "IHDR" && n >= 13:
			m.BitDepth, m.ColorType = data[8], data[9]
			m.Interlaced = data[12] != 0
		case typ == "IDAT":
			seenIDAT = true
		case typ == "IEND":
			return nil
		case typ[0] >= 'a' && typ[0] <= 'z': // служебный блок
			m.Chunks = append(m.Chunks, Chunk{Type: typ, Data: append([]byte(nil), data...), BeforeIDAT: !seenIDAT})
		}
		p += 12 + n
	}
	return nil
}

func (m *Meta) parseBMP(d []byte) error {
	if len(d) < 54 || d[0] != 'B' || d[1] != 'M' {
		return errors.New("не BMP")
	}
	m.BMPHeaderSize = int(binary.LittleEndian.Uint32(d[14:]))
	h := int32(binary.LittleEndian.Uint32(d[22:]))
	m.TopDown = h < 0
	m.BMPBits = int(binary.LittleEndian.Uint16(d[28:]))
	m.XPPM = int32(binary.LittleEndian.Uint32(d[38:]))
	m.YPPM = int32(binary.LittleEndian.Uint32(d[42:]))
	return nil
}

// Changes — чем файл с сообщением неизбежно будет отличаться от исходного.
func (m *Meta) Changes() []string {
	var out []string
	if m.Format == PNG {
		if m.ColorType == 3 {
			out = append(out, "PNG с палитрой будет сохранён полноцветным")
		}
		if m.BitDepth == 16 {
			out = append(out, "16-битный PNG будет сохранён 8-битным")
		}
		if m.Interlaced {
			out = append(out, "чересстрочный PNG будет сохранён обычным")
		}
	}
	if m.Format == BMP && m.BMPBits != 0 && m.BMPBits != 24 && m.BMPBits != 32 && m.BMPBits != 8 {
		out = append(out, fmt.Sprintf("%d-битный BMP будет сохранён 24-битным", m.BMPBits))
	}
	return out
}

// SaveLike записывает изображение в формате f, повторяя свойства исходного
// файла m (если формат тот же), и при keepTime — его дату изменения.
// Запись — через временный файл.
func SaveLike(path string, img image.Image, f Format, m *Meta, keepTime bool) error {
	tmp := path + ".tmp"
	file, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("не удалось создать файл: %w", err)
	}
	w := bufio.NewWriter(file)
	same := m != nil && m.Format == f
	switch f {
	case PNG:
		var chunks []Chunk
		var origType, origDepth uint8
		if same {
			chunks, origType, origDepth = m.Chunks, m.ColorType, m.BitDepth
		}
		err = writePNG(w, img, origType, origDepth, chunks)
	case BMP:
		if same {
			err = writeBMP(w, img, m)
		} else {
			err = writeBMP(w, img, &Meta{})
		}
	default:
		err = fmt.Errorf("неизвестный формат %q", f)
	}
	if err == nil {
		err = w.Flush()
	}
	if cerr := file.Close(); err == nil {
		err = cerr
	}
	if err == nil && keepTime && m != nil && !m.ModTime.IsZero() {
		err = os.Chtimes(tmp, time.Now(), m.ModTime)
	}
	if err != nil {
		os.Remove(tmp)
		return fmt.Errorf("не удалось записать изображение: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("не удалось сохранить файл: %w", err)
	}
	return nil
}

// ─── PNG ────────────────────────────────────────────────────────────────────

func writeChunk(w io.Writer, typ string, data []byte) error {
	var hdr [8]byte
	binary.BigEndian.PutUint32(hdr[:4], uint32(len(data)))
	copy(hdr[4:], typ)
	crc := crc32.NewIEEE()
	crc.Write(hdr[4:8])
	crc.Write(data)
	var tail [4]byte
	binary.BigEndian.PutUint32(tail[:], crc.Sum32())
	for _, b := range [][]byte{hdr[:], data, tail[:]} {
		if _, err := w.Write(b); err != nil {
			return err
		}
	}
	return nil
}

// Блоки, смысл которых не зависит от младших битов пикселей; остальные
// известные — только если тип цвета и разрядность не изменились; неизвестные —
// по правилу PNG: копировать можно лишь помеченные «safe-to-copy».
var keepAlways = map[string]bool{
	"iCCP": true, "sRGB": true, "gAMA": true, "cHRM": true, "cICP": true, "mDCv": true, "cLLi": true,
	"pHYs": true, "tEXt": true, "zTXt": true, "iTXt": true, "tIME": true, "eXIf": true,
	"oFFs": true, "sCAL": true, "pCAL": true, "sTER": true,
}
var keepIfSameType = map[string]bool{"tRNS": true, "bKGD": true, "sBIT": true}

func keepChunk(c Chunk, sameType bool) bool {
	if keepAlways[c.Type] {
		return true
	}
	if keepIfSameType[c.Type] {
		return sameType
	}
	switch c.Type {
	case "hIST", "sPLT": // относятся к палитре
		return false
	}
	return c.Type[3] >= 'a' && c.Type[3] <= 'z' // safe-to-copy
}

// writePNG — 8 бит, тип цвета: серый (0), RGB (2) или RGBA (6); RGBA — если
// есть прозрачность или исходник был RGBA. Фильтры строк — как в libpng
// (минимум суммы модулей), сжатие zlib уровня 6, блоки IDAT по 8 КиБ.
func writePNG(w io.Writer, img image.Image, origType, origDepth uint8, chunks []Chunk) error {
	b := img.Bounds()
	width, height := b.Dx(), b.Dy()
	var ct uint8
	var bpp int
	var row func(y int, dst []byte)
	switch im := img.(type) {
	case *image.Gray:
		ct, bpp = 0, 1
		row = func(y int, dst []byte) { copy(dst, im.Pix[y*im.Stride:y*im.Stride+width]) }
	case *image.NRGBA:
		opaque := im.Opaque()
		if !opaque || origType == 6 || origType == 4 {
			ct, bpp = 6, 4
			row = func(y int, dst []byte) { copy(dst, im.Pix[y*im.Stride:y*im.Stride+4*width]) }
		} else {
			ct, bpp = 2, 3
			row = func(y int, dst []byte) {
				src := im.Pix[y*im.Stride:]
				for x := 0; x < width; x++ {
					dst[3*x], dst[3*x+1], dst[3*x+2] = src[4*x], src[4*x+1], src[4*x+2]
				}
			}
		}
	default:
		return fmt.Errorf("writePNG: неподдерживаемое изображение %T", img)
	}
	sameType := ct == origType && origDepth == 8

	if _, err := w.Write(pngSig); err != nil {
		return err
	}
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:], uint32(width))
	binary.BigEndian.PutUint32(ihdr[4:], uint32(height))
	ihdr[8], ihdr[9] = 8, ct
	if err := writeChunk(w, "IHDR", ihdr); err != nil {
		return err
	}
	for _, c := range chunks {
		if c.BeforeIDAT && keepChunk(c, sameType) {
			if err := writeChunk(w, c.Type, c.Data); err != nil {
				return err
			}
		}
	}

	// данные: фильтр каждой строки — с наименьшей суммой модулей
	var z bytes.Buffer
	zw, _ := zlib.NewWriterLevel(&z, zlib.DefaultCompression)
	stride := width * bpp
	prev := make([]byte, stride)
	cur := make([]byte, stride)
	cand := make([][]byte, 5)
	for i := range cand {
		cand[i] = make([]byte, stride+1)
	}
	for y := 0; y < height; y++ {
		row(y, cur)
		best, bestSum := 0, -1
		for f := 0; f < 5; f++ {
			out := cand[f]
			out[0] = byte(f)
			sum := 0
			for i := 0; i < stride; i++ {
				var a, up, c byte
				if i >= bpp {
					a, c = cur[i-bpp], prev[i-bpp]
				}
				up = prev[i]
				var v byte
				switch f {
				case 0:
					v = cur[i]
				case 1:
					v = cur[i] - a
				case 2:
					v = cur[i] - up
				case 3:
					v = cur[i] - byte((int(a)+int(up))/2)
				case 4:
					v = cur[i] - paeth(a, up, c)
				}
				out[i+1] = v
				if v < 128 {
					sum += int(v)
				} else {
					sum += 256 - int(v)
				}
			}
			if bestSum < 0 || sum < bestSum {
				best, bestSum = f, sum
			}
		}
		if _, err := zw.Write(cand[best]); err != nil {
			return err
		}
		prev, cur = cur, prev
	}
	if err := zw.Close(); err != nil {
		return err
	}
	data := z.Bytes()
	for len(data) > 0 {
		n := min(len(data), 8192)
		if err := writeChunk(w, "IDAT", data[:n]); err != nil {
			return err
		}
		data = data[n:]
	}
	for _, c := range chunks {
		if !c.BeforeIDAT && keepChunk(c, sameType) {
			if err := writeChunk(w, c.Type, c.Data); err != nil {
				return err
			}
		}
	}
	return writeChunk(w, "IEND", nil)
}

func paeth(a, b, c byte) byte {
	p := int(a) + int(b) - int(c)
	pa, pb, pc := abs(p-int(a)), abs(p-int(b)), abs(p-int(c))
	switch {
	case pa <= pb && pa <= pc:
		return a
	case pb <= pc:
		return b
	}
	return c
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// ─── BMP ────────────────────────────────────────────────────────────────────

// writeBMP — заголовок BITMAPINFOHEADER (40 байт), 24 или 32 бита (как в
// исходнике) с его разрешением и порядком строк. Серые и полупрозрачные
// изображения пишет golang.org/x/image/bmp (палитра 8 бит / 32 бита с альфой).
func writeBMP(w io.Writer, img image.Image, m *Meta) error {
	im, ok := img.(*image.NRGBA)
	if !ok || !im.Opaque() {
		return bmp.Encode(w, img)
	}
	b := im.Bounds()
	width, height := b.Dx(), b.Dy()
	bits := 24
	if m.BMPBits == 32 {
		bits = 32
	}
	rowSize := (width*bits/8 + 3) &^ 3
	imgSize := rowSize * height
	hdr := make([]byte, 54)
	hdr[0], hdr[1] = 'B', 'M'
	binary.LittleEndian.PutUint32(hdr[2:], uint32(54+imgSize))
	binary.LittleEndian.PutUint32(hdr[10:], 54)
	binary.LittleEndian.PutUint32(hdr[14:], 40)
	binary.LittleEndian.PutUint32(hdr[18:], uint32(width))
	hh := int32(height)
	if m.TopDown {
		hh = -hh
	}
	binary.LittleEndian.PutUint32(hdr[22:], uint32(hh))
	binary.LittleEndian.PutUint16(hdr[26:], 1)
	binary.LittleEndian.PutUint16(hdr[28:], uint16(bits))
	binary.LittleEndian.PutUint32(hdr[34:], uint32(imgSize))
	binary.LittleEndian.PutUint32(hdr[38:], uint32(m.XPPM))
	binary.LittleEndian.PutUint32(hdr[42:], uint32(m.YPPM))
	if _, err := w.Write(hdr); err != nil {
		return err
	}
	line := make([]byte, rowSize)
	for i := 0; i < height; i++ {
		y := height - 1 - i
		if m.TopDown {
			y = i
		}
		src := im.Pix[y*im.Stride:]
		for x := 0; x < width; x++ {
			p := src[4*x : 4*x+4]
			if bits == 24 {
				line[3*x], line[3*x+1], line[3*x+2] = p[2], p[1], p[0]
			} else {
				line[4*x], line[4*x+1], line[4*x+2], line[4*x+3] = p[2], p[1], p[0], 255
			}
		}
		if _, err := w.Write(line); err != nil {
			return err
		}
	}
	return nil
}
