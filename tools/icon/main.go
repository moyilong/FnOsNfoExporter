package main

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
)

func main() {
	base := os.Args[1]
	for _, size := range []int{64, 80, 256} {
		im := image.NewRGBA(image.Rect(0, 0, size, size))
		draw.Draw(im, im.Bounds(), &image.Uniform{color.RGBA{36, 95, 229, 255}}, image.Point{}, draw.Src)
		rect := image.Rect(size/4, size/6, 3*size/4, 5*size/6)
		draw.Draw(im, rect, &image.Uniform{color.White}, image.Point{}, draw.Src)
		for _, y := range []int{size / 3, size / 2, 2 * size / 3} {
			draw.Draw(im, image.Rect(size/3, y, 2*size/3, y+size/24), &image.Uniform{color.RGBA{36, 95, 229, 255}}, image.Point{}, draw.Src)
		}
		names := []string{filepath.Join(base, "app/ui/images", name(size))}
		if size == 80 {
			names = append(names, filepath.Join(base, "ICON.PNG"))
		}
		if size == 256 {
			names = append(names, filepath.Join(base, "ICON_256.PNG"))
		}
		for _, n := range names {
			os.MkdirAll(filepath.Dir(n), 0755)
			f, e := os.Create(n)
			if e != nil {
				panic(e)
			}
			if e = png.Encode(f, im); e != nil {
				panic(e)
			}
			f.Close()
		}
	}
}
func name(s int) string {
	switch s {
	case 64:
		return "icon_64.png"
	case 80:
		return "icon_80.png"
	default:
		return "icon_256.png"
	}
}
