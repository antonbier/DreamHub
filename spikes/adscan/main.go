// Spike: logo detection for AdScan (docs/ADSCAN_PLAN.md, #44).
//
// Questions: can a channel's logo be learned from sparse keyframes alone,
// does a mask score separate "logo" from "no logo", and does seeking by
// feeding FFmpeg a short slice of the .ts over stdin work?
//
//	adscan learn <rec.ts>             learn a mask from all keyframes, print it
//	adscan scan  <rec.ts> <mask.json> score every keyframe against a mask
//	adscan seek  <rec.ts> <mask.json> score one keyframe per byte slice
//
// FFmpeg comes from $FFMPEG (default "ffmpeg").
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"time"
)

// Every frame is scaled to one fixed size before anything else, so masks
// are comparable between SD and HD and the pixels per frame stay small.
const (
	fw, fh = 384, 216
	// A pixel counts as an edge above this Sobel magnitude.
	edgeThreshold = 48
	// A pixel belongs to the logo if it is an edge in at least this share
	// of the sampled frames.
	stableShare = 0.8
)

// The four corner regions searched for a logo, in frame pixels.
var corners = []struct {
	name         string
	x0, y0, w, h int
}{
	{"top-left", 0, 0, fw / 4, fh / 4},
	{"top-right", fw - fw/4, 0, fw / 4, fh / 4},
	{"bottom-left", 0, fh - fh/4, fw / 4, fh / 4},
	{"bottom-right", fw - fw/4, fh - fh/4, fw / 4, fh / 4},
}

type frame struct {
	pts float64 // seconds
	pix []byte  // fw*fh grayscale
}

type mask struct {
	Corner string  `json:"corner"`
	X0     int     `json:"x0"`
	Y0     int     `json:"y0"`
	W      int     `json:"w"`
	H      int     `json:"h"`
	Dirs   []int8  `json:"dirs"` // W*H: the logo edge's direction (0-7), -1 = not logo
	Count  int     `json:"count"`
	Frames int     `json:"frames"`
	Share  float64 `json:"share"`
}

func ffmpegPath() string {
	if p := os.Getenv("FFMPEG"); p != "" {
		return p
	}
	return "ffmpeg"
}

var ptsRe = regexp.MustCompile(`pts_time:\s*(-?[0-9.]+)`)

// keyframes decodes the keyframes of src (a path, or "-" for stdin with
// in as the data) and returns at most max of them (0 = all).
func keyframes(src string, in io.Reader, max int) ([]frame, error) {
	args := []string{"-hide_banner", "-loglevel", "info"}
	if src != "-" {
		args = append(args, "-nostdin")
	}
	// -copyts keeps the stream's own timestamps, so a slice read from the
	// middle of a recording reports where it is, not 0.
	args = append(args, "-skip_frame", "nokey", "-copyts", "-i", src, "-map", "0:v:0", "-an", "-sn",
		"-vf", fmt.Sprintf("scale=%d:%d,format=gray,showinfo", fw, fh),
		"-fps_mode", "passthrough")
	if max > 0 {
		args = append(args, "-frames:v", strconv.Itoa(max))
	}
	args = append(args, "-f", "rawvideo", "-")
	cmd := exec.Command(ffmpegPath(), args...)
	cmd.Stdin = in
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	var frames []frame
	r := bufio.NewReaderSize(out, fw*fh)
	for {
		pix := make([]byte, fw*fh)
		if _, err := io.ReadFull(r, pix); err != nil {
			break
		}
		frames = append(frames, frame{pix: pix})
	}
	werr := cmd.Wait()
	pts := ptsRe.FindAllStringSubmatch(stderr.String(), -1)
	for i := range frames {
		if i < len(pts) {
			frames[i].pts, _ = strconv.ParseFloat(pts[i][1], 64)
		}
	}
	if len(frames) == 0 && werr != nil {
		return nil, fmt.Errorf("ffmpeg: %v: %s", werr, tail(stderr.String()))
	}
	return frames, nil
}

func tail(s string) string {
	if len(s) > 400 {
		return s[len(s)-400:]
	}
	return s
}

// edges returns, per pixel, the Sobel edge direction quantized to eight
// bins (so a bright-on-dark stroke and a dark-on-bright one differ), or -1
// where there is no edge.
func edges(pix []byte) []int8 {
	e := make([]int8, fw*fh)
	for i := range e {
		e[i] = -1
	}
	at := func(x, y int) int { return int(pix[y*fw+x]) }
	for y := 1; y < fh-1; y++ {
		for x := 1; x < fw-1; x++ {
			gx := -at(x-1, y-1) - 2*at(x-1, y) - at(x-1, y+1) + at(x+1, y-1) + 2*at(x+1, y) + at(x+1, y+1)
			gy := -at(x-1, y-1) - 2*at(x, y-1) - at(x+1, y-1) + at(x-1, y+1) + 2*at(x, y+1) + at(x+1, y+1)
			if abs(gx)+abs(gy) <= edgeThreshold*4 {
				continue
			}
			a := math.Atan2(float64(gy), float64(gx)) // -pi..pi
			e[y*fw+x] = int8(int(math.Round(a/(math.Pi/4))+8) % 8)
		}
	}
	return e
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// learn finds, per corner, the pixels that are an edge with the same
// direction in most frames, and keeps the corner with the most of them.
func learn(frames []frame) mask {
	hits := make([][8]int, fw*fh)
	for _, f := range frames {
		for i, d := range edges(f.pix) {
			if d >= 0 {
				hits[i][d]++
			}
		}
	}
	need := int(stableShare * float64(len(frames)))
	var best mask
	for _, c := range corners {
		m := mask{Corner: c.name, X0: c.x0, Y0: c.y0, W: c.w, H: c.h, Dirs: make([]int8, c.w*c.h), Frames: len(frames)}
		for y := 0; y < c.h; y++ {
			for x := 0; x < c.w; x++ {
				m.Dirs[y*c.w+x] = -1
				h := hits[(c.y0+y)*fw+c.x0+x]
				for d, n := range h {
					// Neighbouring bins count too: a direction near a bin
					// boundary flips between the two with encoding noise.
					if n+h[(d+1)%8]+h[(d+7)%8] >= need && n >= h[(d+1)%8] && n >= h[(d+7)%8] {
						m.Dirs[y*c.w+x] = int8(d)
						m.Count++
						break
					}
				}
			}
		}
		fmt.Printf("  %-12s %4d stable edge pixels\n", c.name, m.Count)
		if m.Count > best.Count {
			best = m
		}
	}
	best.Share = stableShare
	return best
}

// score is the share of mask pixels where the frame has an edge in the
// same direction (one bin either way) at the same place. A busy
// background has edges everywhere, but rarely in the logo's directions.
func score(m mask, pix []byte) float64 {
	if m.Count == 0 {
		return 0
	}
	e := edges(pix)
	found := 0
	// No positional slack: with it, textured backgrounds scored up to 0.65
	// against a foreign logo; without, at most 0.35 (README).
	const slack = 0
	for y := 0; y < m.H; y++ {
		for x := 0; x < m.W; x++ {
			want := m.Dirs[y*m.W+x]
			if want < 0 {
				continue
			}
			gx, gy := m.X0+x, m.Y0+y
		near:
			for dy := -slack; dy <= slack; dy++ {
				for dx := -slack; dx <= slack; dx++ {
					px, py := gx+dx, gy+dy
					if px < 0 || py < 0 || px >= fw || py >= fh {
						continue
					}
					if d := e[py*fw+px]; d >= 0 && (d == want || (d+1)%8 == want || (d+7)%8 == want) {
						found++
						break near
					}
				}
			}
		}
	}
	return float64(found) / float64(m.Count)
}

func printMask(m mask) {
	fmt.Printf("mask: %s, %d pixels from %d frames\n", m.Corner, m.Count, m.Frames)
	for y := 0; y < m.H; y++ {
		var b bytes.Buffer
		for x := 0; x < m.W; x++ {
			if m.Dirs[y*m.W+x] >= 0 {
				b.WriteByte('#')
			} else {
				b.WriteByte('.')
			}
		}
		fmt.Println(b.String())
	}
}

func loadMask(path string) mask {
	var m mask
	data, err := os.ReadFile(path)
	check(err)
	check(json.Unmarshal(data, &m))
	return m
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// One DVB TS packet; slices start on a packet boundary.
const tsPacket = 188

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: adscan learn|scan|seek <rec.ts> [mask.json]")
		os.Exit(2)
	}
	cmd, rec := os.Args[1], os.Args[2]
	switch cmd {
	case "learn":
		t := time.Now()
		frames, err := keyframes(rec, nil, 0)
		check(err)
		fmt.Printf("%d keyframes in %v\n", len(frames), time.Since(t).Round(time.Millisecond))
		m := learn(frames)
		printMask(m)
		data, _ := json.Marshal(m)
		check(os.WriteFile(rec+".mask.json", data, 0o644))
		fmt.Println("written:", rec+".mask.json")
	case "scan":
		m := loadMask(os.Args[3])
		frames, err := keyframes(rec, nil, 0)
		check(err)
		for _, f := range frames {
			s := score(m, f.pix)
			fmt.Printf("%7.2fs  %.2f  %s\n", f.pts, s, bar(s))
		}
	case "seek":
		m := loadMask(os.Args[3])
		f, err := os.Open(rec)
		check(err)
		st, _ := f.Stat()
		const slice = 1536 * 1024
		for off := int64(0); off+slice <= st.Size(); off += st.Size() / 10 {
			aligned := off / tsPacket * tsPacket
			t := time.Now()
			frames, err := keyframes("-", io.NewSectionReader(f, aligned, slice), 1)
			d := time.Since(t).Round(time.Millisecond)
			if err != nil || len(frames) == 0 {
				fmt.Printf("offset %10d: no keyframe (%v) %v\n", aligned, err, d)
				continue
			}
			s := score(m, frames[0].pix)
			fmt.Printf("offset %10d: pts %9.2fs  score %.2f  %v\n", aligned, frames[0].pts, s, d)
		}
	default:
		fmt.Fprintln(os.Stderr, "unknown command", cmd)
		os.Exit(2)
	}
}

func bar(s float64) string {
	n := int(s*40 + 0.5)
	return string(bytes.Repeat([]byte("█"), n))
}
