package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"unsafe"

	"github.com/c-bata/go-prompt"
)

type Terminal struct {
	originalState syscall.Termios
}

var commandHistory []string

func (t *Terminal) Save(fd int) error {
	var termios syscall.Termios
	_, _, err := syscall.Syscall6(
		syscall.SYS_IOCTL,
		uintptr(fd),
		uintptr(syscall.TCGETS),
		uintptr(unsafe.Pointer(&termios)),
		0, 0, 0)
	if err != 0 {
		return fmt.Errorf("failed to save terminal state: %v", err)
	}
	t.originalState = termios
	return nil
}

func (t *Terminal) Restore(fd int) error {
	fmt.Print("\x1b[0m\r")

	_, _, err := syscall.Syscall6(
		syscall.SYS_IOCTL,
		uintptr(fd),
		uintptr(syscall.TCSETS),
		uintptr(unsafe.Pointer(&t.originalState)),
		0, 0, 0)
	if err != 0 {
		return fmt.Errorf("failed to restore terminal: %v", err)
	}

	cmd := exec.Command("stty", "sane")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	_ = cmd.Run()

	fmt.Print("\x1b[0m\x1b[?25h")
	return nil
}

func (t *Terminal) MakeRaw(fd int) error {
	termios := t.originalState
	termios.Lflag &^= syscall.ECHO | syscall.ICANON | syscall.ISIG
	termios.Cc[syscall.VMIN] = 1
	termios.Cc[syscall.VTIME] = 0

	_, _, err := syscall.Syscall6(
		syscall.SYS_IOCTL,
		uintptr(fd),
		uintptr(syscall.TCSETS),
		uintptr(unsafe.Pointer(&termios)),
		0, 0, 0)
	if err != 0 {
		return fmt.Errorf("failed to set raw mode: %v", err)
	}
	return nil
}

func StartInteractiveShell() {
	term := &Terminal{}
	fd := int(os.Stdin.Fd())

	if err := term.Save(fd); err != nil {
		fmt.Printf("❌ Failed to save terminal: %v\n", err)
		return
	}

	defer func() {
		if err := term.Restore(fd); err != nil {
			fmt.Printf("❌ Failed to restore terminal: %v\n", err)
		}
	}()

	if err := term.MakeRaw(fd); err != nil {
		fmt.Printf("❌ Failed to set raw mode: %v\n", err)
		return
	}

	cleanExit := func() {
		fmt.Print("\r\x1b[0m")
		term.Restore(fd)
		os.Exit(0)
	}

	executor := func(in string) {
		in = strings.TrimSpace(in)
		if in == "" {
			return
		}
		if !strings.HasPrefix(in, "!") {
			commandHistory = append(commandHistory, in)
		}

		switch in {
		case "exit", "quit":
			fmt.Println("\rGoodbye! 👋")
			cleanExit()
			return
		case "!clear", "clr":
			fmt.Print("\x1b[2J\x1b[H")
			return
		case "!history", "his":
			showCommandHistory()
			return
		}

		args := strings.Fields(in)
		if foundCmd, _, err := rootCmd.Find(args); err == nil {
			fmt.Printf("\rExecuting: %s\n", foundCmd.Name())
			rootCmd.SetArgs(args)
			if err := rootCmd.Execute(); err != nil {
				fmt.Printf("\rError: %v\n", err)
			}
			return
		}

		fmt.Printf("\rCommand not found: %s\n", in)
	}

	p := prompt.New(
		executor,
		completer,
		prompt.OptionPrefix("\rochan> "),
		prompt.OptionAddKeyBind(
			prompt.KeyBind{
				Key: prompt.ControlC,
				Fn: func(*prompt.Buffer) {
					fmt.Print("\r")
					cleanExit()
				},
			},
			prompt.KeyBind{
				Key: prompt.ControlD,
				Fn: func(*prompt.Buffer) {
					fmt.Print("\r")
					cleanExit()
				},
			},
		),
		prompt.OptionTitle("OctoChan Interactive Shell"),
	)

	fmt.Println("\r🐙 Welcome to OctoChan! Type 'help' for commands")
	p.Run()
}

func showCommandHistory() {
	if len(commandHistory) == 0 {
		fmt.Println("\rNo commands in history")
		return
	}

	fmt.Println("\rCommand history:")
	for i, cmd := range commandHistory {
		if i < 9 {
			fmt.Printf(" %d. %s\n", i+1, cmd)
		} else {
			fmt.Printf("%d. %s\n", i+1, cmd)
		}
	}
} 
