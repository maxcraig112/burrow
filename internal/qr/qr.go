package qr

import (
	"io"

	qrt "github.com/mdp/qrterminal/v3"
)

// Print writes a terminal QR code for url to w.
// GenerateWithConfig is used instead of Generate to avoid the sixel-support
// probe (IsSixelSupported) which panics on Windows when stdout is not a TTY.
func Print(w io.Writer, url string) {
	qrt.GenerateWithConfig(url, qrt.Config{
		Level:          qrt.M,
		Writer:         w,
		BlackChar:      qrt.BLACK,
		WhiteChar:      qrt.WHITE,
		BlackWhiteChar: qrt.BLACK_WHITE,
		WhiteBlackChar: qrt.WHITE_BLACK,
		QuietZone:      1,
	})
}
