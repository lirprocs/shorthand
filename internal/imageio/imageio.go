// Package imageio — чтение и запись контейнеров PNG и BMP.
package imageio

import (
	"bufio"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/image/bmp"
)

// Format — формат файла.
type Format string

const (
	PNG Format = "png"
	BMP Format = "bmp"
)

// FormatOf определяет формат по расширению.
func FormatOf(path string) (Format, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png":
		return PNG, nil
	case ".bmp":
		return BMP, nil
	}
	return "", fmt.Errorf("формат %q не поддерживается: используйте PNG или BMP (форматы со сжатием с потерями, например JPEG, разрушают встроенные данные)", filepath.Ext(path))
}

// Load читает изображение.
func Load(path string) (image.Image, Format, error) {
	f, err := FormatOf(path)
	if err != nil {
		return nil, "", err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, "", fmt.Errorf("не удалось открыть файл: %w", err)
	}
	defer file.Close()
	var img image.Image
	switch f {
	case PNG:
		img, err = png.Decode(bufio.NewReader(file))
	case BMP:
		img, err = bmp.Decode(bufio.NewReader(file))
	}
	if err != nil {
		return nil, "", fmt.Errorf("не удалось прочитать изображение: %w", err)
	}
	return img, f, nil
}

// Save записывает изображение. Файл сначала пишется рядом под временным
// именем и затем переименовывается, чтобы при ошибке не остался обрезанный файл.
func Save(path string, img image.Image, f Format) error {
	tmp := path + ".tmp"
	file, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("не удалось создать файл: %w", err)
	}
	w := bufio.NewWriter(file)
	switch f {
	case PNG:
		enc := png.Encoder{CompressionLevel: png.BestCompression}
		err = enc.Encode(w, img)
	case BMP:
		err = bmp.Encode(w, img)
	default:
		err = fmt.Errorf("неизвестный формат %q", f)
	}
	if err == nil {
		err = w.Flush()
	}
	if cerr := file.Close(); err == nil {
		err = cerr
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
