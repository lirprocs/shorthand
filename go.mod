module github.com/lirprocs/shorthand

go 1.26.0

// До слияния PR в lirprocs/MGM — локальный клон с исправлениями
// (ветка fix-counters-length). После слияния строку replace убрать
// и обновить версию: go get github.com/lirprocs/MGM@master
replace github.com/lirprocs/MGM => ../MGM

require (
	github.com/lirprocs/Kuznyechik v0.0.0-20250322221820-7703e70b67ba
	github.com/lirprocs/MGM v0.0.0-00010101000000-000000000000
)

require (
	golang.org/x/crypto v0.58.0
	golang.org/x/sys v0.49.0 // indirect
)
