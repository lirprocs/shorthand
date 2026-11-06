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
	fyne.io/fyne/v2 v2.4.3
	github.com/sqweek/dialog v0.0.0-20220809060634-e981b270ebbf
	golang.org/x/image v0.47.0
)

require (
	fyne.io/systray v1.10.1-0.20231115130155-104f5ef7839e // indirect
	github.com/TheTitanrain/w32 v0.0.0-20180517000239-4f5cfb03fabf // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/fredbi/uri v1.0.0 // indirect
	github.com/fsnotify/fsnotify v1.6.0 // indirect
	github.com/fyne-io/gl-js v0.0.0-20220119005834-d2da28d9ccfe // indirect
	github.com/fyne-io/glfw-js v0.0.0-20220120001248-ee7290d23504 // indirect
	github.com/fyne-io/image v0.0.0-20220602074514-4956b0afb3d2 // indirect
	github.com/go-gl/gl v0.0.0-20211210172815-726fda9656d6 // indirect
	github.com/go-gl/glfw/v3.3/glfw v0.0.0-20221017161538-93cebf72946b // indirect
	github.com/go-text/render v0.0.0-20230619120952-35bccb6164b8 // indirect
	github.com/go-text/typesetting v0.0.0-20230616162802-9c17dd34aa4a // indirect
	github.com/godbus/dbus/v5 v5.1.0 // indirect
	github.com/gopherjs/gopherjs v1.17.2 // indirect
	github.com/jsummers/gobmp v0.0.0-20151104160322-e2ba15ffa76e // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/srwiley/oksvg v0.0.0-20221011165216-be6e8873101c // indirect
	github.com/srwiley/rasterx v0.0.0-20220730225603-2ab79fcdd4ef // indirect
	github.com/stretchr/testify v1.8.4 // indirect
	github.com/tevino/abool v1.2.0 // indirect
	github.com/yuin/goldmark v1.5.5 // indirect
	golang.org/x/mobile v0.0.0-20230531173138-3c911d8e3eda // indirect
	golang.org/x/net v0.60.0 // indirect
	golang.org/x/text v0.43.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	honnef.co/go/js/dom v0.0.0-20210725211120-f030747120f2 // indirect
)

require (
	golang.org/x/crypto v0.58.0
	golang.org/x/sys v0.49.0 // indirect
)
