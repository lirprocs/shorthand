package stego

import (
	"github.com/lirprocs/shorthand/internal/crypto/kdf"
	"github.com/lirprocs/shorthand/internal/crypto/suite"
)

// Чтение заголовка версии 3 (изображения, записанные веткой v3 до коммита
// 8e03055). Запись в этом виде больше не выполняется.
//
// Заголовок версии 3 (26 байт):
//
//	0–1   «SH»
//	2     версия = 3
//	3     режим, способ сжатия, уровень — как в версии 4
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
// учёта стоимости.

var (
	styleCompactV3 = headerStyle{version: 3, parity: 16, h: 10, w: 8}
	styleRobustV3  = headerStyle{version: 3, parity: 64, repeat: 3}
)

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
