package mock

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
)

// coverJPEG is a small real JPEG poster for the mock's cover-art fixture.
func coverJPEG() string {
	img := image.NewRGBA(image.Rect(0, 0, 120, 180))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.RGBA{140, 40, 50, 255}), image.Point{}, draw.Src)
	draw.Draw(img, image.Rect(0, 120, 120, 180), image.NewUniform(color.RGBA{30, 30, 40, 255}), image.Point{}, draw.Src)
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80})
	return buf.String()
}

// Linux input-event codes the mock's simulated screen reacts to — the
// subset confirmed live on the reference box (ARCHITECTURE.md, "Real
// screenshot and remote-control endpoints").
const (
	keyUp    = 103
	keyDown  = 108
	keyOK    = 352
	keyExit  = 174
	keyMenu  = 139
	keyPower = 116
)

// menuRows is how many entries the mock's simulated on-screen menu has.
const menuRows = 6

// screen is the mock box's simulated on-screen state, so ControlHub's
// remote visibly does something in mock mode instead of every key press
// disappearing without a trace: MENU opens a menu overlay, UP/DOWN move
// its highlight, OK/EXIT close it, POWER toggles standby (a black screen,
// like the real box's screenshot in standby). Any other key is ignored,
// the same way the real box ignores a key its current screen doesn't use.
type screen struct {
	menuOpen bool
	selected int
}

// press applies one key to the simulated screen. Must be called with the
// client's mutex held; standby is the client's own Standby field.
func (s *screen) press(code int, standby *bool) {
	if code == keyPower {
		*standby = !*standby
		s.menuOpen = false
		return
	}
	if *standby {
		return // the real box ignores everything but POWER in standby
	}
	switch code {
	case keyMenu:
		s.menuOpen = true
		s.selected = 0
	case keyExit, keyOK:
		s.menuOpen = false
	case keyUp:
		if s.menuOpen {
			s.selected = (s.selected + menuRows - 1) % menuRows
		}
	case keyDown:
		if s.menuOpen {
			s.selected = (s.selected + 1) % menuRows
		}
	}
}

// render draws the current state as a 1280×720 PNG: a color-bar test card
// as the "video", plus the menu overlay when it's open.
func (s *screen) render(standby bool) ([]byte, error) {
	const w, h = 1280, 720
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	if standby {
		draw.Draw(img, img.Bounds(), image.NewUniform(color.Black), image.Point{}, draw.Src)
	} else {
		bars := []color.RGBA{
			{192, 192, 192, 255}, {192, 192, 0, 255}, {0, 192, 192, 255}, {0, 192, 0, 255},
			{192, 0, 192, 255}, {192, 0, 0, 255}, {0, 0, 192, 255},
		}
		barW := w / len(bars)
		for i, c := range bars {
			x1 := (i + 1) * barW
			if i == len(bars)-1 {
				x1 = w
			}
			draw.Draw(img, image.Rect(i*barW, 0, x1, h*2/3), image.NewUniform(c), image.Point{}, draw.Src)
		}
		draw.Draw(img, image.Rect(0, h*2/3, w, h), image.NewUniform(color.RGBA{24, 28, 34, 255}), image.Point{}, draw.Src)
		if s.menuOpen {
			panel := image.Rect(w/2-260, 120, w/2+260, 120+menuRows*70+40)
			draw.Draw(img, panel, image.NewUniform(color.RGBA{15, 18, 21, 235}), image.Point{}, draw.Over)
			for i := 0; i < menuRows; i++ {
				row := image.Rect(panel.Min.X+20, panel.Min.Y+20+i*70, panel.Max.X-20, panel.Min.Y+20+i*70+56)
				c := color.RGBA{37, 43, 50, 255}
				if i == s.selected {
					c = color.RGBA{106, 166, 232, 255}
				}
				draw.Draw(img, row, image.NewUniform(c), image.Point{}, draw.Src)
			}
		}
	}
	var buf bytes.Buffer
	if err := (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
