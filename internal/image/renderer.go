package img

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "image/gif"
	_ "image/jpeg"

	"github.com/mattn/go-sixel"
	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const (
	maxBody   = 256 << 10
	userAgent = "Mozilla/5.0 (compatible; sit/1.0; +https://github.com/Panonim/search-in-terminal)"
)

type Renderer struct {
	proto        Protocol
	cacheDir     string
	cells        int
	enabled      bool
	cellW, cellH int
	client       *http.Client

	mu   sync.Mutex
	seqs map[string]string
	bad  map[string]bool
}

func NewRenderer(p Protocol, cacheDir string, cells int, enabled bool) *Renderer {
	if cells < 1 {
		cells = 2
	}
	w, h := cellPx()
	return &Renderer{
		proto:    p,
		cacheDir: cacheDir,
		cells:    cells,
		enabled:  enabled && p != ProtocolNone,
		cellW:    w,
		cellH:    h,
		client:   &http.Client{Timeout: 8 * time.Second},
		seqs:     make(map[string]string),
		bad:      make(map[string]bool),
	}
}

func (r *Renderer) Protocol() Protocol { return r.proto }
func (r *Renderer) Enabled() bool      { return r.enabled }
func (r *Renderer) Cells() int         { return r.cells }

func cellPx() (int, int) {
	if v := os.Getenv("SIT_CELL_PX"); v != "" {
		if ws, hs, ok := strings.Cut(strings.ToLower(v), "x"); ok {
			w, err1 := strconv.Atoi(ws)
			h, err2 := strconv.Atoi(hs)
			if err1 == nil && err2 == nil && w > 0 && h > 0 && w <= 200 && h <= 200 {
				return w, h
			}
		}
	}
	return 10, 20
}

func (r *Renderer) Fetch(ctx context.Context, pageURL, faviconURL string) error {
	if !r.enabled {
		return nil
	}
	host := hostOf(pageURL)
	if host == "" {
		return errors.New("img: no host in url")
	}

	r.mu.Lock()
	_, have := r.seqs[host]
	bad := r.bad[host]
	r.mu.Unlock()
	if have {
		return nil
	}
	if bad {
		return errors.New("img: no icon for " + host)
	}

	if data, err := os.ReadFile(r.cachePath(host)); err == nil {
		if m, err := png.Decode(bytes.NewReader(data)); err == nil {
			return r.store(host, m)
		}
	}

	var last error
	for _, c := range candidates(host, faviconURL) {
		m, err := r.download(ctx, c)
		if err != nil {
			last = err
			continue
		}
		scaled := scale(m, r.cells*r.cellW, r.cellH)
		if err := r.save(host, scaled); err != nil {
			return err
		}
		return r.store(host, scaled)
	}

	r.mu.Lock()
	r.bad[host] = true
	r.mu.Unlock()
	if last == nil {
		last = errors.New("img: no favicon candidates")
	}
	return last
}

func (r *Renderer) Cell(pageURL string) (string, bool) {
	if !r.enabled {
		return "", false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	seq, ok := r.seqs[hostOf(pageURL)]
	return seq, ok
}

func candidates(host, faviconURL string) []string {
	var out []string
	if faviconURL != "" {
		out = append(out, faviconURL)
	}
	return append(out,
		"https://"+host+"/favicon.ico",
		"https://icons.duckduckgo.com/ip3/"+host+".ico",
		"https://www.google.com/s2/favicons?domain="+host+"&sz=32",
	)
}

func (r *Renderer) download(ctx context.Context, rawURL string) (image.Image, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "image/*")

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("img: %s: %s", rawURL, resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, err
	}
	if isICO(body) {
		return decodeICO(body)
	}
	m, _, err := image.Decode(bytes.NewReader(body))
	return m, err
}

// scale fits src inside w x h with its aspect ratio kept, centred on transparency.
func scale(src image.Image, w, h int) image.Image {
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	b := src.Bounds()
	if b.Dx() <= 0 || b.Dy() <= 0 {
		return dst
	}
	tw, th := w, b.Dy()*w/b.Dx()
	if th > h {
		tw, th = b.Dx()*h/b.Dy(), h
	}
	if tw < 1 {
		tw = 1
	}
	if th < 1 {
		th = 1
	}
	off := image.Rect((w-tw)/2, (h-th)/2, (w-tw)/2+tw, (h-th)/2+th)
	xdraw.CatmullRom.Scale(dst, off, src, b, xdraw.Over, nil)
	return dst
}

func (r *Renderer) cachePath(host string) string {
	sum := sha256.Sum256([]byte(host))
	return filepath.Join(r.cacheDir, hex.EncodeToString(sum[:])[:16]+".png")
}

func (r *Renderer) save(host string, m image.Image) error {
	if r.cacheDir == "" {
		return nil
	}
	if err := os.MkdirAll(r.cacheDir, 0o755); err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, m); err != nil {
		return err
	}
	tmp := r.cachePath(host) + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, r.cachePath(host))
}

func (r *Renderer) store(host string, m image.Image) error {
	seq, err := r.encode(host, m)
	if err != nil {
		return err
	}
	r.mu.Lock()
	r.seqs[host] = seq
	r.mu.Unlock()
	return nil
}

func (r *Renderer) encode(host string, m image.Image) (string, error) {
	switch r.proto {
	case ProtocolKitty:
		return r.encodeKitty(host, m)
	case ProtocolITerm:
		return r.encodeITerm(m)
	case ProtocolSixel:
		return r.encodeSixel(m)
	}
	return "", errors.New("img: no graphics protocol")
}

func (r *Renderer) encodeKitty(host string, m image.Image) (string, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, m); err != nil {
		return "", err
	}
	payload := base64.StdEncoding.EncodeToString(buf.Bytes())

	sum := sha256.Sum256([]byte(host))
	id := binary.BigEndian.Uint32(sum[:4])%4_000_000_000 + 1

	var out strings.Builder
	const chunk = 4096
	first := true
	for len(payload) > 0 {
		n := min(chunk, len(payload))
		more := 0
		if n < len(payload) {
			more = 1
		}
		if first {
			// q=2 silences the terminal's reply, which would otherwise land in Bubble Tea's input;
			// a fixed placement id keeps redraws from stacking placements.
			fmt.Fprintf(&out, "\x1b_Ga=T,f=100,t=d,i=%d,p=1,q=2,c=%d,r=1,C=1,m=%d;%s\x1b\\", id, r.cells, more, payload[:n])
			first = false
		} else {
			fmt.Fprintf(&out, "\x1b_Gm=%d;%s\x1b\\", more, payload[:n])
		}
		payload = payload[n:]
	}
	// C=1 keeps the cursor put, so the spaces do the whole advance.
	return passthrough(out.String()) + strings.Repeat(" ", r.cells), nil
}

func (r *Renderer) encodeITerm(m image.Image) (string, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, m); err != nil {
		return "", err
	}
	seq := fmt.Sprintf("\x1b]1337;File=inline=1;width=%d;height=1;preserveAspectRatio=1:%s\a",
		r.cells, base64.StdEncoding.EncodeToString(buf.Bytes()))
	return passthrough(seq), nil
}

func (r *Renderer) encodeSixel(m image.Image) (string, error) {
	var buf bytes.Buffer
	enc := sixel.NewEncoder(&buf)
	enc.Transparent = true
	if err := enc.Encode(m); err != nil {
		return "", err
	}
	// Sixel cursor handling differs per terminal, so save and restore it and advance by hand.
	return passthrough("\x1b7"+buf.String()+"\x1b8") + strings.Repeat(" ", r.cells), nil
}

// passthrough wraps escape sequences for tmux, which otherwise swallows them.
func passthrough(seq string) string {
	if os.Getenv("TMUX") == "" {
		return seq
	}
	return "\x1bPtmux;" + strings.ReplaceAll(seq, "\x1b", "\x1b\x1b") + "\x1b\\"
}
