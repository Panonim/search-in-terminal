package img

import (
	"image"
	"image/color"
	"image/draw"
	"testing"
)

func solid(c color.Color) image.Image {
	m := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	draw.Draw(m, m.Bounds(), &image.Uniform{c}, image.Point{}, draw.Src)
	return m
}

func TestBackdropOnlyWhenIconBlends(t *testing.T) {
	black, white, orange := solid(color.Black), solid(color.White), solid(color.NRGBA{0xff, 0x45, 0x00, 0xff})
	cases := []struct {
		name string
		icon image.Image
		b    Backdrop
		want bool
	}{
		{"black on dark", black, BackdropLight, true},
		{"white on dark", white, BackdropLight, false},
		{"white on light", white, BackdropDark, true},
		{"black on light", black, BackdropDark, false},
		{"orange on dark", orange, BackdropLight, false},
		{"orange on light", orange, BackdropDark, false},
		{"off", black, BackdropOff, false},
	}
	for _, c := range cases {
		out := withBackdrop(c.icon, 20, 20, c.b)
		// The corner lies outside the circle, so only the scaled icon can colour it.
		corner := color.NRGBAModel.Convert(out.At(0, 0)).(color.NRGBA)
		rim := color.NRGBAModel.Convert(out.At(10, 1)).(color.NRGBA)
		got := corner.A == 0 && rim == backdropColors[c.b]
		if got != c.want {
			t.Errorf("%s: backdrop = %v, want %v (corner %v, rim %v)", c.name, got, c.want, corner, rim)
		}
	}
}

func TestFlattenBlendsEdgesOverBackground(t *testing.T) {
	m := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	m.SetNRGBA(0, 0, color.NRGBA{0xff, 0xff, 0xff, 0x80})
	out := flatten(m, color.NRGBA{0x00, 0x00, 0x00, 0xff})
	edge := color.NRGBAModel.Convert(out.At(0, 0)).(color.NRGBA)
	if edge.A != 0xff || edge.R < 0x7e || edge.R > 0x81 {
		t.Errorf("half white over black = %v, want opaque mid grey", edge)
	}
	if _, _, _, a := out.At(1, 0).RGBA(); a != 0 {
		t.Error("fully transparent pixels must stay transparent")
	}
}

func TestBackdropIgnoresTransparentIcon(t *testing.T) {
	if blends(solid(color.Transparent), BackdropLight) {
		t.Error("a fully transparent icon should not get a backdrop")
	}
}
