package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/sqweek/dialog"
	"path/filepath"
)

func main() {
	//var mu sync.Mutex
	myApp := app.New()
	myWindow := myApp.NewWindow("Lirprocs")

	// Создание переменных для путей файлов и текста
	var filePath string
	var fileName string
	var filePath2 string
	var inputText string
	var directoryPath string
	var seed string
	var seed2 string
	var outputText string

	icon, _ := fyne.LoadResourceFromPath("icon.png")
	myWindow.SetIcon(icon)

	// Вкладка 1
	errorLabel := widget.NewLabel("")
	errorLabel.TextStyle = fyne.TextStyle{Bold: true, Monospace: true, Italic: true}

	fileInLabel := widget.NewLabel("")
	fileInLabel.TextStyle = fyne.TextStyle{Bold: true, Monospace: true, Italic: true}

	fileOutLabel := widget.NewLabel("")
	fileOutLabel.TextStyle = fyne.TextStyle{Bold: true, Monospace: true, Italic: true}

	fileSelect := widget.NewButton("Выбрать файл", func() {
		filePath, _ = dialog.File().Load()
		fileName = filepath.Base(filePath)
		//print("Имя файла:", fileName)
		fileInLabel.SetText(filePath)
		fileInLabel.Refresh()
	})

	textEntry := widget.NewMultiLineEntry()
	textEntry.SetPlaceHolder("Введите текст")
	textEntry.OnChanged = func(text string) {
		inputText = text
	}

	scrollContainer := container.NewVScroll(textEntry)
	//scrollContainer.Resize(fyne.NewSize(200, 100))     // Установите необходимые размеры
	scrollContainer.SetMinSize(fyne.NewSize(200, 150)) // Установите минимальные размеры

	textEntry1 := widget.NewEntry()
	textEntry1.SetPlaceHolder("Придумайте пароль")
	textEntry1.OnChanged = func(text string) {
		seed = text
	}

	dirSelect := widget.NewButton("Выбрать директорию сохранения", func() {
		directoryPath, _ = dialog.Directory().Title("Выберите директорию").Browse()
		fileOutLabel.SetText(directoryPath)
		fileOutLabel.Refresh()
		return
	})

	startButton := widget.NewButtonWithIcon("Старт", theme.MediaPlayIcon(), func() {
		// Действия по нажатию кнопки "Старт"
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

		cimg, err := GetPosition(&wg, seed, inputText, file)
		if err != "" {
			errorLabel.SetText(err)
			errorLabel.Refresh()
			return
		}
		err = SaveFile(cimg, directoryPath+`\enc_`+fileName, ext)
		if err != "" {
			errorLabel.SetText(err)
			errorLabel.Refresh()
			return
		}
		errorLabel.SetText("Готово")
		errorLabel.Refresh()
		return
	})

	//tab1 := container.New(layout.NewFormLayout(), fileSelect, textEntry, dirSelect, startButton)
	tab1 := container.NewVBox(
		fileSelect,
		fileInLabel,
		scrollContainer,
		textEntry1,
		dirSelect,
		fileOutLabel,
		startButton,
		errorLabel,
	)

	// Вкладка 2
	fileInLabel2 := widget.NewLabel("")

	fileInLabel2.TextStyle = fyne.TextStyle{Bold: true, Monospace: true, Italic: true}

	textLabel2 := widget.NewLabel("")
	textLabel2.TextStyle = fyne.TextStyle{Bold: true, Monospace: true, Italic: true}

	errorLabel2 := widget.NewLabel("")
	errorLabel2.TextStyle = fyne.TextStyle{Bold: true, Monospace: true, Italic: true}

	fileSelect2 := widget.NewButton("Выбрать файл", func() {
		filePath2, _ = dialog.File().Load()
		fileInLabel2.SetText(filePath2)
		fileInLabel2.Refresh()
	})
	textEntry2 := widget.NewEntry()
	textEntry2.SetPlaceHolder("Введите пароль")
	textEntry2.OnChanged = func(text string) {
		seed2 = text
	}
	outputTextEntry := widget.NewLabel("")
	scrollContainer1 := container.NewVScroll(outputTextEntry)
	//scrollContainer1.Resize(fyne.NewSize(500, 400))
	scrollContainer1.SetMinSize(fyne.NewSize(500, 190)) // Установите минимальные размеры

	startButton2 := widget.NewButtonWithIcon("Старт", theme.MediaPlayIcon(), func() {
		// Действия по нажатию кнопки "Старт"
		// Сюда можно добавить вывод текста из файла в outputTextEntry
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
			errorLabel.SetText(err)
			errorLabel.Refresh()
			return
		}
		outputText = GetPositionBack(&wg, file2, seed2)
		textEntry2.SetText("")
		if len(outputText) <= 45 {
			textLabel2.SetText("Полученный текст:")
			outputTextEntry.SetText(outputText)
		} else {
			textLabel2.SetText("Текст слишком большой и был записан в файл")
			textLabel2.Refresh()
			dirPath := filepath.Dir(filePath2)
			err := ToFile(dirPath, outputText)
			if err != "" {
				errorLabel2.SetText(err)
				errorLabel2.Refresh()
				return
			}
		}
	})

	tab2 := container.NewVBox(
		fileSelect2,
		fileInLabel2,
		textEntry2,
		textLabel2,
		scrollContainer1,
		startButton2,
		errorLabel2,
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
