package stego

import (
	"github.com/lirprocs/shorthand/internal/crypto/kdf"
	"github.com/lirprocs/shorthand/internal/crypto/suite"
)

// Чтение заголовков прежних версий. Запись в этом виде больше не выполняется;
// эталонные изображения — internal/stego/testdata (v3_*.png, v4_*.png).
//
// ─── Версия 4 (ветка v3.1, master до ветки v3.2) ────────────────────────────
//
// Поля и маска — как в версии 5 (format.go), отличаются способы записи и код
// сообщения:
//
//	уровень        код заголовка  запись заголовка  код сообщения  запись сообщения
//	обычная        RS(42, 26)     STC h = 10, w = 8  RS 32          STC h = 10, w ≤ 64
//	повышенная     RS(90, 26)     h = 1, w = 4       RS 64          h = 1, w ≤ 4
//	максимальная   RS(90, 26)     h = 1, w = 1       RS 128         h = 1, w = 1
//
// Почему заменён (docs/EXPERIMENTS.md, раздел 7): на «обычной» код RS 32 не
// исправлял почти ничего, а вместе с узким заголовком давал до половины
// изменений на коротком сообщении; на «повышенной» чётность группы из четырёх
// элементов требовала в полтора раза больше изменений, чем код Хэмминга при
// той же устойчивости.
//
// ─── Версия 3 (ветка v3 до коммита 8e03055) ─────────────────────────────────
//
// Заголовок (26 байт):
//
//	0–1   «SH»
//	2     версия = 3
//	3     режим, способ сжатия, уровень — как в версиях 4 и 5
//	4     h
//	5     w
//	6–9   L
//	10–25 nonce (для «Магмы» — 8 байт и нули; старший бит обнулён)
//
// Весь заголовок складывался с маской, зависящей только от пароля и режима, и
// записывался одним из двух способов:
//
//   - компактный («обычная»): RS(42, 26), STC с h = 10, w = 8;
//   - устойчивый («повышенная», «максимальная»): RS(90, 26), каждый из 720 бит —
//     прямым ±1 в трёх элементах, при чтении — большинство из трёх.
//
// Почему заменён: постоянные поля под постоянной маской дают одинаковые
// младшие биты в одних и тех же элементах у всех изображений одного размера с
// одним паролем (225 элементов на устойчивом заголовке), а тройной повтор —
// пары элементов с всегда равными младшими битами и около 1100 изменений без
// учёта стоимости. Код сообщения — как в версии 4.

const maxWOld = 64 // наибольшая ширина STC в версиях 3 и 4

var (
	styleNormalV4 = headerStyle{version: 4, parity: 16, h: 10, w: 8}
	styleHighV4   = headerStyle{version: 4, parity: 64, h: 1, w: 4}
	styleMaxV4    = headerStyle{version: 4, parity: 64, h: 1, w: 1}

	styleCompactV3 = headerStyle{version: 3, parity: 16, h: 10, w: 8}
	styleRobustV3  = headerStyle{version: 3, parity: 64, repeat: 3}
)

func styleForV4(l Level) headerStyle {
	switch l {
	case LevelNormal:
		return styleNormalV4
	case LevelHigh:
		return styleHighV4
	}
	return styleMaxV4
}

func styleForV3(l Level) headerStyle {
	if l == LevelNormal {
		return styleCompactV3
	}
	return styleRobustV3
}

// headerMaskV3 — маска заголовка версии 3: поток генератора режима.
func (k keys) headerMaskV3(m suite.Mode) []byte {
	mask := make([]byte, styleRobustV3.coded())
	m.NewRand(kdf.Derive(k.master, nil, infoHeader(m), 32)).Read(mask)
	return mask
}

// parseHeaderV3 разбирает 26 байт заголовка версии 3 (маска уже снята).
func parseHeaderV3(b []byte) (*Header, error) {
	if len(b) != headerLen || b[0] != magic[0] || b[1] != magic[1] || b[2] != 3 {
		return nil, errBadHeader
	}
	h := &Header{Version: 3}
	copy(h.Nonce[:], b[10:26])
	return h, h.setFields(b[3], b[4], b[5], b[6:10])
}
