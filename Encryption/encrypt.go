package encrypt

import (
	crypto "crypto/rand"
	"fmt"
	mgm "github.com/lirprocs/MGM"
	"image"
	"image/color"
	"math/rand"
	alphabet "shorthand/Alphabet"
	"strconv"
	"sync"
)

const X1, Y1 = 8, 8

var Separator = []byte{0x00, 0xFF, 0x00}

type MyUint interface {
	uint | uint8 | uint16 | uint32 | uint64
}

func UintToBinary[T MyUint](num T, osn int) string {
	binaryStr := ""
	for i := osn; i >= 0; i-- {
		bit := (num >> i) & 1
		binaryStr += fmt.Sprint(bit)
	}
	return binaryStr
}

type Changeable interface {
	image.Image
	Set(x, y int, c color.Color)
}

func GetSeed(seed string) int64 {
	var numSeed int64
	for num, r := range seed {
		if num%2 == 0 {
			numSeed += int64(r) * int64(num)
		} else {
			numSeed -= int64(r) / int64(num)
		}
	}
	//fmt.Println(numSeed)
	return numSeed
}

func StringToBin(wg *sync.WaitGroup, s string) []uint8 {
	list := make([]uint8, len(s))
	for i, c := range s {
		wg.Add(1)
		go func(p int, char int32) {
			defer wg.Done()
			if binaryCode, ok := alphabet.RussianDictionary[char]; ok {
				binaryBytes, _ := strconv.ParseUint(binaryCode, 2, 8)
				list[p] = byte(binaryBytes)
			} else {
				list[p] = uint8(char)
			}
		}(i, c)
	}
	wg.Wait()
	return list
}

func SetInfo(file Changeable, cipher bool, lenText int, pol *map[int][]int) Changeable { // Функция записи данных о длине и типе
	x, y := X1, Y1
	cimg := file.(Changeable)
	binary := UintToBinary(uint64(lenText), 63)
	i0 := 40
	i1 := 44
	i2 := 48
	i3 := 52
	for i := 0; i < 2; i++ {
		numR, _ := strconv.ParseUint("0000"+binary[i0:i1], 2, 8)
		numG, _ := strconv.ParseUint("0000"+binary[i1:i2], 2, 8)
		numB, _ := strconv.ParseUint("0000"+binary[i2:i3], 2, 8)
		i0 = i3
		i1 = i0 + 4
		i2 = i0 + 8
		i3 = i0 + 12
		oldColor := cimg.At(x, y).(color.RGBA)
		newColor := color.RGBA{
			R: ((oldColor.R & uint8(0b11110000)) | uint8(numR)),
			G: ((oldColor.G & uint8(0b11110000)) | uint8(numG)),
			B: ((oldColor.B & uint8(0b11110000)) | uint8(numB)),
			A: oldColor.A,
		}
		cimg.Set(x, y, newColor)
		x, y = x+1, y+1
		(*pol)[y] = append((*pol)[y], x)
	}
	//fmt.Println(pol)
	return cimg
}

func ChangeIMG(wg *sync.WaitGroup, x, y, i int, bin []uint8, img Changeable) {
	defer wg.Done()
	//var mu sync.Mutex
	var chunk uint8
	i = i - 1
	chunk = bin[i]
	oldColor := img.At(x, y).(color.RGBA)
	newColor := color.RGBA{
		R: ((oldColor.R & uint8(0b11111000)) | (chunk >> 5 & uint8(0b00000111))),
		G: ((oldColor.G & uint8(0b11111100)) | (chunk >> 3 & uint8(0b00000011))),
		B: ((oldColor.B & uint8(0b11111000)) | (chunk >> 0 & uint8(0b00000111))),
		A: oldColor.A,
	}
	//mu.Lock()
	img.Set(x, y, newColor)
	//mu.Unlock()
}

func mergeSlices(cipherText [][16]byte, t []byte, nonce [16]byte) []byte {
	var res []byte
	for _, v := range cipherText {
		res = append(res, v[:]...)
	}

	res = append(res, Separator...)
	res = append(res, t...)
	res = append(res, Separator...)
	res = append(res, nonce[:]...)

	return res
}

func GetPosition(wg *sync.WaitGroup, cipher bool, seedOld string, plainText, aText string, file image.Image) (image.Image, string) {
	var key [32]byte
	var nonce [16]byte
	_, err := crypto.Read(nonce[:])
	if err != nil {
		nonce = [16]byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x00, 0xff, 0xee, 0xdd, 0xcc, 0xbb, 0xaa, 0x99, 0x88}
	}

	img := file.(Changeable)
	pol := map[int][]int{
		Y1: {X1},
	}
	i := 0
	seed := GetSeed(seedOld)
	copy(key[:], seedOld)
	pText := StringToBin(wg, plainText)
	imitoText := StringToBin(wg, aText) //TODO
	err, b, t := mgm.Encrypt(imitoText, pText, key, nonce)
	bin := mergeSlices(b, t, nonce)
	if err != nil {
		return nil, fmt.Sprintf("%e", err)
	}
	img = SetInfo(img, cipher, len(bin), &pol)
	bounds := img.Bounds()
	width, height := bounds.Max.X, bounds.Max.Y
	if width*height-len(pol) >= len(bin) {
		rand.Seed(seed)
		for i < len(bin) {
			y := rand.Intn(height)
			x := rand.Intn(width)
			found := false
			for _, val := range pol[y] {
				if val == x {
					found = true
					break
				}
			}
			if !found {
				pol[y] = append(pol[y], x)
				i++
				wg.Add(1)
				go ChangeIMG(wg, x, y, i, bin, img)
			}
		}
	} else {
		return nil, fmt.Sprintf("Выберите файл большего размера, весь текст не поместится")
	}
	wg.Wait()
	//fmt.Println(pol)
	return img, ""
}
