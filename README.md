# Shorthand
## Описание проекта
Этот проект представляет собой программу для стеганографии текста в изображении. Программа использует язык программирования Go (Golang) и предоставляет возможность скрывать и извлекать текст в и из изображений в форматах BMP и PNG.
## Описание файлов
### program.go
program.go содержит логику работы приложени и всё используемые функции:  

1)**uintToBinary:** Преобразует число типа uint в строку, представляющую его двоичное представление.  
2)**GetSeed:** Генерирует "зерно" (seed) на основе введенной строки.  
3)**GetFile:** Открывает файл изображения и возвращает изображение, его расширение и ошибку (если есть).  
4)**SaveFile:** Сохраняет изображение в файл и возвращает ошибку (если есть).  
5)**SetInfo:** Записывает информацию о длине текста и его типе в изображение.  
6)**GetInfo:** Получает информацию о длине текста и его типе из изображения.  
7)**GetPosition:** Шифрует текст в изображении на основе введенных данных.  
8)**StringToBin:** Преобразует строку в бинарный формат.  
9)**ChangeIMG:** Изменяет изображение, внедряя биты текста в изображение.  
10)**GetPositionBack:** Дешифрует текст из изображения.  
11)**GetBin:** Получает бинарные данные из изображения.  
12)**GetText:** Преобразует бинарные данные в текст.  
13)**ToFile:** Сохраняет текст в файл и открывает его.  
### main.go
main.go создает графический интерфейс пользователя (GUI) с использованием библиотеки Fyne.
### Sample
Папка Sample содержит изображения которые вы можете использовать
### Examples
Папка Examples содержит изображения с закодированным в них тексто имя файла содержит пароль к нему и количество символов в зашифрованном тексте
## Способы запуска
### Со сборкой GUI приложения.
1) Скопировать проект 
```bash
git clone [https://github.com/your_username/steganography-tool.git](https://github.com/lirprocs/shorthand.git)
```
2) Собрать fyne прект
```bash
fyne package -os windows -icon icon.png
```
3) Запустить полученый файл shorthand
### Без GUI приложения
1) Скопировать проект 
```bash
git clone [https://github.com/your_username/steganography-tool.git](https://github.com/lirprocs/shorthand.git)
```
2) Закоментировать main.go и разкоментировать функцию main в program.go
3) Указать необходимые параметры в функции main
4) Запустить приложение
```bash
go run program.go
```
5) Полученный результат будет выведен в консоль
