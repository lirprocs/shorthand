# Shorthand
## Описание проекта
Этот проект представляет собой программу для стеганографии текста в изображении. Программа использует язык программирования Go (Golang) и предоставляет возможность скрывать и извлекать текст в и из изображений в форматах BMP и PNG.
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
