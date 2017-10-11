package rat

import (
	"fmt"
	"strings"

	termbox "github.com/nsf/termbox-go"
)

type Pager interface {
	Widget
	AddEventListener(keyStr string, handler func())
	AddAnnotationEventListener(keyStr string, annotationTypes []string, handler func(Context))
	Reload()
	CursorUp()
	CursorDown()
	CursorFirstLine()
	CursorLastLine()
	ScrollUp()
	ScrollDown()
	PageUp()
	PageDown()
}

type cmdPager struct {
	modes                    []Mode
	cmd                      string
	ctx                      Context
	command                  ShellCommand
	buffer                   Buffer
	scrollOffsetY            int
	cursorY                  int
	stop                     chan bool
	eventListeners           map[keyEvent]func()
	annotationEventListeners map[keyEvent]map[string]func(Context)

	box        Box
	headerBox  Box
	contentBox Box
}

func NewCmdPager(modeNames string, cmd string, ctx Context) Pager {
	p := &cmdPager{}
	p.cmd = cmd
	p.ctx = ctx

	p.eventListeners = make(map[keyEvent]func())
	p.annotationEventListeners = make(map[keyEvent]map[string]func(Context))

	p.addDefaultListeners()

	splitModeNames := strings.Split(modeNames, ",")
	p.modes = make([]Mode, 0, len(splitModeNames))

	for _, modeName := range splitModeNames {
		if mode, ok := modes[modeName]; ok {
			p.modes = append(p.modes, mode)

			mode.AddEventListeners(ctx)(p)
		}
	}

	p.RunCommand()

	return p
}

func (p *cmdPager) AddEventListener(keyStr string, handler func()) {
	p.eventListeners[KeyEventFromString(keyStr)] = handler
}

func (p *cmdPager) AddAnnotationEventListener(keyStr string, annotationTypes []string, handler func(Context)) {
	ke := KeyEventFromString(keyStr)

	if _, ok := p.annotationEventListeners[ke]; !ok {
		p.annotationEventListeners[ke] = make(map[string]func(Context))
	}

	for _, annotationType := range annotationTypes {
		p.annotationEventListeners[KeyEventFromString(keyStr)][annotationType] = handler
	}
}

func (p *cmdPager) Destroy() {
	p.Stop()
}

func (p *cmdPager) Stop() {
	p.command.Close()
	p.buffer.Close()
}

func (p *cmdPager) Reload() {
	p.Stop()
	p.RunCommand()
}

func (p *cmdPager) RunCommand() {
	var err error

	if p.command, err = NewShellCommand(p.InterpolatedCmd()); err != nil {
		panic(err)
	}

	p.buffer = NewBuffer(p.command)

	for _, m := range p.modes {
		for _, a := range m.InitAnnotators(p.ctx)() {
			go p.buffer.AnnotateWith(a)
		}
	}
}

func (p *cmdPager) HandleEvent(ke keyEvent) bool {
	p.buffer.Lock()
	defer p.buffer.Unlock()

	annotations := p.buffer.AnnotationsForLine(p.cursorY)

	ctx := Context{}
	for _, a := range annotations {
		ctx[a.Class()] = a.Val()
	}

	if handlers, ok := p.annotationEventListeners[ke]; ok && len(annotations) > 0 {
		for _, a := range annotations {
			if handler, ok := handlers[a.Class()]; ok {
				handler(ctx)
				return true
			}
		}
	}

	if handler, ok := p.eventListeners[ke]; ok {
		handler()
		return true
	}

	return false
}

func (p *cmdPager) SetBox(box Box) {
	p.box = box
	p.layout()
}

func (p *cmdPager) GetBox() Box {
	return p.box
}

func (p *cmdPager) layout() {
	p.headerBox = NewBox(p.box.Left(), p.box.Top(), p.box.Width(), 1)
	p.contentBox = NewBox(p.box.Left(), p.box.Top()+1, p.box.Width(), p.box.Height()-1)
}

func (p *cmdPager) drawHeader() {
	p.headerBox.DrawStyledRunes(1, 0, StyledRunesFromString(p.InterpolatedCmd(), gTermStyles.Get(termbox.AttrUnderline, termbox.ColorDefault)))

	pagerInfo := StyledRunesFromString(fmt.Sprintf(" %d %d/%d ", p.buffer.NumAnnotations(), p.cursorY+1, p.buffer.NumLines()), gTermStyles.Get(termbox.AttrBold, termbox.ColorDefault))
	p.headerBox.DrawStyledRunes(p.headerBox.Width()-len(pagerInfo), 0, pagerInfo)
}

func (p *cmdPager) drawContent() {
	p.contentBox.DrawStyledRune(1, p.cursorY-p.scrollOffsetY, NewStyledRune('>', gTermStyles.Get(termbox.ColorRed, termbox.ColorDefault)))

	for y, line := range p.buffer.StyledLines(p.scrollOffsetY, p.contentBox.Height()) {
		p.contentBox.DrawStyledRunes(3, y, []StyledRune(line))
	}
}

func (p *cmdPager) Render() {
	p.buffer.Lock()
	p.drawHeader()
	p.drawContent()
	p.buffer.Unlock()
}

func (p *cmdPager) MoveCursorToY(cursorY int) {
	if cursorY < 0 {
		p.cursorY = 0
	} else if cursorY >= p.buffer.NumLines() {
		p.cursorY = p.buffer.NumLines() - 1
	} else {
		p.cursorY = cursorY
	}

	if p.cursorY < p.scrollOffsetY {
		p.ScrollToY(p.cursorY)
	} else if p.cursorY > p.scrollOffsetY+p.contentBox.Height()-1 {
		p.ScrollToY(p.cursorY - (p.contentBox.Height() - 1))
	}
}

func (p *cmdPager) MoveCursorY(delta int) {
	p.MoveCursorToY(p.cursorY + delta)
}

func (p *cmdPager) ScrollToY(scrollY int) {
	if scrollY < 0 {
		p.scrollOffsetY = 0
	} else if scrollY >= p.buffer.NumLines()-p.contentBox.Height() {
		if p.buffer.NumLines() > p.contentBox.Height() {
			p.scrollOffsetY = p.buffer.NumLines() - p.contentBox.Height()
		} else {
			p.scrollOffsetY = 0
		}
	} else {
		p.scrollOffsetY = scrollY
	}

	if p.cursorY < p.scrollOffsetY {
		p.MoveCursorToY(p.scrollOffsetY)
	} else if p.cursorY > p.scrollOffsetY+p.contentBox.Height()-1 {
		p.MoveCursorToY(p.scrollOffsetY + p.contentBox.Height() - 1)
	}
}

func (p *cmdPager) ScrollY(delta int) {
	p.ScrollToY(p.scrollOffsetY + delta)
}

func (p *cmdPager) CursorY() int {
	return p.cursorY
}

func (p *cmdPager) addDefaultListeners() {
	p.AddEventListener("C-r", p.Reload)
	p.AddEventListener("j", p.CursorDown)
	p.AddEventListener("k", p.CursorUp)
	p.AddEventListener("down", p.CursorDown)
	p.AddEventListener("up", p.CursorUp)
	p.AddEventListener("C-e", p.ScrollDown)
	p.AddEventListener("C-y", p.ScrollUp)
	p.AddEventListener("pgdn", p.PageDown)
	p.AddEventListener("pgup", p.PageUp)
	p.AddEventListener("g", p.CursorFirstLine)
	p.AddEventListener("S-g", p.CursorLastLine)
}

func (p *cmdPager) CursorUp() {
	p.MoveCursorY(-1)
}

func (p *cmdPager) CursorDown() {
	p.MoveCursorY(1)
}

func (p *cmdPager) CursorFirstLine() {
	p.MoveCursorToY(0)
}

func (p *cmdPager) CursorLastLine() {
	p.MoveCursorToY(p.buffer.NumLines())
}

func (p *cmdPager) ScrollUp() {
	p.ScrollY(-1)
}

func (p *cmdPager) ScrollDown() {
	p.ScrollY(1)
}

func (p *cmdPager) PageUp() {
	p.ScrollY(-p.contentBox.Height())
}

func (p *cmdPager) PageDown() {
	p.ScrollY(p.contentBox.Height())
}

func (p *cmdPager) InterpolatedCmd() string {
	return InterpolateContext(p.cmd, p.ctx)
}
