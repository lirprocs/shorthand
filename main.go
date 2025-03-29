package main

import (
	"bufio"
	"fmt"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/sqweek/dialog"
	"golang.org/x/image/bmp"
	"image"
	"image/color"
	"image/png"
	//"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	decrypt "shorthand/Decryption"
	encrypt "shorthand/Encryption"
	"strings"
	"sync"
)

var wg sync.WaitGroup

func GetFile(name string) (image.Image, string, string) {
	file, err := os.Open(name)
	if err != nil {
		return nil, "", fmt.Sprintf("Ошибка открытия файла: %v", err)
	}
	defer file.Close()

	ext := filepath.Ext(name)
	var img image.Image
	switch ext {
	case ".bmp":
		bmps, err := bmp.Decode(file)
		if err != nil {
			return nil, "", fmt.Sprintf("Ошибка декодирования файла: %v", err)
		}
		//return bmps, ext, ""
		img = bmps
	case ".png":
		pngs, err := png.Decode(file)
		if err != nil {
			return nil, "", fmt.Sprintf("Ошибка декодирования файла: %v", err)
		}
		//return pngs, ext, ""
		img = pngs
	default:
		return nil, "", fmt.Sprintf("Не верный тип файла используйте bmp/png")
	}

	// Проверка, является ли изображение в градациях серого (Gray)
	if grayImg, ok := img.(*image.Gray); ok {
		// Можно либо вернуть предупреждение, либо преобразовать в RGBA
		rgbaImg := ConvertGrayToRGBA(grayImg)
		return rgbaImg, ext, ""
	}

	return img, ext, ""
}

func ConvertGrayToRGBA(grayImg *image.Gray) *image.RGBA {
	bounds := grayImg.Bounds()
	rgbaImg := image.NewRGBA(bounds)

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			grayColor := grayImg.GrayAt(x, y)
			rgbaColor := color.RGBA{
				R: grayColor.Y,
				G: grayColor.Y,
				B: grayColor.Y,
				A: 255,
			}
			rgbaImg.Set(x, y, rgbaColor)
		}
	}

	return rgbaImg
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

func main() {
	myApp := app.New()
	myWindow := myApp.NewWindow("Lirprocs")

	var errorMy error
	var filePath string
	var fileName string
	var textFilePath string
	var filePath2 string
	var dirPath string
	var inputText string
	var directoryPath string
	var seed string
	var seed2 string
	var outputText string
	var aText string
	var aText2 string

	icon, _ := fyne.LoadResourceFromPath("Samples/ico.ico")
	myWindow.SetIcon(icon)

	// Вкладка 1
	errorLabel := widget.NewLabel("")
	//errorLabel.Wrapping = fyne.TextWrapWord
	errorLabel.TextStyle = fyne.TextStyle{Bold: true, Monospace: true, Italic: true}
	errorContainer := container.NewHScroll(errorLabel)

	fileInLabel := widget.NewLabel("")
	//fileInLabel.Wrapping = fyne.TextWrapWord
	fileInLabel.TextStyle = fyne.TextStyle{Bold: true, Monospace: true, Italic: true}
	fileInContainer := container.NewHScroll(fileInLabel)

	fileOutLabel := widget.NewLabel("")
	//fileOutLabel.Wrapping = fyne.TextWrapWord
	fileOutLabel.TextStyle = fyne.TextStyle{Bold: true, Monospace: true, Italic: true}
	fileOutContainer := container.NewHScroll(fileOutLabel)

	fileSelect := widget.NewButton("Выбрать файл", func() {
		filePath, errorMy = dialog.File().Load()
		if errorMy != nil {
			errorLabel.SetText("Не удалось получить файл")
			errorLabel.Refresh()
		}
		fileName = filepath.Base(filePath)
		//print("Имя файла:", fileName)
		fileInLabel.SetText(filePath)
		fileInLabel.Refresh()
	})

	textEntry := widget.NewMultiLineEntry()
	//textEntry.Wrapping
	textEntry.SetPlaceHolder("Введите текст")
	textEntry.OnChanged = func(text string) {
		inputText = text
	}

	aTextEntry := widget.NewMultiLineEntry()
	aTextEntry.SetPlaceHolder("Введите имитозащищаемые данные (Не обязательно)")
	aTextEntry.OnChanged = func(text string) {
		aText = text
	}

	scrollContainer := container.NewVScroll(textEntry)
	//scrollContainer.Resize(fyne.NewSize(200, 100))
	scrollContainer.SetMinSize(fyne.NewSize(200, 150))

	// Новая кнопка для загрузки текста из файла
	loadTextFromFile := widget.NewButton("Загрузить текст из файла", func() {
		textFilePath, errorMy = dialog.File().Filter("Text files", "txt").Load()
		if errorMy != nil {
			errorLabel.SetText("Не удалось загрузить файл")
			errorLabel.Refresh()
			return
		}

		file, err := os.Open(textFilePath)
		if err != nil {
			errorLabel.SetText("Ошибка открытия файла")
			errorLabel.Refresh()
			return
		}
		defer file.Close()

		var textFromFile strings.Builder
		scanner := bufio.NewScanner(file)
		bufSize := 50 * 1024 * 1024 // 50 МБ
		scanner.Buffer(make([]byte, bufSize), bufSize)

		for scanner.Scan() {
			textFromFile.WriteString(scanner.Text() + "\n")
		}

		if err = scanner.Err(); err != nil {
			errorLabel.SetText("Ошибка чтения файла")
			errorLabel.Refresh()
			return
		}

		inputText = textFromFile.String()
		errorLabel.SetText("Файл успешно загружен")
		errorLabel.Refresh()
	})

	textEntry1 := widget.NewEntry()
	textEntry1.SetPlaceHolder("Придумайте пароль")
	textEntry1.OnChanged = func(text string) {
		seed = text
	}

	dirSelect := widget.NewButton("Выбрать директорию сохранения", func() {
		directoryPath, errorMy = dialog.Directory().Title("Выберите директорию").Browse()
		if errorMy != nil {
			errorLabel.SetText("Не удалось получить директорию")
			errorLabel.Refresh()
		}
		fileOutLabel.SetText(directoryPath)
		fileOutLabel.Refresh()
		return
	})

	startButton := widget.NewButtonWithIcon("Старт", theme.MediaPlayIcon(), func() {
		//errorLabel.SetText("")
		if filePath == "" {
			errorLabel.SetText("Пожалуйста, выберите путь до изображения")
			errorLabel.Refresh()
			return
		} else if inputText == "" {
			errorLabel.SetText("Пожалуйста, введите кодируемый текст")
			errorLabel.Refresh()
			return
		} else if seed == "" {
			errorLabel.SetText("Пожалуйста, задайте пароль")
			errorLabel.Refresh()
			return
		} else if directoryPath == "" {
			errorLabel.SetText("Пожалуйста, выберете путь сохранения")
			errorLabel.Refresh()
			return
		}
		file, ext, err := GetFile(filePath)
		if err != "" {
			errorLabel.SetText(err)
			errorLabel.Refresh()
			return
		}

		//TODO cipher

		cipher := true
		cimg, err := encrypt.GetPosition(&wg, cipher, seed, inputText, aText, file)
		if err != "" {
			errorLabel.SetText(err)
			errorLabel.Refresh()
			return
		}
		err = SaveFile(cimg, filepath.Join(directoryPath, "enc_"+fileName), ext)
		if err != "" {
			errorLabel.SetText(err)
			errorLabel.Refresh()
			return
		}
		errorLabel.SetText("Готово")
		errorLabel.Refresh()
		return
	})

	tab1 := container.NewVBox(
		fileSelect,
		fileInContainer,
		scrollContainer,
		aTextEntry,
		loadTextFromFile,
		textEntry1,
		dirSelect,
		fileOutContainer,
		startButton,
		errorContainer,
	)

	// Вкладка 2
	fileInLabel2 := widget.NewLabel("")
	//fileInLabel2.Wrapping = fyne.TextWrapWord
	fileInLabel2.TextStyle = fyne.TextStyle{Bold: true, Monospace: true, Italic: true}
	fileInContainer2 := container.NewHScroll(fileInLabel2)

	textLabel2 := widget.NewLabel("")
	textLabel2.TextStyle = fyne.TextStyle{Bold: true, Monospace: true, Italic: true}

	errorLabel2 := widget.NewLabel("")
	//errorLabel2.Wrapping = fyne.TextWrapWord
	errorLabel2.TextStyle = fyne.TextStyle{Bold: true, Monospace: true, Italic: true}
	errorContainer2 := container.NewHScroll(errorLabel2)

	fileSelect2 := widget.NewButton("Выбрать файл", func() {
		filePath2, errorMy = dialog.File().Load()
		if errorMy != nil {
			errorLabel2.SetText("Не удалось получить файл")
			errorLabel2.Refresh()
		}
		fileInLabel2.SetText(filePath2)
		fileInLabel2.Refresh()
	})

	textEntry2 := widget.NewEntry()
	textEntry2.SetPlaceHolder("Введите пароль")
	textEntry2.OnChanged = func(text string) {
		seed2 = text
	}

	aTextEntry2 := widget.NewMultiLineEntry()
	aTextEntry2.SetPlaceHolder("Введите имитозащищаемые данные (Не обязательно)")
	aTextEntry2.OnChanged = func(text string) {
		aText2 = text
	}

	outputTextEntry := widget.NewLabel("")
	outputTextEntry.Wrapping = fyne.TextWrapWord
	scrollContainer1 := container.NewVScroll(outputTextEntry)
	//scrollContainer1.Resize(fyne.NewSize(500, 400))
	scrollContainer1.SetMinSize(fyne.NewSize(500, 230))

	scrollContainer.SetMinSize(fyne.NewSize(500, 190))

	startButton2 := widget.NewButtonWithIcon("Старт", theme.MediaPlayIcon(), func() {
		errorLabel.SetText("")
		if filePath2 == "" {
			errorLabel2.SetText("Пожалуйста, выберите файл")
			errorLabel2.Refresh()
			return
		} else if seed2 == "" {
			errorLabel2.SetText("Пожалуйста, введите пароль")
			errorLabel2.Refresh()
			return
		}
		file2, _, err := GetFile(filePath2)
		if err != "" {
			errorLabel2.SetText(err)
			errorLabel2.Refresh()
			return
		}

		//TODO aText2
		outputText = decrypt.GetPositionBack(&wg, file2, seed2, aText2)
		textEntry2.SetText("")
		if len(outputText) <= 1000000 {
			textLabel2.SetText("Полученный текст:")
			outputTextEntry.SetText(outputText)
			errorLabel2.SetText("Готово")
		} else {
			textLabel2.SetText("Текст слишком большой и был записан в файл" + dirPath)
			textLabel2.Refresh()
			dirPath = filepath.Dir(filePath2)
			err = ToFile(dirPath, outputText)
			errorLabel2.SetText("Готово")
			if err != "" {
				errorLabel2.SetText(err)
				errorLabel2.Refresh()
				return
			}
		}
	})

	saveToFile := widget.NewButton("Сохранить текст в файл", func() {
		if outputText == "" {
			if seed2 == "" {
				errorLabel2.SetText("Пожалуйста, введите пароль")
				errorLabel2.Refresh()
				return
			} else if filePath2 == "" {
				errorLabel2.SetText("Пожалуйста, выберите файл")
				errorLabel2.Refresh()
				return
			}
			file2, _, err := GetFile(filePath2)
			if err != "" {
				errorLabel2.SetText(err)
				errorLabel2.Refresh()
				return
			}

			//TODO
			outputText = decrypt.GetPositionBack(&wg, file2, seed2, aText2)
		}

		dirPath = filepath.Dir(filePath2)
		err := ToFile(dirPath, outputText)
		if err != "" {
			errorLabel2.SetText(err)
			errorLabel2.Refresh()
			return
		}
		errorLabel2.SetText("Текст был записан в файл в деррикторию: " + dirPath)
		errorLabel2.Refresh()
	})

	tab2 := container.NewVBox(
		fileSelect2,
		fileInContainer2,
		aTextEntry2,
		textEntry2,
		textLabel2,
		scrollContainer1,
		startButton2,
		saveToFile,
		errorContainer2,
	)

	tabs := container.NewAppTabs(
		container.NewTabItem("Шифрование", tab1),
		container.NewTabItem("Дешифрование", tab2),
	)

	myWindow.Resize(fyne.NewSize(600, 390))
	myWindow.SetFixedSize(true)
	myWindow.SetContent(tabs)
	myWindow.ShowAndRun()
}

//func main() {
//	var wg sync.WaitGroup
//	var mu sync.Mutex
//
//	in := "sample.bmp"
//	in := "samplePNG.png"
//	in := "sampleBMP.bmp"
//	out := "output"
//	out := "sample_output"
//	strok := "Hello my frend!"
//	strok = "Привет мой друг!"
//	strok := "Привет мой frend!"
//	strok := "In the bustling city, where lights never dim and life never slows, people weave through crowded streets, chasing dreams and evading shadows"
//	strok1 := "а б в г д е ё ж з и й к л м н о п р с т у ф х ц ч ш щ ъ ы ь э ю я А Б В Г Д Е Ё Ж З И Й К Л М Н О П Р С Т У Ф Х Ц Ч Ш Щ Ъ Ы Ь Э Ю Я , . ! ? - : ; ( ) ' [ ] { } < > / | _ @ # $ % ^ & * + ="
//	strok2 := "a b c d e f g h i j k l m n o p q r s t u v w x y z A B C D E F G H I J K L M N O P Q R S T U V W X Y Z , . ! ? - : ; ( ) ' [ ] { } < > /  | _ @ # $ % ^ & * + = "
//	strok := strok1 + strok2
//
//	seed := "8812332wkjwjw!@#$%"
//	in := "sampleBMP.bmp"
//
//	file, ext, err := GetFile(in)
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	//Поле для ввод с клавиатуры любого текста
//	strok := "Привет мой frend!"
//	//Поле для ввод с клавиатуры пароля
//	seed := "8812332wkjwjw!@#$%"
//
//	cimg, err := GetPosition(&wg, seed, strok, file)
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	//Поле для ввода пути сохранения файла
//	out := "output"
//
//	err = SaveFile(cimg, out, ext)
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	out1 := "output.bmp"
//
//	file2, ext, err := GetFile(out1)
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	//Поле для ввод с клавиатуры пароля
//	seed = "8812332wkjwjw!@#$%"
//	list := decrypt.GetPositionBack(&wg, file2, seed)
//
//	//Поле для вывода полученного текста
//	fmt.Println(list)
//}
