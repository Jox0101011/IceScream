package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gdamore/tcell/v2"
	"github.com/google/uuid"
)

var (
	styleDefault   = tcell.StyleDefault.Background(tcell.ColorBlack).Foreground(tcell.ColorSilver)
	styleStatusBar = tcell.StyleDefault.Background(tcell.ColorNavy).Foreground(tcell.ColorWhite).Bold(true)
	styleInputBar  = tcell.StyleDefault.Background(tcell.ColorBlack).Foreground(tcell.ColorWhite)
	styleTimestamp = tcell.StyleDefault.Foreground(tcell.ColorGray)
	styleNick      = tcell.StyleDefault.Foreground(tcell.ColorAqua).Bold(true)
	styleNickSelf  = tcell.StyleDefault.Foreground(tcell.ColorLime).Bold(true)
	styleSystem    = tcell.StyleDefault.Foreground(tcell.ColorYellow)
	styleError     = tcell.StyleDefault.Foreground(tcell.ColorRed).Bold(true)
	styleJoin      = tcell.StyleDefault.Foreground(tcell.ColorGreen)
	stylePart      = tcell.StyleDefault.Foreground(tcell.ColorOlive)
	styleSeparator = tcell.StyleDefault.Background(tcell.ColorNavy).Foreground(tcell.ColorSilver)
)

type LineKind int

const (
	KindChat LineKind = iota
	KindSystem
	KindError
	KindJoin
	KindPart
	KindUpload
	KindExec
)

type Line struct {
	kind      LineKind
	timestamp time.Time
	nick      string 
	text      string
	self      bool
}

type TUI struct {
	node   *Node
	screen tcell.Screen

	lines   []Line
	linesMu sync.Mutex

	scrollOffset int

	input    []rune
	inputPos int

	quit chan struct{}
}

func NewTUI(node *Node) (*TUI, error) {
	s, err := tcell.NewScreen()
	if err != nil {
		return nil, err
	}
	if err := s.Init(); err != nil {
		return nil, err
	}
	s.SetStyle(styleDefault)
	s.Clear()

	tui := &TUI{
		node:   node,
		screen: s,
		quit:   make(chan struct{}),
	}

	node.SetOutput(tui)

	return tui, nil
}

func (t *TUI) AddLine(l Line) {
	t.linesMu.Lock()
	t.lines = append(t.lines, l)
	t.linesMu.Unlock()
	t.draw()
}

func (t *TUI) WriteSystem(text string)          { t.AddLine(Line{KindSystem, time.Now(), "", text, false}) }
func (t *TUI) WriteError(text string)           { t.AddLine(Line{KindError, time.Now(), "", text, false}) }
func (t *TUI) WriteJoin(nick, addr string)      { t.AddLine(Line{KindJoin, time.Now(), nick, addr, false}) }
func (t *TUI) WritePart(nick, addr string)      { t.AddLine(Line{KindPart, time.Now(), nick, addr, false}) }
func (t *TUI) WriteChat(nick, text string, self bool) {
	t.AddLine(Line{KindChat, time.Now(), nick, text, self})
}
func (t *TUI) WriteUpload(from, filename string, size int64) {
	t.AddLine(Line{KindUpload, time.Now(), from,
		fmt.Sprintf("sended '%s' (%d bytes) → ./received/", filename, size), false})
}
func (t *TUI) WriteExec(from, cmd, reqID string) {
	t.AddLine(Line{KindExec, time.Now(), from,
		fmt.Sprintf("exec '%s' (req: %s)", cmd, reqID), false})
}
func (t *TUI) WriteExecReply(from, output, errStr string) {
	text := output
	if errStr != "" {
		text += "\n[erro] " + errStr
	}
	t.AddLine(Line{KindExec, time.Now(), from, text, false})
}

func (t *TUI) Run() {
	defer t.screen.Fini()

	t.AddLine(Line{KindSystem, time.Now(), "", "IceScream v0.1.0 — /help to commands list", false})

	go func() {
		tick := time.NewTicker(2 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-tick.C:
				t.draw()
			case <-t.quit:
				return
			}
		}
	}()

	for {
		t.draw()
		ev := t.screen.PollEvent()
		switch ev := ev.(type) {
		case *tcell.EventResize:
			t.screen.Sync()

		case *tcell.EventKey:
			if !t.handleKey(ev) {
				return
			}
		}
	}
}

func (t *TUI) handleKey(ev *tcell.EventKey) bool {
	switch ev.Key() {
	case tcell.KeyEscape, tcell.KeyCtrlC:
		t.node.Quit()
		close(t.quit)
		return false

	case tcell.KeyEnter:
		t.submit()

	case tcell.KeyBackspace, tcell.KeyBackspace2:
		if t.inputPos > 0 {
			t.input = append(t.input[:t.inputPos-1], t.input[t.inputPos:]...)
			t.inputPos--
		}

	case tcell.KeyDelete:
		if t.inputPos < len(t.input) {
			t.input = append(t.input[:t.inputPos], t.input[t.inputPos+1:]...)
		}

	case tcell.KeyLeft:
		if t.inputPos > 0 {
			t.inputPos--
		}
	case tcell.KeyRight:
		if t.inputPos < len(t.input) {
			t.inputPos++
		}
	case tcell.KeyHome, tcell.KeyCtrlA:
		t.inputPos = 0
	case tcell.KeyEnd, tcell.KeyCtrlE:
		t.inputPos = len(t.input)

	case tcell.KeyPgUp:
		_, h := t.screen.Size()
		chatH := h - 3
		t.scrollOffset += chatH / 2
		t.clampScroll()

	case tcell.KeyPgDn:
		_, h := t.screen.Size()
		chatH := h - 3
		t.scrollOffset -= chatH / 2
		if t.scrollOffset < 0 {
			t.scrollOffset = 0
		}

	case tcell.KeyRune:
		r := ev.Rune()
		t.input = append(t.input[:t.inputPos], append([]rune{r}, t.input[t.inputPos:]...)...)
		t.inputPos++
	}
	return true
}

func (t *TUI) clampScroll() {
	_, h := t.screen.Size()
	chatH := h - 3
	t.linesMu.Lock()
	total := len(t.lines)
	t.linesMu.Unlock()
	max := total - chatH
	if max < 0 {
		max = 0
	}
	if t.scrollOffset > max {
		t.scrollOffset = max
	}
}

func (t *TUI) submit() {
	line := strings.TrimSpace(string(t.input))
	t.input = t.input[:0]
	t.inputPos = 0
	if line == "" {
		return
	}

	if strings.HasPrefix(line, "/") {
		t.handleSlashCommand(line)
	} else if strings.HasPrefix(line, ":") {
		t.handleColonCommand(line)
	} else {
		t.doChatSend(line)
	}
}

func (t *TUI) handleSlashCommand(line string) {
	parts := strings.Fields(line)
	switch parts[0] {
	case "/connect":
		if len(parts) < 2 {
			t.WriteError("Uso: /connect <host:port>")
			return
		}
		addr := parts[1]
		if err := t.node.Connect(addr); err != nil {
			t.WriteError(fmt.Sprintf("connect: %v", err))
		}

	case "/peers":
		t.node.peersMu.RLock()
		defer t.node.peersMu.RUnlock()
		if len(t.node.peers) == 0 {
			t.WriteSystem("none peer connected.")
		} else {
			for addr := range t.node.peers {
				t.WriteSystem("  peer: " + addr)
			}
		}

	case "/ping":
		t.node.peersMu.RLock()
		peers := make([]*Peer, 0, len(t.node.peers))
		for _, p := range t.node.peers {
			peers = append(peers, p)
		}
		t.node.peersMu.RUnlock()
		for _, p := range peers {
			ping := NewMessage(MsgPing, t.node.nickname, t.node.listenAddr, nil)
			_ = t.node.sendTo(p, ping)
			t.WriteSystem("PING → " + p.addr)
		}

	case "/quit":
		t.node.Quit()
		close(t.quit)

	case "/help":
		t.printHelp()

	default:
		t.WriteError("Command unknown: " + parts[0])
	}
}

func (t *TUI) handleColonCommand(line string) {
	idx := strings.Index(line, " ")
	var cmd, args string
	if idx == -1 {
		cmd = line
	} else {
		cmd = line[:idx]
		args = strings.TrimSpace(line[idx+1:])
	}

	switch cmd {
	case ":chat":
		if args == "" {
			t.WriteSystem("Chat mode: simply type and press enter")
			return
		}
		t.doChatSend(args)

	case ":upload":
		if args == "" {
			t.WriteError("Usage: :upload <archive>")
			return
		}
		t.doUpload(args)

	case ":exec":
		if args == "" {
			t.WriteError("Uso: :exec <comando>")
			return
		}
		t.doExec(args)

	default:
		t.WriteError("Unknown mode: " + cmd)
	}
}

func (t *TUI) doChatSend(text string) {
	if t.node.peerCount() == 0 {
		t.WriteError("None connected peer.")
		return
	}
	msg := NewMessage(MsgChat, t.node.nickname, t.node.listenAddr, ChatPayload{Text: text})
	t.node.Broadcast(msg)
	t.WriteChat(t.node.nickname, text, true)
}

func (t *TUI) doUpload(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		t.WriteError(fmt.Sprintf("upload: %v", err))
		return
	}
	payload := UploadPayload{Filename: filepath.Base(path), Size: int64(len(data)), Data: data}
	msg := NewMessage(MsgUpload, t.node.nickname, t.node.listenAddr, payload)
	t.node.Broadcast(msg)
	t.WriteSystem(fmt.Sprintf("Sending '%s' (%d bytes)...", filepath.Base(path), len(data)))
}

func (t *TUI) doExec(command string) {
	payload := ExecPayload{Command: command, ReqID: uuid.New().String()}
	msg := NewMessage(MsgExec, t.node.nickname, t.node.listenAddr, payload)
	t.node.Broadcast(msg)
	t.WriteSystem(fmt.Sprintf("exec '%s'", command))
}

func (t *TUI) printHelp() {
	helps := []string{
		"── IceScream Commands ──────────────────────",
		" /connect <host:porta>  connect to one peer",
		" /peers                 list the peers",
		" /ping                  ping in all peers",
		" /quit                  exit program",
		" /help                  commands",
		" :chat <msg>            send msg",
		" :upload <arquivo>      send archive",
		" :exec <cmd>            exec command",
		" PgUp/PgDn              scroll",
		"────────────────────────────────────────────",
	}
	for _, h := range helps {
		t.WriteSystem(h)
	}
}

func (t *TUI) draw() {
	s := t.screen
	w, h := s.Size()
	if w == 0 || h == 0 {
		return
	}

	chatH := h - 2
	if chatH < 1 {
		chatH = 1
	}

	s.Clear()
	t.drawChat(0, 0, w, chatH)
	t.drawStatusBar(h-2, w)
	t.drawInputBar(h-1, w)
	s.Show()
}

func (t *TUI) drawChat(x, y, w, h int) {
	t.linesMu.Lock()
	lines := make([]Line, len(t.lines))
	copy(lines, t.lines)
	t.linesMu.Unlock()

	type VLine struct {
		prefix      string
		prefixStyle tcell.Style
		text        string
		textStyle   tcell.Style
	}

	var visual []VLine
	prefixW := 14

	for _, l := range lines {
		ts := l.timestamp.Format("15:04:05")
		prefix := fmt.Sprintf("[%s] ", ts)

		var pStyle, tStyle tcell.Style
		var fullText string

		switch l.kind {
		case KindChat:
			nick := "<" + l.nick + ">"
			if l.self {
				pStyle = styleTimestamp
				tStyle = styleNickSelf
			} else {
				pStyle = styleTimestamp
				tStyle = styleNick
			}

			fullText = nick + " " + l.text
			pStyle = styleTimestamp
			tStyle = styleDefault
			if l.self {
				tStyle = styleNickSelf
			} else {
				tStyle = styleNick
			}
			_ = fullText

			nickStr := "<" + l.nick + "> "
			bodyStyle := styleDefault
			availW := w - prefixW - utf8.RuneCountInString(nickStr)
			if availW < 1 {
				availW = 1
			}
			wrapped := wrapText(l.text, availW)
			for i, wl := range wrapped {
				if i == 0 {
					visual = append(visual, VLine{prefix + nickStr, pStyle, wl, bodyStyle})
					_ = tStyle
				} else {
					visual = append(visual, VLine{strings.Repeat(" ", prefixW+utf8.RuneCountInString(nickStr)), styleDefault, wl, bodyStyle})
				}
			}
			continue

		case KindSystem:
			pStyle = styleTimestamp
			tStyle = styleSystem
		case KindError:
			pStyle = styleTimestamp
			tStyle = styleError
		case KindJoin:
			pStyle = styleTimestamp
			tStyle = styleJoin
			l.text = fmt.Sprintf("-!- %s (%s) its online now", l.nick, l.text)
		case KindPart:
			pStyle = styleTimestamp
			tStyle = stylePart
			l.text = fmt.Sprintf("-!- %s (%s) its offline now", l.nick, l.text)
		case KindUpload:
			pStyle = styleTimestamp
			tStyle = styleSystem
			l.text = fmt.Sprintf("*upload* <%s> %s", l.nick, l.text)
		case KindExec:
			pStyle = styleTimestamp
			tStyle = styleSystem
			l.text = fmt.Sprintf("*exec* <%s> %s", l.nick, l.text)
		}

		availW := w - prefixW
		if availW < 1 {
			availW = 1
		}
		wrapped := wrapText(l.text, availW)
		for i, wl := range wrapped {
			if i == 0 {
				visual = append(visual, VLine{prefix, pStyle, wl, tStyle})
			} else {
				visual = append(visual, VLine{strings.Repeat(" ", prefixW), styleDefault, wl, tStyle})
			}
		}
	}

	// Aplica scroll
	total := len(visual)
	start := total - h - t.scrollOffset
	if start < 0 {
		start = 0
	}
	end := start + h
	if end > total {
		end = total
	}
	visible := visual[start:end]

	// Renderiza
	for row, vl := range visible {
		col := x
		for _, r := range vl.prefix {
			t.screen.SetContent(col, y+row, r, nil, vl.prefixStyle)
			col++
		}
		for _, r := range vl.text {
			if col >= w {
				break
			}
			t.screen.SetContent(col, y+row, r, nil, vl.textStyle)
			col++
		}
	}

	if t.scrollOffset > 0 {
		indicator := fmt.Sprintf(" ^ scroll (%d) ^ ", t.scrollOffset)
		drawText(t.screen, w-len(indicator)-1, y, styleSeparator, indicator)
	}
}

func (t *TUI) drawStatusBar(row, w int) {
	for col := 0; col < w; col++ {
		t.screen.SetContent(col, row, ' ', nil, styleStatusBar)
	}

	t.node.peersMu.RLock()
	pc := len(t.node.peers)
	t.node.peersMu.RUnlock()

	left := fmt.Sprintf(" [IceScream] %s@%s ", t.node.nickname, t.node.listenAddr)
	right := fmt.Sprintf(" peers: %d | PgUp/PgDn scroll ", pc)

	drawText(t.screen, 0, row, styleStatusBar, left)
	drawText(t.screen, w-len(right), row, styleStatusBar, right)
}

func (t *TUI) drawInputBar(row, w int) {
	for col := 0; col < w; col++ {
		t.screen.SetContent(col, row, ' ', nil, styleInputBar)
	}

	prompt := "> "
	drawText(t.screen, 0, row, styleSystem, prompt)

	inputX := len(prompt)
	runes := t.input
	visibleW := w - inputX - 1
	start := 0
	if t.inputPos >= visibleW {
		start = t.inputPos - visibleW + 1
	}
	for i, r := range runes[start:] {
		col := inputX + i
		if col >= w-1 {
			break
		}
		t.screen.SetContent(col, row, r, nil, styleInputBar)
	}
	cursorCol := inputX + (t.inputPos - start)
	if cursorCol < w {
		t.screen.ShowCursor(cursorCol, row)
	}
}

func drawText(s tcell.Screen, x, y int, style tcell.Style, text string) {
	for i, r := range text {
		s.SetContent(x+i, y, r, nil, style)
	}
}

func wrapText(text string, maxW int) []string {
	if maxW <= 0 {
		return []string{text}
	}
	var lines []string
	for _, rawLine := range strings.Split(text, "\n") {
		runes := []rune(rawLine)
		for len(runes) > maxW {
			cut := maxW
			for i := maxW; i > 0; i-- {
				if runes[i-1] == ' ' {
					cut = i
					break
				}
			}
			lines = append(lines, string(runes[:cut]))
			runes = runes[cut:]
		}
		lines = append(lines, string(runes))
	}
	return lines
}


func (n *Node) RunCLI() {
	tui, err := NewTUI(n)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start TUI: %v\n", err)
		os.Exit(1)
	}
	tui.Run()
}
