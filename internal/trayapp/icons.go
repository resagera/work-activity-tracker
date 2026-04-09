package trayapp

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
)

func iconBytes(fill color.RGBA) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	bg := color.RGBA{0, 0, 0, 0}
	border := color.RGBA{40, 40, 40, 255}

	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			img.SetRGBA(x, y, bg)
			dx := x - 8
			dy := y - 8
			dist := dx*dx + dy*dy
			switch {
			case dist <= 25:
				img.SetRGBA(x, y, fill)
			case dist <= 36:
				img.SetRGBA(x, y, border)
			}
		}
	}

	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func activeIcon() []byte {
	return iconBytes(color.RGBA{46, 204, 113, 255})
}

func pausedIcon() []byte {
	return iconBytes(color.RGBA{241, 196, 15, 255})
}

func idleIcon() []byte {
	return iconBytes(color.RGBA{149, 165, 166, 255})
}

func errorIcon() []byte {
	return iconBytes(color.RGBA{231, 76, 60, 255})
}
