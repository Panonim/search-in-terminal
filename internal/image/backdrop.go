package img

import (
	"image"
	"image/color"
	"image/draw"
	"math"
)

// Backdrop is the circle drawn behind icons that would blend into the terminal background.
type Backdrop int

const (
	BackdropOff   Backdrop = iota
	BackdropLight          // for dark terminals
	BackdropDark           // for light terminals
)

var backdropColors = map[Backdrop]color.NRGBA{
	BackdropLight: {0xd9, 0xd9, 0xd9, 0xff},
	BackdropDark:  {0x33, 0x33, 0x33, 0xff},
}

// withBackdrop fits src into w x h, putting it on a circle when it is too close to the terminal background.
func withBackdrop(src image.Image, w, h int, b Backdrop) image.Image {
	d := min(w, h)
	if b == BackdropOff || !blends(scale(src, d, d), b) {
		return scale(src, w, h)
	}
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	cx, cy, radius := float64(w)/2, float64(h)/2, float64(d)/2
	c := backdropColors[b]
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dist := math.Hypot(float64(x)+0.5-cx, float64(y)+0.5-cy)
			// Partial coverage at the edge anti-aliases the outline.
			cover := math.Max(0, math.Min(1, radius-dist+0.5))
			dst.SetNRGBA(x, y, color.NRGBA{c.R, c.G, c.B, uint8(cover * 255)})
		}
	}
	// Slightly smaller than the inscribed square (d/√2) so a rim stays visible.
	side := int(float64(d) * 0.68)
	icon := scale(src, side, side)
	at := image.Pt((w-side)/2, (h-side)/2)
	draw.Draw(dst, icon.Bounds().Add(at), icon, image.Point{}, draw.Over)
	return dst
}

func backdropFor(bg color.NRGBA, on bool) Backdrop {
	if !on {
		return BackdropOff
	}
	if luma(bg) < 0.5 {
		return BackdropLight
	}
	return BackdropDark
}

func luma(c color.NRGBA) float64 {
	return (0.2126*float64(c.R) + 0.7152*float64(c.G) + 0.0722*float64(c.B)) / 255
}

// blends reports whether the icon's average visible colour is too dark (or light) for the terminal.
func blends(m image.Image, b Backdrop) bool {
	var sum, weight float64
	bounds := m.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			c := color.NRGBAModel.Convert(m.At(x, y)).(color.NRGBA)
			a := float64(c.A) / 255
			sum += a * luma(c)
			weight += a
		}
	}
	if weight < 1 {
		return false
	}
	lum := sum / weight
	if b == BackdropLight {
		return lum < 0.3
	}
	return lum > 0.7
}

// flatten blends partly transparent pixels over bg, since sixel can only show a pixel fully or not at all.
func flatten(m image.Image, bg color.NRGBA) image.Image {
	b := m.Bounds()
	out := image.NewNRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := color.NRGBAModel.Convert(m.At(x, y)).(color.NRGBA)
			if c.A == 0 {
				continue
			}
			a := uint32(c.A)
			mix := func(fg, bg uint8) uint8 { return uint8((uint32(fg)*a + uint32(bg)*(255-a)) / 255) }
			out.SetNRGBA(x, y, color.NRGBA{mix(c.R, bg.R), mix(c.G, bg.G), mix(c.B, bg.B), 0xff})
		}
	}
	return out
}
