package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

type Terminal struct {
	ScreenBuffer    [80 * 25]byte
	CursorPos       int
	KeyBuffer       []byte
	InteractiveMode bool
	spinnerActive   bool
	InputMode       bool
	Echo            bool
	ControlChars    map[byte]func(*Terminal)
	bufferMutex     sync.Mutex
	cpu             *CPUContext
}

func (t *Terminal) WriteRune(r rune) {
	t.bufferMutex.Lock()
	defer t.bufferMutex.Unlock()
	if t.CursorPos >= len(t.ScreenBuffer) {
		t.scrollScreenUnsafe()
	}
	t.ScreenBuffer[t.CursorPos] = byte(r)
	t.CursorPos++
	if t.Echo {
		fmt.Printf("%c", r)
	}
}

func NewTerminal() *Terminal {
	term := &Terminal{
		ControlChars: map[byte]func(*Terminal){
			0x03: func(t *Terminal) {
				t.WriteChar('\n')
				fmt.Println("[Ctrl+C] Прерывание")
				os.Exit(0)
			},
			0x0D: func(t *Terminal) {
				t.WriteChar('\n')
			},
			0x7F: func(t *Terminal) {
				t.handleBackspace()
			},
		},
		Echo:      true,
		InputMode: true,
	}
	term.ClearScreen()
	go term.CaptureInput()
	return term
}

func (t *Terminal) handleBackspace() {
	t.bufferMutex.Lock()
	defer t.bufferMutex.Unlock()

	if t.CursorPos > 0 {
		t.CursorPos--
		t.ScreenBuffer[t.CursorPos] = ' '
		if t.Echo {
			fmt.Print("\b \b")
		}
	}
}

func (t *Terminal) Read(addr [4]bool) [4]bool {
	addrInt := bitsToInt(addr)
	t.bufferMutex.Lock()
	defer t.bufferMutex.Unlock()

	switch addrInt {
	case 0x00:
		return byteTo4Bits(t.readKeyUnsafe())
	case 0x01:
		hasData := len(t.KeyBuffer) > 0
		return [4]bool{hasData, t.InputMode, false, false}
	case 0x02:
		return [4]bool{t.Echo, false, false, false}
	default:
		return [4]bool{false, false, false, false}
	}
}

func (t *Terminal) readKeyUnsafe() byte {
	if len(t.KeyBuffer) == 0 {
		return 0
	}
	key := t.KeyBuffer[0]
	t.KeyBuffer = t.KeyBuffer[1:]
	return key
}

func (cpu *CPUContext) ExecuteMultiLine(code string) error {
	lines := strings.Split(code, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, ";") {
			continue
		}
		if err := cpu.executeCommand(line); err != nil {
			return err
		}
	}
	return nil
}

func (t *Terminal) Write(addr [4]bool, data [4]bool, clock bool) {
	if !clock {
		return
	}

	addrInt := bitsToInt(addr)
	value := bitsToByte(data)
	t.bufferMutex.Lock()
	defer t.bufferMutex.Unlock()

	switch addrInt {
	case 0x00:
		t.writeCharUnsafe(value)
	case 0x02:
		t.Echo = data[0]
		t.InputMode = data[1]
	}
}

func (t *Terminal) writeCharUnsafe(c byte) {
	if handler, ok := t.ControlChars[c]; ok {
		handler(t)
		return
	}

	if t.CursorPos >= len(t.ScreenBuffer) {
		t.scrollScreenUnsafe()
	}

	t.ScreenBuffer[t.CursorPos] = c
	t.CursorPos++

	if t.Echo {
		fmt.Printf("%c", c)
	}
}

func (t *Terminal) WriteChar(c byte) {
	t.bufferMutex.Lock()
	defer t.bufferMutex.Unlock()
	t.writeCharUnsafe(c)
}

func (t *Terminal) scrollScreenUnsafe() {
	copy(t.ScreenBuffer[:], t.ScreenBuffer[80:])
	for i := len(t.ScreenBuffer) - 80; i < len(t.ScreenBuffer); i++ {
		t.ScreenBuffer[i] = ' '
	}
	t.CursorPos -= 80
}

func (t *Terminal) ClearScreen() {
	t.bufferMutex.Lock()
	defer t.bufferMutex.Unlock()
	for i := range t.ScreenBuffer {
		t.ScreenBuffer[i] = ' '
	}
	t.CursorPos = 0
	fmt.Print("\033[H\033[2J")
}

func (t *Terminal) Render() {
	t.bufferMutex.Lock()
	defer t.bufferMutex.Unlock()

	fmt.Print("\033[H\033[2J")
	for y := 0; y < 25; y++ {
		for x := 0; x < 80; x++ {
			pos := y*80 + x
			fmt.Printf("%c", t.ScreenBuffer[pos])
		}
		fmt.Println()
	}
}

func (t *Terminal) CaptureInput() {
	reader := bufio.NewReader(os.Stdin)
	for {
		char, _, err := reader.ReadRune()
		if err != nil {
			continue
		}

		t.bufferMutex.Lock()
		if char == '\x0A' && len(t.KeyBuffer) > 0 && t.KeyBuffer[len(t.KeyBuffer)-1] == '\x1A' {
			t.cpu.SwitchInterfaceMode()
			t.KeyBuffer = t.KeyBuffer[:len(t.KeyBuffer)-1]
			t.bufferMutex.Unlock()
			continue
		}

		t.KeyBuffer = append(t.KeyBuffer, byte(char))
		t.bufferMutex.Unlock()
	}
}

func (t *Terminal) RunWithSpinner(fn func()) {
	if !t.InteractiveMode {
		fn()
		return
	}

	t.spinnerActive = true
	done := make(chan struct{})
	go func() {
		t.showSpinner(done)
		t.spinnerActive = false
	}()
	fn()
	close(done)
	time.Sleep(150 * time.Millisecond)
	t.ClearLine()
}

func (t *Terminal) ClearLine() {
	fmt.Print("\r\033[K")
}

func (t *Terminal) CursorBack() {
	fmt.Print("\b")
}

func (t *Terminal) showSpinner(done <-chan struct{}) {
	frames := []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"}
	i := 0
	for {
		select {
		case <-done:
			return
		default:
			fmt.Printf("\r%s Обработка...", frames[i%len(frames)])
			time.Sleep(100 * time.Millisecond)
			i++
		}
	}
}

func (cpu *CPUContext) setInterfaceMode(mode int) {
	cpu.interfaceMode = mode
	fmt.Printf("\nРежим переключен на: %s\n", map[int]string{0: "Shell", 1: "Pipeline"}[mode])
}
