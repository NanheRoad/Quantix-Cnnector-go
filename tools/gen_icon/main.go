package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
)

func main() {
	if err := os.MkdirAll("build/windows", 0o755); err != nil {
		panic(err)
	}
	img := drawIcon(256)
	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, img); err != nil {
		panic(err)
	}
	if err := writeICO("build/windows/quantix.ico", pngBuf.Bytes()); err != nil {
		panic(err)
	}
	println(filepath.Clean("build/windows/quantix.ico"))
}

func drawIcon(size int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	bg := color.RGBA{15, 91, 216, 255}
	white := color.RGBA{255, 255, 255, 255}
	dark := color.RGBA{10, 63, 158, 255}

	fillRoundedRect(img, 10, 10, size-10, size-10, 52, bg)

	// Q ring
	cx, cy := float64(size)*0.39, float64(size)*0.42
	rOuter := float64(size) * 0.24
	rInner := float64(size) * 0.18
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx := float64(x) - cx
			dy := float64(y) - cy
			d2 := dx*dx + dy*dy
			if d2 <= rOuter*rOuter && d2 >= rInner*rInner {
				img.SetRGBA(x, y, white)
			}
		}
	}
	// Q tail
	drawLine(img, int(float64(size)*0.51), int(float64(size)*0.53), int(float64(size)*0.67), int(float64(size)*0.69), int(float64(size)*0.045), white)

	// Connector block
	fillRoundedRect(img, int(float64(size)*0.63), int(float64(size)*0.22), int(float64(size)*0.84), int(float64(size)*0.52), 18, white)
	fillRect(img, int(float64(size)*0.67), int(float64(size)*0.28), int(float64(size)*0.70), int(float64(size)*0.35), dark)
	fillRect(img, int(float64(size)*0.73), int(float64(size)*0.28), int(float64(size)*0.76), int(float64(size)*0.35), dark)
	fillRect(img, int(float64(size)*0.79), int(float64(size)*0.28), int(float64(size)*0.82), int(float64(size)*0.35), dark)

	// Weight baseline
	fillRoundedRect(img, int(float64(size)*0.21), int(float64(size)*0.74), int(float64(size)*0.57), int(float64(size)*0.79), 8, white)
	fillRoundedRect(img, int(float64(size)*0.28), int(float64(size)*0.83), int(float64(size)*0.49), int(float64(size)*0.87), 8, white)
	return img
}

func fillRect(img *image.RGBA, x0, y0, x1, y1 int, c color.RGBA) {
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			img.SetRGBA(x, y, c)
		}
	}
}

func fillRoundedRect(img *image.RGBA, x0, y0, x1, y1, r int, c color.RGBA) {
	rr := r * r
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			ok := true
			if x < x0+r && y < y0+r {
				dx, dy := x-(x0+r), y-(y0+r)
				ok = dx*dx+dy*dy <= rr
			}
			if x > x1-r-1 && y < y0+r {
				dx, dy := x-(x1-r-1), y-(y0+r)
				ok = dx*dx+dy*dy <= rr
			}
			if x < x0+r && y > y1-r-1 {
				dx, dy := x-(x0+r), y-(y1-r-1)
				ok = dx*dx+dy*dy <= rr
			}
			if x > x1-r-1 && y > y1-r-1 {
				dx, dy := x-(x1-r-1), y-(y1-r-1)
				ok = dx*dx+dy*dy <= rr
			}
			if ok {
				img.SetRGBA(x, y, c)
			}
		}
	}
}

func drawLine(img *image.RGBA, x0, y0, x1, y1, thickness int, c color.RGBA) {
	dx := x1 - x0
	dy := y1 - y0
	steps := dx
	if steps < 0 {
		steps = -steps
	}
	if ady := abs(dy); ady > steps {
		steps = ady
	}
	if steps == 0 {
		steps = 1
	}
	for i := 0; i <= steps; i++ {
		x := x0 + dx*i/steps
		y := y0 + dy*i/steps
		fillCircle(img, x, y, thickness/2, c)
	}
}

func fillCircle(img *image.RGBA, cx, cy, r int, c color.RGBA) {
	rr := r * r
	for y := cy - r; y <= cy+r; y++ {
		for x := cx - r; x <= cx+r; x++ {
			if x < 0 || y < 0 || x >= img.Bounds().Dx() || y >= img.Bounds().Dy() {
				continue
			}
			dx, dy := x-cx, y-cy
			if dx*dx+dy*dy <= rr {
				img.SetRGBA(x, y, c)
			}
		}
	}
}

func writeICO(path string, pngData []byte) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	// ICONDIR
	if err := binary.Write(f, binary.LittleEndian, uint16(0)); err != nil { // reserved
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint16(1)); err != nil { // type = icon
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint16(1)); err != nil { // count
		return err
	}

	// ICONDIRENTRY (single 256x256 PNG entry -> width/height = 0)
	entry := make([]byte, 16)
	entry[0] = 0 // width 256
	entry[1] = 0 // height 256
	entry[2] = 0 // palette
	entry[3] = 0 // reserved
	binary.LittleEndian.PutUint16(entry[4:], 1)                     // color planes
	binary.LittleEndian.PutUint16(entry[6:], 32)                    // bpp
	binary.LittleEndian.PutUint32(entry[8:], uint32(len(pngData)))  // bytes in res
	binary.LittleEndian.PutUint32(entry[12:], uint32(6+16))         // offset
	if _, err := f.Write(entry); err != nil {
		return err
	}
	_, err = f.Write(pngData)
	return err
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

