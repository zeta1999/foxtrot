// SPDX-License-Identifier: Unlicense OR MIT

package editor

import (
	"fmt"
	"image"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"gioui.org/gesture"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"

	"golang.org/x/image/math/fixed"
)

// Editor implements an editable and scrollable text area.
type Editor struct {
	Alignment text.Alignment
	// SingleLine force the text to stay on a single line.
	// SingleLine also sets the scrolling direction to
	// horizontal.
	SingleLine bool
	// Submit enabled translation of carriage return keys to SubmitEvents.
	// If not enabled, carriage returns are inserted as newlines in the text.
	Submit bool

	eventKey     int
	scale        int
	font         text.Font
	blinkStart   time.Time
	focused      bool
	rr           editBuffer
	maxWidth     int
	viewSize     image.Point
	valid        bool
	lines        []text.Line
	shapes       []line
	dims         layout.Dimensions
	carWidth     fixed.Int26_6
	requestFocus bool
	caretOn      bool
	caretScroll  bool

	// carXOff is the offset to the current caret
	// position when moving between lines.
	carXOff fixed.Int26_6

	// is the offset of the select caret
	// position when selecting text
	selected *image.Point

	// Track the number of lines and length of current line
	lineCount, lineWidth, carLine, carCol int

	scroller  gesture.Scroll
	scrollOff image.Point

	clicker gesture.Click

	// events is the list of events not yet processed.
	events []EditorEvent
	// prevEvents is the number of events from the previous frame.
	prevEvents int
}

type EditorEvent interface {
	isEditorEvent()
}

// A ChangeEvent is generated for every user change to the text.
type ChangeEvent struct{}

type SubmitEvent struct{}

type UpEvent struct{}

type DownEvent struct{}

// A CommandEvent is generated for when moving the cursor
// and when command keys like enter and arrows-keys is pressed
type CommandEvent struct {
	Event key.Event
}

// A SubmitEvent is generated when Submit is set
// and a carriage return key is pressed.
//type SubmitEvent struct {
//	Text string
//}

const (
	blinksPerSecond  = 1
	maxBlinkDuration = 10 * time.Second
)

func (e *Editor) CaretLine() {
	e.carLine, e.carCol, _, _ = e.LayoutCaret()
	line := e.lines[e.carLine]
	e.lineWidth = len(line.Text.String)
}

// Events returns available editor events.
func (e *Editor) Events(gtx *layout.Context) []EditorEvent {
	e.processEvents(gtx)
	events := e.events
	e.events = nil
	e.prevEvents = 0
	return events
}

func (e *Editor) processEvents(gtx *layout.Context) {
	e.processPointer(gtx)
	e.processKey(gtx)
}

func (e *Editor) processPointer(gtx *layout.Context) {
	sbounds := e.scrollBounds()
	var smin, smax int
	var axis gesture.Axis
	if e.SingleLine {
		axis = gesture.Horizontal
		smin, smax = sbounds.Min.X, sbounds.Max.X
	} else {
		axis = gesture.Vertical
		smin, smax = sbounds.Min.Y, sbounds.Max.Y
	}
	sdist := e.scroller.Scroll(gtx, gtx, gtx.Now(), axis)
	var soff int
	if e.SingleLine {
		e.scrollRel(sdist, 0)
		soff = e.scrollOff.X
	} else {
		e.scrollRel(0, sdist)
		soff = e.scrollOff.Y
	}
	for _, evt := range e.clicker.Events(gtx) {
		switch {
		case evt.Type == gesture.TypePress && evt.Source == pointer.Mouse,
			evt.Type == gesture.TypeClick && evt.Source == pointer.Touch:
			e.blinkStart = gtx.Now()
			if e.selected == nil && evt.Modifiers.Contain(key.ModShift) {
				e.selected = &image.Point{X: e.carCol, Y: e.carLine}
			}
			p := image.Point{
				X: int(math.Round(float64(evt.Position.X))),
				Y: int(math.Round(float64(evt.Position.Y)))}
			e.moveCoord(gtx, p)
			e.requestFocus = true
			if e.scroller.State() != gesture.StateFlinging {
				e.caretScroll = true
			}
		}
	}
	if (sdist > 0 && soff >= smax) || (sdist < 0 && soff <= smin) {
		e.scroller.Stop()
	}
}

func (e *Editor) processKey(gtx *layout.Context) {
	for _, ke := range gtx.Events(&e.eventKey) {
		e.blinkStart = gtx.Now()
		switch ke := ke.(type) {
		case key.FocusEvent:
			e.focused = ke.Focus
		case key.Event:
			if !e.focused {
				break
			}
			if (ke.Name == key.NameEnter || ke.Name == key.NameReturn) && ke.Modifiers.Contain(key.ModShift) {
				e.events = append(e.events, SubmitEvent{})
				return
			} else if ke.Name == key.NameUpArrow && e.carLine == 0 {
				e.events = append(e.events, UpEvent{})
			} else if ke.Name == key.NameLeftArrow && e.carLine == 0 && e.carCol == 0 {
				e.events = append(e.events, UpEvent{})
			} else if ke.Name == key.NameDownArrow && e.carLine == e.lineCount-1 {
				e.events = append(e.events, DownEvent{})
			} else if ke.Name == key.NameRightArrow && e.carLine == e.lineCount-1 && e.carCol == e.lineWidth {
				e.events = append(e.events, DownEvent{})
			}
			fmt.Printf("Caret: lineCount: %v, lineWidth: %v, caretY: %v, caretX: %v\n", e.lineCount, e.lineWidth, e.carLine, e.carCol)
			if e.command(ke) {
				e.caretScroll = true
				e.scroller.Stop()
				e.CaretLine()
			}
			//e.events = append(e.events, CommandEvent{Event: ke})
		case key.EditEvent:
			e.caretScroll = true
			e.scroller.Stop()
			e.append(ke.Text)
			e.CaretLine()
		}
		if e.rr.Changed() {
			e.events = append(e.events, ChangeEvent{})
		}
	}
}

// Focus requests the input focus for the Editor.
func (e *Editor) Focus() {
	e.requestFocus = true
}

func (e *Editor) PaintText(gtx *layout.Context) {
	clip := textPadding(e.lines)
	clip.Max = clip.Max.Add(e.viewSize)
	for _, shape := range e.shapes {
		var stack op.StackOp
		stack.Push(gtx.Ops)
		op.TransformOp{}.Offset(shape.offset).Add(gtx.Ops)
		shape.clip.Add(gtx.Ops)
		paint.PaintOp{Rect: toRectF(clip).Sub(shape.offset)}.Add(gtx.Ops)
		stack.Pop()
	}
}

func (e *Editor) PaintCaret(gtx *layout.Context) {
	if !e.caretOn {
		return
	}
	carLine, _, carX, carY := e.LayoutCaret()

	var stack op.StackOp
	stack.Push(gtx.Ops)
	carX -= e.carWidth / 2
	carAsc, carDesc := -e.lines[carLine].Bounds.Min.Y, e.lines[carLine].Bounds.Max.Y
	carRect := image.Rectangle{
		Min: image.Point{X: carX.Ceil(), Y: carY - carAsc.Ceil()},
		Max: image.Point{X: carX.Ceil() + e.carWidth.Ceil(), Y: carY + carDesc.Ceil()},
	}
	carRect = carRect.Add(image.Point{
		X: -e.scrollOff.X,
		Y: -e.scrollOff.Y,
	})
	clip := textPadding(e.lines)
	// Account for caret width to each side.
	whalf := (e.carWidth / 2).Ceil()
	if clip.Max.X < whalf {
		clip.Max.X = whalf
	}
	if clip.Min.X > -whalf {
		clip.Min.X = -whalf
	}
	clip.Max = clip.Max.Add(e.viewSize)
	carRect = clip.Intersect(carRect)
	if !carRect.Empty() {
		paint.PaintOp{Rect: toRectF(carRect)}.Add(gtx.Ops)
	}
	stack.Pop()
}

// Len is the length of the editor contents.
func (e *Editor) Len() int {
	return e.rr.len()
}

// Text returns the contents of the editor.
func (e *Editor) Text() string {
	return e.rr.String()
}

// SetText replaces the contents of the editor.
func (e *Editor) SetText(s string) {
	e.rr = editBuffer{}
	e.carXOff = 0
	e.prepend(s)
}

func (e *Editor) scrollBounds() image.Rectangle {
	var b image.Rectangle
	if e.SingleLine {
		if len(e.lines) > 0 {
			b.Min.X = align(e.Alignment, e.lines[0].Width, e.viewSize.X).Floor()
			if b.Min.X > 0 {
				b.Min.X = 0
			}
		}
		b.Max.X = e.dims.Size.X + b.Min.X - e.viewSize.X
	} else {
		b.Max.Y = e.dims.Size.Y - e.viewSize.Y
	}
	return b
}

func (e *Editor) scrollRel(dx, dy int) {
	e.scrollAbs(e.scrollOff.X+dx, e.scrollOff.Y+dy)
}

func (e *Editor) scrollAbs(x, y int) {
	e.scrollOff.X = x
	e.scrollOff.Y = y
	b := e.scrollBounds()
	if e.scrollOff.X > b.Max.X {
		e.scrollOff.X = b.Max.X
	}
	if e.scrollOff.X < b.Min.X {
		e.scrollOff.X = b.Min.X
	}
	if e.scrollOff.Y > b.Max.Y {
		e.scrollOff.Y = b.Max.Y
	}
	if e.scrollOff.Y < b.Min.Y {
		e.scrollOff.Y = b.Min.Y
	}
}

func (e *Editor) moveCoord(c unit.Converter, pos image.Point) {
	var (
		prevDesc fixed.Int26_6
		carLine  int
		y        int
	)
	for _, l := range e.lines {
		y += (prevDesc + l.Ascent).Ceil()
		prevDesc = l.Descent
		if y+prevDesc.Ceil() >= pos.Y+e.scrollOff.Y {
			break
		}
		carLine++
	}
	x := fixed.I(pos.X + e.scrollOff.X)
	e.moveToLine(x, carLine)
}

func (e *Editor) layoutText(c unit.Converter, s *text.Shaper, font text.Font) ([]text.Line, layout.Dimensions) {
	txt := e.rr.String()
	opts := text.LayoutOptions{MaxWidth: e.maxWidth}
	textLayout := s.Layout(c, font, txt, opts)
	lines := textLayout.Lines
	dims := linesDimens(lines)
	for i := 0; i < len(lines)-1; i++ {
		s := lines[i].Text.String
		// To avoid layout flickering while editing, assume a soft newline takes
		// up all available space.
		if len(s) > 0 {
			r, _ := utf8.DecodeLastRuneInString(s)
			if r != '\n' {
				dims.Size.X = e.maxWidth
				break
			}
		}
	}
	return lines, dims
}

func (e *Editor) LayoutCaret() (carLine, carCol int, x fixed.Int26_6, y int) {
	var idx int
	var prevDesc fixed.Int26_6
loop:
	for carLine = 0; carLine < len(e.lines); carLine++ {
		l := e.lines[carLine]
		y += (prevDesc + l.Ascent).Ceil()
		prevDesc = l.Descent
		if carLine == len(e.lines)-1 || idx+len(l.Text.String) > e.rr.caret {
			str := l.Text.String
			for _, adv := range l.Text.Advances {
				if idx == e.rr.caret {
					break loop
				}
				x += adv
				_, s := utf8.DecodeRuneInString(str)
				idx += s
				str = str[s:]
				carCol++
			}
			break
		}
		idx += len(l.Text.String)
	}
	x += align(e.Alignment, e.lines[carLine].Width, e.viewSize.X)
	return
}

func (e *Editor) invalidate() {
	e.valid = false
}

func (e *Editor) deleteRune() {
	e.rr.deleteRune()
	e.carXOff = 0
	e.invalidate()
}

func (e *Editor) deleteRuneForward() {
	e.rr.deleteRuneForward()
	e.carXOff = 0
	e.invalidate()
}

func (e *Editor) append(s string) {
	if e.SingleLine {
		s = strings.ReplaceAll(s, "\n", "")
	}
	e.prepend(s)
	e.rr.caret += len(s)
}

func (e *Editor) prepend(s string) {
	e.rr.prepend(s)
	e.carXOff = 0
	e.invalidate()
}

func (e *Editor) movePages(pages int) {
	_, _, carX, carY := e.LayoutCaret()
	y := carY + pages*e.viewSize.Y
	var (
		prevDesc fixed.Int26_6
		carLine2 int
	)
	y2 := e.lines[0].Ascent.Ceil()
	for i := 1; i < len(e.lines); i++ {
		if y2 >= y {
			break
		}
		l := e.lines[i]
		h := (prevDesc + l.Ascent).Ceil()
		prevDesc = l.Descent
		if y2+h-y >= y-y2 {
			break
		}
		y2 += h
		carLine2++
	}
	e.carXOff = e.moveToLine(carX+e.carXOff, carLine2)
}

func (e *Editor) moveToLine(carX fixed.Int26_6, carLine2 int) fixed.Int26_6 {
	carLine, carCol, _, _ := e.LayoutCaret()
	if carLine2 < 0 {
		carLine2 = 0
	}
	if carLine2 >= len(e.lines) {
		carLine2 = len(e.lines) - 1
	}
	// Move to start of line.
	for i := carCol - 1; i >= 0; i-- {
		_, s := e.rr.runeBefore(e.rr.caret)
		e.rr.caret -= s
	}
	if carLine2 != carLine {
		// Move to start of line2.
		if carLine2 > carLine {
			for i := carLine; i < carLine2; i++ {
				e.rr.caret += len(e.lines[i].Text.String)
			}
		} else {
			for i := carLine - 1; i >= carLine2; i-- {
				e.rr.caret -= len(e.lines[i].Text.String)
			}
		}
	}
	l2 := e.lines[carLine2]
	carX2 := align(e.Alignment, l2.Width, e.viewSize.X)
	// Only move past the end of the last line
	end := 0
	if carLine2 < len(e.lines)-1 {
		end = 1
	}
	// Move to rune closest to previous horizontal position.
	for i := 0; i < len(l2.Text.Advances)-end; i++ {
		adv := l2.Text.Advances[i]
		if carX2 >= carX {
			break
		}
		if carX2+adv-carX >= carX-carX2 {
			break
		}
		carX2 += adv
		_, s := e.rr.runeAt(e.rr.caret)
		e.rr.caret += s
	}
	return carX - carX2
}

func (e *Editor) moveLeft() {
	e.rr.moveLeft()
	e.carXOff = 0
	e.selected = nil
}

func (e *Editor) moveRight() {
	e.rr.moveRight()
	e.carXOff = 0
	e.selected = nil
}

func (e *Editor) moveStart() {
	carLine, carCol, x, _ := e.LayoutCaret()
	advances := e.lines[carLine].Text.Advances
	for i := carCol - 1; i >= 0; i-- {
		_, s := e.rr.runeBefore(e.rr.caret)
		e.rr.caret -= s
		x -= advances[i]
	}
	e.carXOff = -x
}

func (e *Editor) moveEnd() {
	carLine, carCol, x, _ := e.LayoutCaret()
	l := e.lines[carLine]
	// Only move past the end of the last line
	end := 0
	if carLine < len(e.lines)-1 {
		end = 1
	}
	for i := carCol; i < len(l.Text.Advances)-end; i++ {
		adv := l.Text.Advances[i]
		_, s := e.rr.runeAt(e.rr.caret)
		e.rr.caret += s
		x += adv
	}
	a := align(e.Alignment, l.Width, e.viewSize.X)
	e.carXOff = l.Width + a - x
}

func (e *Editor) scrollToCaret() {
	carLine, _, x, y := e.LayoutCaret()
	l := e.lines[carLine]
	if e.SingleLine {
		var dist int
		if d := x.Floor() - e.scrollOff.X; d < 0 {
			dist = d
		} else if d := x.Ceil() - (e.scrollOff.X + e.viewSize.X); d > 0 {
			dist = d
		}
		e.scrollRel(dist, 0)
	} else {
		miny := y - l.Ascent.Ceil()
		maxy := y + l.Descent.Ceil()
		var dist int
		if d := miny - e.scrollOff.Y; d < 0 {
			dist = d
		} else if d := maxy - (e.scrollOff.Y + e.viewSize.Y); d > 0 {
			dist = d
		}
		e.scrollRel(0, dist)
	}
}

func (e *Editor) hasSelection(k key.Event) bool {
	return false
}

func (e *Editor) command(k key.Event) bool {
	switch k.Name {
	case key.NameReturn, key.NameEnter:
		e.append("\n")
	case key.NameDeleteBackward:
		e.deleteRune()
	case key.NameDeleteForward:
		e.deleteRuneForward()
	case key.NameUpArrow:
		line, carCol, carX, _ := e.LayoutCaret()
		e.carXOff = e.moveToLine(carX+e.carXOff, line-1)
		if k.Modifiers.Contain(key.ModShift) && e.selected != nil {
			e.selected = &image.Point{X: carCol, Y: line}
		}
	case key.NameDownArrow:
		line, carCol, carX, _ := e.LayoutCaret()
		e.carXOff = e.moveToLine(carX+e.carXOff, line+1)
		if k.Modifiers.Contain(key.ModShift) && e.selected != nil {
			e.selected = &image.Point{X: carCol, Y: line}
		}
	case key.NameLeftArrow:
		e.moveLeft()
		if k.Modifiers.Contain(key.ModShift) && e.selected != nil {
			e.selected = &image.Point{X: e.carCol, Y: e.carLine}
		}
	case key.NameRightArrow:
		e.moveRight()
		if k.Modifiers.Contain(key.ModShift) && e.selected != nil {
			e.selected = &image.Point{X: e.carCol, Y: e.carLine}
		}
	case key.NamePageUp:
		e.movePages(-1)
	case key.NamePageDown:
		e.movePages(+1)
	case key.NameHome:
		e.moveStart()
	case key.NameEnd:
		e.moveEnd()
	default:
		return false
	}
	return true
}

func (s ChangeEvent) isEditorEvent()  {}
func (s CommandEvent) isEditorEvent() {}
func (s SubmitEvent) isEditorEvent()  {}
func (s UpEvent) isEditorEvent()      {}
func (s DownEvent) isEditorEvent()    {}
