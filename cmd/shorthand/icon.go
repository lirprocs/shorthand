package main

import (
	"image"
	"image/color"
	"math"
)

// Значок программы — Samples/ico.ico (зелёный замок на чёрном круге с
// двоичными цифрами, 256×256). Для заголовка окна и панели задач нужны
// значки 16–64 px: при уменьшении цифры превращаются в шум, поэтому малые
// значки рисуются здесь по той же геометрии, но без цифр, со сглаживанием
// 8×8 выборок на пиксель. Координаты — в системе исходника 256×256.

var (
	iconGreen = color.NRGBA{29, 230, 18, 255}
	iconBlack = color.NRGBA{0, 0, 0, 255}
)

// iconColor — цвет точки (x, y) в координатах 0…256; ok = false — вне круга.
func iconColor(x, y float64) (c color.NRGBA, ok bool) {
	dx, dy := x-128, y-128
	r := math.Hypot(dx, dy)
	switch {
	case r > 128:
		return color.NRGBA{}, false
	case r > 116: // кольцо
		return iconGreen, true
	}
	inRect := func(x0, y0, x1, y1 float64) bool { return x >= x0 && x < x1 && y >= y0 && y < y1 }
	if inRect(62, 116, 194, 191) || inRect(62, 198, 194, 218) { // корпус и планка
		return iconGreen, true
	}
	// дужка: полукольцо r = 42…56 с центром (128, 81) и ножки до корпуса
	sx, sy := x-128, y-81
	sr := math.Hypot(sx, sy)
	if y <= 81 && sr >= 42 && sr <= 56 {
		return iconGreen, true
	}
	if y > 81 && y < 117 && ((x >= 72 && x < 86) || (x >= 170 && x < 184)) {
		return iconGreen, true
	}
	return iconBlack, true
}

// renderIcon рисует упрощённый значок size×size.
func renderIcon(size int) *image.NRGBA {
	const ss = 8 // выборок по каждой оси
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	scale := 256 / float64(size)
	for py := 0; py < size; py++ {
		for px := 0; px < size; px++ {
			var r, g, b, a float64
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					x := (float64(px) + (float64(sx)+0.5)/ss) * scale
					y := (float64(py) + (float64(sy)+0.5)/ss) * scale
					if c, ok := iconColor(x, y); ok {
						r += float64(c.R)
						g += float64(c.G)
						b += float64(c.B)
						a++
					}
				}
			}
			if a == 0 {
				continue
			}
			// среднее по закрашенным выборкам, прозрачность — доля закрашенных
			img.SetNRGBA(px, py, color.NRGBA{
				R: uint8(r/a + 0.5), G: uint8(g/a + 0.5), B: uint8(b/a + 0.5),
				A: uint8(255*a/(ss*ss) + 0.5),
			})
		}
	}
	return img
}
