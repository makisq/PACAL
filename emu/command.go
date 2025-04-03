package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Command struct {
	Name     string
	OpCode   int
	Operands int
	Help     string
	Exec     func(cpu *CPUContext, args []string) error
	cpu      *CPUContext
}

var commands []Command

func initCommands() {
	rawCommands := []Command{
		{
			Name:     "add",
			OpCode:   OpAdd,
			Operands: 2,
			Help:     "add <reg1> <reg2> - сложение регистров (reg1 = reg1 + reg2)",
			Exec:     genALUCommand(OpAdd),
		},
		{
			Name:   "sub",
			OpCode: OpSub,

			Operands: 2,
			Help:     "sub <reg1> <reg2> - вычитание регистров (reg1 = reg1 - reg2)",
			Exec:     genALUCommand(OpSub),
		},
		{
			Name:     "mul",
			OpCode:   OpMul,
			Operands: 2,
			Help:     "mul <reg1> <reg2> - умножение регистров",
			Exec:     genALUCommand(OpMul),
		},

		{
			Name:     "and",
			OpCode:   OpAnd,
			Operands: 2,
			Help:     "and <reg1> <reg2> - логическое И",
			Exec:     genALUCommand(OpAnd),
		},
		{
			Name:     "or",
			OpCode:   OpOr,
			Operands: 2,
			Help:     "or <reg1> <reg2> - логическое ИЛИ",
			Exec:     genALUCommand(OpOr),
		},
		{
			Name:     "xor",
			OpCode:   OpXor,
			Operands: 2,
			Help:     "xor <reg1> <reg2> - исключающее ИЛИ",
			Exec:     genALUCommand(OpXor),
		},

		{
			Name:     "cmp",
			OpCode:   OpCmp,
			Operands: 2,
			Help:     "cmp <reg1> <reg2> - сравнение регистров (устанавливает флаги)",
			Exec:     genALUCommand(OpCmp),
		},
		{
			Name:     "mov",
			OpCode:   OpMov,
			Operands: 2,
			Help:     "mov <reg> <value> - запись значения в регистр (4 бита, напр. 1010)",
			Exec:     movCommand,
		},

		{
			Name:     "load",
			OpCode:   OpLoad,
			Operands: 2,
			Help:     "load <reg> <addr> - загрузка из памяти",
			Exec:     loadCommand,
		},
		{
			Name:     "store",
			OpCode:   OpStore,
			Operands: 2,
			Help:     "store <addr> <reg> - запись в память",
			Exec:     storeCommand,
		},

		{
			Name:     "jmp",
			OpCode:   OpJmp,
			Operands: 1,
			Help:     "jmp <addr> - безусловный переход",
			Exec:     jmpCommand,
		},
		{
			Name:     "irq",
			OpCode:   OpIRQ,
			Operands: 1,
			Help:     "irq <num> - инициировать прерывание (0-3)",
			Exec:     irqCommand,
		},

		{
			Name:     "regs",
			OpCode:   -1,
			Operands: 0,
			Help:     "regs - показать регистры и флаги",
			Exec:     regsCommand,
		},
		{
			Name:     "run",
			OpCode:   -1,
			Operands: 0,
			Help:     "run - запустить выполнение",
			Exec:     runCommand,
		},
		{
			Name:     "step",
			OpCode:   -1,
			Operands: 0,
			Help:     "step - выполнить шаг",
			Exec:     stepCommand,
		},
		{
			Name:     "mem",
			OpCode:   -1,
			Operands: 1,
			Help:     "mem <addr> - показать память",
			Exec:     memCommand,
		},
		{
			Name:     "help",
			OpCode:   -1,
			Operands: 0,
			Help:     "help - справка",
			Exec:     helpCommand,
		},
		{
			Name: "perf",
			Help: "Показать статистику производительности",
			Exec: func(cpu *CPUContext, _ []string) error {
				fmt.Printf("Последняя операция: %v\n", cpu.alu.lastOpTime)
				return nil
			},
		},
	}
	for _, cmd := range rawCommands {
		commands = append(commands, wrapWithSpinner(cmd))
	}
}

func genALUCommand(op int) func(cpu *CPUContext, args []string) error {
	return func(cpu *CPUContext, args []string) error {
		if len(args) != 2 {
			return fmt.Errorf("требуется 2 аргумента")
		}
		reg1, err := strconv.Atoi(args[0])
		if err != nil || reg1 < 0 || reg1 > 3 {
			return fmt.Errorf("неверный регистр-назначение (0-3)")
		}

		reg2, err := strconv.Atoi(args[1])
		if err != nil || reg2 < 0 || reg2 > 3 {
			return fmt.Errorf("неверный регистр-источник (0-3)")
		}

		val1 := cpu.regFile.Read(reg1)
		val2 := cpu.regFile.Read(reg2)

		if len(val1) < 4 || len(val2) < 4 {
			return fmt.Errorf("ошибка чтения регистров")
		}

		cpu.alu.aBits = make([]bool, 4)
		cpu.alu.bBits = make([]bool, 4)
		copy(cpu.alu.aBits, val1[:4])
		copy(cpu.alu.bBits, val2[:4])
		cpu.alu.op = op
		result, flags, err := cpu.alu.Execute()
		if err != nil {
			return fmt.Errorf("ошибка выполнения операции: %v", err)
		}

		if len(result) != 4 {
			return fmt.Errorf("неверный размер результата")
		}
		cpu.regFile.Write(reg1, boolToByteSlice(result[:]))
		cpu.alu.flags = flags

		return nil
	}
}

func movCommand(cpu *CPUContext, args []string) error {
	reg, err := parseRegister(args[0])
	if err != nil {
		return fmt.Errorf("неверный номер регистра: %v", err)
	}

	value, err := strconv.ParseInt(args[1], 2, 8)
	if err != nil || value < 0 || value > 15 {
		return fmt.Errorf("неверное значение: %s (должно быть 4-битное двоичное)", args[1])
	}

	data := [4]byte{
		byte((value >> 3) & 1),
		byte((value >> 2) & 1),
		byte((value >> 1) & 1),
		byte(value & 1),
	}
	cpu.regFile.Write(reg, data[:])
	return nil
}

func irqCommand(cpu *CPUContext, args []string) error {
	irqNum, err := strconv.Atoi(args[0])
	if err != nil || irqNum < 0 || irqNum > 3 {
		return fmt.Errorf("неверный номер прерывания: %s (должен быть 0-3)", args[0])
	}

	cpu.alu.interrupts.irqStatus[irqNum] = true
	return nil
}

func loadCommand(cpu *CPUContext, args []string) error {
	reg, err := parseRegister(args[0])
	if err != nil {
		return fmt.Errorf("неверный номер регистра: %v", err)
	}

	addr, err := parseAddress(args[1])
	if err != nil {
		return fmt.Errorf("неверный адрес памяти: %v", err)
	}

	cpu.pipeline.executeStage = PipelineStage{
		op:    OpLoad,
		reg1:  reg,
		reg2:  addr,
		valid: true,
	}
	cpu.handleLoad()
	return nil
}

func storeCommand(cpu *CPUContext, args []string) error {
	addr, err := parseAddress(args[0])
	if err != nil {
		return fmt.Errorf("неверный адрес памяти: %v", err)
	}

	reg, err := parseRegister(args[1])
	if err != nil {
		return fmt.Errorf("неверный номер регистра: %v", err)
	}

	cpu.pipeline.executeStage = PipelineStage{
		op:    OpStore,
		reg1:  addr,
		reg2:  reg,
		valid: true,
	}
	cpu.handleStore()
	return nil
}

func jmpCommand(cpu *CPUContext, args []string) error {
	addr, err := parseAddress(args[0])
	if err != nil {
		return fmt.Errorf("неверный адрес перехода: %v", err)
	}
	//cpu.mu.Lock()
	//defer cpu.mu.Unlock()
	target := cpu.regFile.Read(addr)
	if len(target) < 4 {
		return fmt.Errorf("неверный адрес в регистре R%d", addr)
	}
	var newPC [4]bool
	copy(newPC[:], target[:4])
	cpu.pc.Write(newPC)

	return nil
}

func regsCommand(cpu *CPUContext, _ []string) error {
	fmt.Println("\nСостояние процессора:")
	fmt.Println("Регистры:")
	for i := 0; i < 4; i++ {
		reg := cpu.regFile.Read(i)
		fmt.Printf("R%d: %s\n", i, bitsToStr(reg))
	}

	fmt.Println("\nФлаги:")
	fmt.Printf("Z (Zero): %v\n", cpu.alu.flags.zero)
	fmt.Printf("C (Carry): %v\n", cpu.alu.flags.carry)

	pcValue := cpu.pc.Read()
	fmt.Printf("\nPC (Program Counter): %s\n", bitsToStr(pcValue))
	return nil
}

func memCommand(cpu *CPUContext, args []string) error {
	addr, err := parseAddress(args[0])
	if err != nil {
		return fmt.Errorf("неверный адрес памяти: %v", err)
	}

	data := cpu.bus.Read(intTo4Bits(addr))
	fmt.Printf("Память по адресу %02X: %s\n", addr, bitsToStr(data))
	return nil
}

func runCommand(cpu *CPUContext, _ []string) error {
	if !cpu.running {
		cpu.running = true
		go cpu.Run()
		fmt.Println("Выполнение программы запущено")
	} else {
		fmt.Println("Программа уже выполняется")
	}
	return nil
}

func stepCommand(cpu *CPUContext, _ []string) error {
	cpu.clock <- true
	fmt.Println("Выполнена одна инструкция")
	return nil
}

func helpCommand(cpu *CPUContext, _ []string) error {
	fmt.Println("\nДоступные команды:")
	fmt.Println("-----------------")

	fmt.Println("\nАрифметические операции:")
	printCommandHelp("add")
	printCommandHelp("sub")
	printCommandHelp("mul")

	fmt.Println("\nЛогические операции:")
	printCommandHelp("and")
	printCommandHelp("or")
	printCommandHelp("xor")

	fmt.Println("\nОперации с памятью:")
	printCommandHelp("load")
	printCommandHelp("store")
	printCommandHelp("mem")

	fmt.Println("\nУправление потоком:")
	printCommandHelp("jmp")
	printCommandHelp("cmp")
	printCommandHelp("irq")

	fmt.Println("\nОтладка:")
	printCommandHelp("regs")
	printCommandHelp("step")
	printCommandHelp("run")

	fmt.Println("\nПрочие:")
	printCommandHelp("help")
	fmt.Println("exit - выход из эмулятора")

	return nil
}
func printCommandHelp(name string) {
	for _, cmd := range commands {
		if cmd.Name == name {
			fmt.Printf("%-10s %s\n", cmd.Name, cmd.Help)
			return
		}
	}
}

func parseRegister(regStr string) (int, error) {
	reg, err := strconv.Atoi(regStr)
	if err != nil || reg < 0 || reg > 3 {
		return 0, fmt.Errorf("неверный регистр: %s (должен быть 0-3)", regStr)
	}
	return reg, nil
}

func parseAddress(arg string) (int, error) {
	addr, err := strconv.Atoi(arg)
	if err != nil {
		return 0, fmt.Errorf("ожидается числовой адрес")
	}

	if addr < 0 || addr > 15 {
		return 0, fmt.Errorf("адрес вне диапазона (0-15)")
	}
	return addr, nil
}

func showProgress(total, current int) {
	width := 50
	percent := float64(current) / float64(total)
	filled := int(float64(width) * percent)

	fmt.Printf("\r[%-*s] %3.0f%%",
		width,
		strings.Repeat("=", filled)+">",
		percent*100)
}

func CommandShell(cpu *CPUContext) {
	initCommands()
	cpu.terminal.cpu = cpu

	fmt.Println("Эмулятор процессора. Введите 'help' для списка команд")
	fmt.Println("Ctrl+Enter для переключения режимов")

	for {
		modePrompt := map[int]string{0: "SHELL> ", 1: "PIPE> "}[cpu.interfaceMode]
		fmt.Print(modePrompt)

		input, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		input = strings.TrimSpace(input)

		if input == "exit" {
			fmt.Println("Выход из эмулятора")
			return
		}

		if err := cpu.executeCommand(input); err != nil {
			fmt.Printf("Ошибка: %v\n", err)
		}
	}
}
func (cpu *CPUContext) pipelineExecuteCommand(line string) error {
	if i := strings.Index(line, ";"); i >= 0 {
		line = line[:i]
	}
	line = strings.TrimSpace(line)

	if line == "" {
		return nil
	}
	if strings.HasSuffix(line, ":") {
		label := strings.TrimSuffix(line, ":")
		addr := cpu.pc.Read()
		cpu.labels[label] = addr
		return nil
	}

	parts := strings.Fields(line)
	if len(parts) == 0 {
		return nil
	}

	instr, err := convertToInstruction(parts)
	if err != nil {
		return err
	}

	return cpu.executeInstruction(instr)
}

func (cpu *CPUContext) shellExecuteCommand(line string) error {
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return nil
	}

	cmdFound := false
	for _, cmd := range commands {
		if cmd.Name == parts[0] {
			cmdFound = true
			if len(parts[1:]) != cmd.Operands {
				return fmt.Errorf("команда '%s' требует %d аргументов", cmd.Name, cmd.Operands)
			}
			return cmd.Exec(cpu, parts[1:])
		}
	}

	if !cmdFound {
		return fmt.Errorf("неизвестная команда: %s", parts[0])
	}
	return nil
}

func (cpu *CPUContext) executeWithSpinner(fn func()) {
	if cpu.terminal == nil {
		fn()
		return
	}

	frames := []rune{'⣾', '⣽', '⣻', '⢿', '⡿', '⣟', '⣯', '⣷'}
	done := make(chan struct{})

	go func() {
		i := 0
		for {
			select {
			case <-done:
				cpu.terminal.WriteRune('\r')
				return
			default:
				cpu.terminal.WriteRune(frames[i%len(frames)])
				time.Sleep(100 * time.Millisecond)
				cpu.terminal.WriteRune('\b')
				i++
			}
		}
	}()

	fn()
	close(done)
}

func wrapWithSpinner(cmd Command) Command {
	return Command{
		Name:     cmd.Name,
		OpCode:   cmd.OpCode,
		Operands: cmd.Operands,
		Help:     cmd.Help,
		Exec: func(cpu *CPUContext, args []string) error {
			if cpu.terminal != nil && cpu.terminal.InputMode {
				var err error
				cpu.executeWithSpinner(func() {
					err = cmd.Exec(cpu, args)
				})
				return err
			}
			return cmd.Exec(cpu, args)
		},
	}
}
