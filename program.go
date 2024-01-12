package main

import (
	"fmt"
	"golang.org/x/image/bmp"
	"image"
	"image/color"
	"image/png"
	_ "log"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

var wg sync.WaitGroup

//var mu sync.Mutex

var russianDictionary = map[rune]string{
	'А':  "11000010",
	'Б':  "11000011",
	'В':  "11000100",
	'Г':  "11000101",
	'Д':  "11000110",
	'Е':  "11000111",
	'Ё':  "11001000",
	'Ж':  "11001001",
	'З':  "11001010",
	'И':  "11001011",
	'Й':  "11001100",
	'К':  "11001101",
	'Л':  "11001110",
	'М':  "11001111",
	'Н':  "11010000",
	'О':  "11010001",
	'П':  "11010010",
	'Р':  "11010011",
	'С':  "11010100",
	'Т':  "11010101",
	'У':  "11010110",
	'Ф':  "11010111",
	'Х':  "11011000",
	'Ц':  "11011001",
	'Ч':  "11011010",
	'Ш':  "11011011",
	'Щ':  "11011100",
	'Ъ':  "11011101",
	'Ы':  "11011110",
	'Ь':  "11011111",
	'Э':  "11100000",
	'Ю':  "11100001",
	'Я':  "11100010",
	'а':  "11000010",
	'б':  "11000011",
	'в':  "11000100",
	'г':  "11000101",
	'д':  "11000110",
	'е':  "11000111",
	'ё':  "11001000",
	'ж':  "11001001",
	'з':  "11001010",
	'и':  "11001011",
	'й':  "11001100",
	'к':  "11001101",
	'л':  "11001110",
	'м':  "11001111",
	'н':  "11010000",
	'о':  "11010001",
	'п':  "11010010",
	'р':  "11010011",
	'с':  "11010100",
	'т':  "11010101",
	'у':  "11010110",
	'ф':  "11010111",
	'х':  "11011000",
	'ц':  "11011001",
	'ч':  "11011010",
	'ш':  "11011011",
	'щ':  "11011100",
	'ъ':  "11011101",
	'ы':  "11011110",
	'ь':  "11011111",
	'э':  "11100000",
	'ю':  "11100001",
	'я':  "11100010",
	'!':  "00100001",
	'"':  "00100010",
	'#':  "00100011",
	'$':  "00100100",
	'%':  "00100101",
	'&':  "00100110",
	'\'': "00100111",
	'(':  "00101000",
	')':  "00101001",
	'*':  "00101010",
	'+':  "00101011",
	',':  "00101100",
	'-':  "00101101",
	'.':  "00101110",
	'/':  "00101111",
	':':  "00111010",
	';':  "00111011",
	'<':  "00111100",
	'=':  "00111101",
	'>':  "00111110",
	'?':  "00111111",
	'@':  "01000000",
	'[':  "01011011",
	'\\': "01011100",
	']':  "01011101",
	'^':  "01011110",
	'_':  "01011111",
	'`':  "01100000",
	'{':  "01111011",
	'|':  "01111100",
	'}':  "01111101",
	'~':  "01111110",
}

var reversedRussianDictionary = map[string]rune{
	"11000010": 'А',
	"11000011": 'Б',
	"11000100": 'В',
	"11000101": 'Г',
	"11000110": 'Д',
	"11000111": 'Е',
	"11001000": 'Ё',
	"11001001": 'Ж',
	"11001010": 'З',
	"11001011": 'И',
	"11001100": 'Й',
	"11001101": 'К',
	"11001110": 'Л',
	"11001111": 'М',
	"11010000": 'Н',
	"11010001": 'О',
	"11010010": 'П',
	"11010011": 'Р',
	"11010100": 'С',
	"11010101": 'Т',
	"11010110": 'У',
	"11010111": 'Ф',
	"11011000": 'Х',
	"11011001": 'Ц',
	"11011010": 'Ч',
	"11011011": 'Ш',
	"11011100": 'Щ',
	"11011101": 'Ъ',
	"11011110": 'Ы',
	"11011111": 'Ь',
	"11100000": 'Э',
	"11100001": 'Ю',
	"11100010": 'Я',
	"00100001": '!',
	"00100010": '"',
	"00100011": '#',
	"00100100": '$',
	"00100101": '%',
	"00100110": '&',
	"00100111": '\'',
	"00101000": '(',
	"00101001": ')',
	"00101010": '*',
	"00101011": '+',
	"00101100": ',',
	"00101101": '-',
	"00101110": '.',
	"00101111": '/',
	"00111010": ':',
	"00111011": ';',
	"00111100": '<',
	"00111101": '=',
	"00111110": '>',
	"00111111": '?',
	"01000000": '@',
	"01011011": '[',
	"01011100": '\\',
	"01011101": ']',
	"01011110": '^',
	"01011111": '_',
	"01100000": '`',
	"01111011": '{',
	"01111100": '|',
	"01111101": '}',
	"01111110": '~',
}

var x1, y1 = 8, 8

type Changeable interface {
	image.Image
	Set(x, y int, c color.Color)
}

type MyUint interface {
	uint | uint8 | uint16 | uint32 | uint64
}

func uintToBinary[T MyUint](num T, osn int) string {
	binaryStr := ""
	for i := osn; i >= 0; i-- {
		bit := (num >> i) & 1
		binaryStr += fmt.Sprint(bit)
	}
	return binaryStr
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

func GetFile(name string) (image.Image, string, string) {
	file, err := os.Open(name)
	if err != nil {
		return nil, "", fmt.Sprintf("Ошибка открытия файла: %v", err)
	}
	defer file.Close()

	ext := filepath.Ext(name)
	switch ext {
	case ".bmp":
		bmps, err := bmp.Decode(file)
		if err != nil {
			return nil, "", fmt.Sprintf("Ошибка декодирования файла: %v", err)
		}
		return bmps, ext, ""
	case ".png":
		pngs, err := png.Decode(file)
		if err != nil {
			return nil, "", fmt.Sprintf("Ошибка декодирования файла: %v", err)
		}
		return pngs, ext, ""
	default:
		return nil, "", fmt.Sprintf("Не верный тип файла используйте bmp/png")
	}
}

func SaveFile(file image.Image, name string, ext string) string {
	outputFile, err := os.Create(name)
	if err != nil {
		return fmt.Sprintf("Ошибка создания файла: %v", err)
	}
	defer outputFile.Close()

	switch ext {
	case ".bmp":
		err = bmp.Encode(outputFile, file)
		if err != nil {
			return fmt.Sprintf("Ошибка кодирования: %v", err)
		}
	case ".png":
		err = png.Encode(outputFile, file)
		if err != nil {
			return fmt.Sprintf("Ошибка кодирования: %v", err)
		}
	default:
		return ""
	}
	return ""
}

func SetInfo(file Changeable, lenText int, pol *map[int][]int) Changeable { // Функция записи данных о длине и типе
	x, y := x1, y1
	cimg := file.(Changeable)
	binary := uintToBinary(uint64(lenText), 63)
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

func GetInfo(file image.Image, pol *map[int][]int) uint64 {
	x, y := x1, y1
	text := "00000000"
	for i := 0; i < 2; i++ {
		oldColor := file.At(x, y).(color.RGBA)
		r := uintToBinary(oldColor.R, 7)
		g := uintToBinary(oldColor.G, 7)
		b := uintToBinary(oldColor.B, 7)
		text = text + r[4:] + g[4:] + b[4:]
		x, y = x+1, y+1
		(*pol)[y] = append((*pol)[y], x)
	}
	//fmt.Println(text)
	lenn, _ := strconv.ParseUint(text, 2, 64)
	return lenn
}

func GetPosition(wg *sync.WaitGroup, seedOld string, strok string, file image.Image) (image.Image, string) {
	img := file.(Changeable)
	pol := map[int][]int{
		y1: {x1},
	}
	i := 0
	seed := GetSeed(seedOld)
	bin := StringToBin(wg, strok)
	img = SetInfo(img, len(bin), &pol)
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

func StringToBin(wg *sync.WaitGroup, s string) []uint8 {
	list := make([]uint8, len(s))
	for i, c := range s {
		wg.Add(1)
		go func(p int, char int32) {
			defer wg.Done()
			if binaryCode, ok := russianDictionary[char]; ok {
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

//func BinToString(binMap []uint8) []string {
//	list := make([]string, len(binMap))
//	for i, binary := range binMap {
//		wg.Add(1)
//		go func(p int, char uint8) {
//			list[p] = string(rune(char))
//			wg.Done()
//		}(i, binary)
//	}
//	wg.Wait()
//	//fmt.Println(list)
//	return list
//}

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

func GetPositionBack(wg *sync.WaitGroup, file image.Image, seedOld string) string {
	pol := map[int][]int{
		y1: {x1},
	}
	i := uint64(0)
	seed := GetSeed(seedOld)
	lenText := GetInfo(file, &pol)
	bounds := file.Bounds()
	width, height := bounds.Max.X, bounds.Max.Y
	list := make([]uint8, lenText)
	rand.Seed(seed)
	for i < lenText {
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
			go GetBin(wg, file, x, y, i, &list)
			//list = append(list, a)
		}
	}
	wg.Wait()
	//textList := GetText(wg, list)
	text := strings.Join(GetText(wg, list), "")
	return text
}

func GetBin(wg *sync.WaitGroup, file image.Image, x, y int, i uint64, list *[]uint8) {
	defer wg.Done()
	i = i - 1
	oldColor := file.At(x, y).(color.RGBA)
	r := oldColor.R << 5 & uint8(0b11100000)
	g := oldColor.G << 3 & uint8(0b00011000)
	b := oldColor.B << 0 & uint8(0b00000111)
	a := r | g | b
	//*list = append(*list, a)
	//mu.Lock()
	(*list)[i] = a
	//mu.Unlock()
}

func GetText(wg *sync.WaitGroup, list []uint8) []string {
	list1 := make([]string, len(list)+1)
	//result := ""
	for c, bin := range list {
		wg.Add(1)
		go func(c1 int, bin1 uint8) {
			defer wg.Done()
			if char, ok := reversedRussianDictionary[uintToBinary(bin1, 7)]; ok {
				//result += string(char)
				//mu.Lock()
				list1[c1] = string(char)
				//mu.Unlock()
			} else {
				//result += string(bin)
				//mu.Lock()
				list1[c1] = string(bin1)
				//mu.Unlock()
			}
		}(c, bin)
	}
	wg.Wait()
	return list1
}

func ToFile(dirPath, text string) string {
	var cmd *exec.Cmd
	name := "text.txt"
	filePath := filepath.Join(dirPath, name)

	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Sprintf("Ошибка при создании файла: %v", err)
	}
	//defer file.Close()

	_, err = file.WriteString(text)
	if err != nil {
		return fmt.Sprintf("Ошибка при записи в файл: %v", err)
	}

	err = file.Close()
	if err != nil {
		return fmt.Sprintf("Ошибка при закрытии файла: %v", err)
	}

	//cmd = exec.Command("cmd", "/c", "start", filePath)
	switch runtime.GOOS {
	case "darwin": //macOS
		cmd = exec.Command("open", filePath)
	case "linux": //linux
		cmd = exec.Command("xdg-open", filePath)
	case "windows": //linux
		cmd = exec.Command("cmd", "/c", "start", filePath)
	default:
		return ("Неподдерживаемая операционная система")
	}

	err = cmd.Run()
	if err != nil {
		// Обработка ошибок
		return fmt.Sprintf("Ошибка при открытии файла: %v", err)
	}
	return ""
}

//in := "sample.bmp"
//in := "samplePNG.png"
//in := "sampleBMP.bmp"
//out := "output"
//out := "sample_output"
//strok := "Hello my frend!"
//strok = "Привет мой друг!"
//strok := "Привет мой frend!"
//strok := "In the bustling city, where lights never dim and life never slows, people weave through crowded streets, chasing dreams and evading shadows"
//strok1 := "а б в г д е ё ж з и й к л м н о п р с т у ф х ц ч ш щ ъ ы ь э ю я А Б В Г Д Е Ё Ж З И Й К Л М Н О П Р С Т У Ф Х Ц Ч Ш Щ Ъ Ы Ь Э Ю Я , . ! ? - : ; ( ) ' [ ] { } < > / | _ @ # $ % ^ & * + ="
//strok2 := "a b c d e f g h i j k l m n o p q r s t u v w x y z A B C D E F G H I J K L M N O P Q R S T U V W X Y Z , . ! ? - : ; ( ) ' [ ] { } < > /  | _ @ # $ % ^ & * + = "
//strok := strok1 + strok2

//seed := "8812332wkjwjw!@#$%"

//1 Вкладка:
//Поле для получение пути до файла
//in := "sampleBMP.bmp"
//
//file, ext, err := GetFile(in)
//if err != nil {
//	log.Fatal(err)
//}
//
////Поле для ввод с клавиатуры любого текста
//strok := "Привет мой frend!"
////Поле для ввод с клавиатуры пароля
//seed := "8812332wkjwjw!@#$%"
//
//cimg, err := GetPosition(&wg, seed, strok, file)
//if err != nil {
//	log.Fatal(err)
//}
//
////Поле для ввода пути сохранения файла
//out := "output"
//
//err = SaveFile(cimg, out, ext)
//if err != nil {
//	log.Fatal(err)
//}

//1 Вкладка:
//Поле для получение пути до файла
//out1 := "output.bmp"
//
//file2, ext, err := GetFile(out1)
//if err != nil {
//	log.Fatal(err)
//}
//
////Поле для ввод с клавиатуры пароля
//seed = "8812332wkjwjw!@#$%"
//list := GetPositionBack(&wg, file2, seed)
//
////Поле для вывода полученного текста
//fmt.Println(list)
