// Package stego — встраивание и извлечение (формат v3).
package stego

import (
	"image"
	"image/color"
)

// Cover — изображение-контейнер как массив элементов: значение одного канала
// одного пикселя. Элемент e = (y·W + x)·C + c.
type Cover struct {
	W, H, C int
	Pix     []uint8 // W·H·C значений
	Alpha   []uint8 // прозрачность по пикселям (nil — непрозрачное)
}

// Len — число элементов.
func (c *Cover) Len() int { return len(c.Pix) }

// Usable — можно ли использовать элемент. Полностью прозрачные пиксели не
// используются: альфа-канал не меняется, поэтому декодер пропускает те же
// элементы, а другие программы могут обнулить цвет прозрачных пикселей.
func (c *Cover) Usable(e int) bool {
	return c.Alpha == nil || c.Alpha[e/c.C] != 0
}

// FromImage строит контейнер. Изображения в градациях серого (в том числе
// цветные с R = G = B во всех пикселях) дают C = 1, остальные — C = 3.
// 16-битные изображения приводятся к 8 битам.
func FromImage(img image.Image) *Cover {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if g, ok := asGray(img); ok {
		return &Cover{W: w, H: h, C: 1, Pix: g}
	}
	cv := &Cover{W: w, H: h, C: 3, Pix: make([]uint8, w*h*3)}
	alpha := make([]uint8, w*h)
	opaque := true
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var nc color.NRGBA
			switch m := img.(type) {
			case *image.NRGBA:
				p := m.Pix[m.PixOffset(b.Min.X+x, b.Min.Y+y):]
				nc = color.NRGBA{p[0], p[1], p[2], p[3]}
			case *image.RGBA:
				p := m.Pix[m.PixOffset(b.Min.X+x, b.Min.Y+y):]
				if p[3] == 255 {
					nc = color.NRGBA{p[0], p[1], p[2], 255}
				} else {
					nc = color.NRGBAModel.Convert(color.RGBA{p[0], p[1], p[2], p[3]}).(color.NRGBA)
				}
			default:
				nc = color.NRGBAModel.Convert(img.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA)
			}
			i := y*w + x
			cv.Pix[3*i], cv.Pix[3*i+1], cv.Pix[3*i+2] = nc.R, nc.G, nc.B
			alpha[i] = nc.A
			if nc.A != 255 {
				opaque = false
			}
		}
	}
	if !opaque {
		cv.Alpha = alpha
	}
	if opaque && isGrayRGB(cv.Pix) {
		g := make([]uint8, w*h)
		for i := range g {
			g[i] = cv.Pix[3*i]
		}
		return &Cover{W: w, H: h, C: 1, Pix: g}
	}
	return cv
}

func asGray(img image.Image) ([]uint8, bool) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	switch m := img.(type) {
	case *image.Gray:
		out := make([]uint8, w*h)
		for y := 0; y < h; y++ {
			copy(out[y*w:(y+1)*w], m.Pix[(y)*m.Stride:(y)*m.Stride+w])
		}
		return out, true
	case *image.Paletted:
		// 8-битный BMP в градациях серого — палитра, где индекс i означает серый i
		for i, c := range m.Palette {
			r, g, bb, a := c.RGBA()
			v := uint32(i) * 0x101
			if a != 0xffff || r != v || g != v || bb != v {
				return nil, false
			}
		}
		out := make([]uint8, w*h)
		for y := 0; y < h; y++ {
			copy(out[y*w:(y+1)*w], m.Pix[y*m.Stride:y*m.Stride+w])
		}
		return out, true
	case *image.Gray16:
		out := make([]uint8, w*h)
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				out[y*w+x] = uint8(m.Gray16At(b.Min.X+x, b.Min.Y+y).Y >> 8)
			}
		}
		return out, true
	}
	return nil, false
}

func isGrayRGB(p []uint8) bool {
	for i := 0; i < len(p); i += 3 {
		if p[i] != p[i+1] || p[i] != p[i+2] {
			return false
		}
	}
	return true
}

// Image возвращает изображение для сохранения: *image.Gray или *image.NRGBA.
func (c *Cover) Image() image.Image {
	if c.C == 1 {
		g := image.NewGray(image.Rect(0, 0, c.W, c.H))
		copy(g.Pix, c.Pix)
		return g
	}
	out := image.NewNRGBA(image.Rect(0, 0, c.W, c.H))
	for i := 0; i < c.W*c.H; i++ {
		out.Pix[4*i] = c.Pix[3*i]
		out.Pix[4*i+1] = c.Pix[3*i+1]
		out.Pix[4*i+2] = c.Pix[3*i+2]
		if c.Alpha != nil {
			out.Pix[4*i+3] = c.Alpha[i]
		} else {
			out.Pix[4*i+3] = 255
		}
	}
	return out
}

// Clone — копия контейнера.
func (c *Cover) Clone() *Cover {
	d := *c
	d.Pix = append([]uint8(nil), c.Pix...)
	if c.Alpha != nil {
		d.Alpha = append([]uint8(nil), c.Alpha...)
	}
	return &d
}
