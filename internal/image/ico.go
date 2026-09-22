package img

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/png"
)

var pngMagic = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}

func isICO(b []byte) bool {
	return len(b) >= 4 && b[0] == 0 && b[1] == 0 && (b[2] == 1 || b[2] == 2) && b[3] == 0
}

// decodeICO picks the largest entry of an ICO/CUR file and decodes it.
func decodeICO(b []byte) (image.Image, error) {
	if len(b) < 6 || !isICO(b) {
		return nil, errors.New("ico: bad header")
	}
	count := int(binary.LittleEndian.Uint16(b[4:6]))
	if count == 0 || len(b) < 6+count*16 {
		return nil, errors.New("ico: bad directory")
	}

	best, bestArea := -1, -1
	for i := 0; i < count; i++ {
		e := b[6+i*16 : 6+i*16+16]
		w, h := int(e[0]), int(e[1])
		if w == 0 {
			w = 256
		}
		if h == 0 {
			h = 256
		}
		if w*h > bestArea {
			best, bestArea = i, w*h
		}
	}
	e := b[6+best*16 : 6+best*16+16]
	size := int(binary.LittleEndian.Uint32(e[8:12]))
	off := int(binary.LittleEndian.Uint32(e[12:16]))
	if off < 0 || size <= 0 || off+size > len(b) {
		return nil, errors.New("ico: entry out of range")
	}
	payload := b[off : off+size]
	if bytes.HasPrefix(payload, pngMagic) {
		return png.Decode(bytes.NewReader(payload))
	}
	return decodeDIB(payload)
}

// decodeDIB reads the 24/32-bit uncompressed BMP variant embedded in an ICO.
func decodeDIB(b []byte) (image.Image, error) {
	if len(b) < 40 || binary.LittleEndian.Uint32(b[0:4]) < 40 {
		return nil, errors.New("ico: unsupported dib header")
	}
	w := int(int32(binary.LittleEndian.Uint32(b[4:8])))
	h := int(int32(binary.LittleEndian.Uint32(b[8:12]))) / 2
	bits := int(binary.LittleEndian.Uint16(b[14:16]))
	if binary.LittleEndian.Uint32(b[16:20]) != 0 {
		return nil, errors.New("ico: compressed dib")
	}
	if w <= 0 || h <= 0 || w > 1024 || h > 1024 {
		return nil, errors.New("ico: bad dib size")
	}
	if bits != 32 && bits != 24 {
		return nil, errors.New("ico: unsupported bit depth")
	}

	hdr := int(binary.LittleEndian.Uint32(b[0:4]))
	stride := (w*bits/8 + 3) &^ 3
	if len(b) < hdr+stride*h {
		return nil, errors.New("ico: truncated pixels")
	}
	pix := b[hdr:]

	maskStride := (w + 31) / 32 * 4
	mask := []byte(nil)
	if len(pix) >= stride*h+maskStride*h {
		mask = pix[stride*h:]
	}

	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	opaque := false
	for y := 0; y < h; y++ {
		row := pix[(h-1-y)*stride:]
		for x := 0; x < w; x++ {
			p := row[x*bits/8:]
			a := uint8(0xff)
			if bits == 32 {
				a = p[3]
				if a != 0 {
					opaque = true
				}
			} else if mask != nil {
				bit := mask[(h-1-y)*maskStride+x/8] >> (7 - uint(x)%8) & 1
				if bit == 1 {
					a = 0
				}
			}
			dst.SetNRGBA(x, y, color.NRGBA{R: p[2], G: p[1], B: p[0], A: a})
		}
	}
	if bits == 32 && !opaque {
		for i := 3; i < len(dst.Pix); i += 4 {
			dst.Pix[i] = 0xff
		}
	}
	return dst, nil
}
