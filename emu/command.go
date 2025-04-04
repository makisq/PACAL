package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"
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

func hltCommand(cpu *CPUContext, _ []string) error {
	cpu.running = false
	fmt.Println("Процессор остановлен по команде HLT")
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
	printCommandHelp("memrange")

	fmt.Println("\nУправление потоком:")
	printCommandHelp("jmp")
	printCommandHelp("jz")
	printCommandHelp("jnz")
	printCommandHelp("jc")
	printCommandHelp("call")
	printCommandHelp("ret")
	printCommandHelp("cmp")
	printCommandHelp("irq")

	fmt.Println("\nОтладка:")
	printCommandHelp("regs")
	printCommandHelp("step")
	printCommandHelp("run")
	printCommandHelp("perf")

	fmt.Println("\nСохранения и загрузка файла состояния:")
	printCommandHelp("save")
	printCommandHelp("loadf")

	fmt.Println("\nПрочие:")
	printCommandHelp("help")
	printCommandHelp("hlt")
	printCommandHelp("mov")
	fmt.Println("exit - выход из эмулятора")

	return nil
}

func initCommands() {
	rawCommands := []Command{
		{
			Name:     "add",
			OpCode:   OpAdd,
			Operands: 2,
			Help:     "add <reg1> <reg2/imm/[mem]> - сложение (reg1 = reg1 + reg2 или imm или [mem])",
			Exec:     genALUCommand(OpAdd),
		},
		{
			Name:     "sub",
			OpCode:   OpSub,
			Operands: 2,
			Help:     "sub <reg1> <reg2/imm/[mem]> - вычитание (reg1 = reg1 - reg2 или imm или [mem])",
			Exec:     genALUCommand(OpSub),
		},
		{
			Name:     "mul",
			OpCode:   OpMul,
			Operands: 2,
			Help:     "mul <reg1> <reg2/imm/[mem]> - умножение (reg1 = reg1 * reg2 или imm или [mem])",
			Exec:     genALUCommand(OpMul),
		},
		{
			Name:     "and",
			OpCode:   OpAnd,
			Operands: 2,
			Help:     "and <reg1> <reg2/imm/[mem]> - логическое И (битовая операция)",
			Exec:     genALUCommand(OpAnd),
		},
		{
			Name:     "or",
			OpCode:   OpOr,
			Operands: 2,
			Help:     "or <reg1> <reg2/imm/[mem]> - логическое ИЛИ (битовая операция)",
			Exec:     genALUCommand(OpOr),
		},
		{
			Name:     "xor",
			OpCode:   OpXor,
			Operands: 2,
			Help:     "xor <reg1> <reg2/imm/[mem]> - исключающее ИЛИ (битовая операция)",
			Exec:     genALUCommand(OpXor),
		},
		{
			Name:     "cmp",
			OpCode:   OpCmp,
			Operands: 2,
			Help:     "cmp <reg1> <reg2/imm/[mem]> - сравнение (устанавливает флаги Z и C)",
			Exec:     genALUCommand(OpCmp),
		},
		{
			Name:     "mov",
			OpCode:   OpMov,
			Operands: 2,
			Help:     "mov <reg/[mem]> <reg/imm/[mem]> - запись значения (4 бита: 1010, 5, 0x5)",
			Exec:     movCommand,
		},
		{
			Name:     "load",
			OpCode:   OpLoad,
			Operands: 2,
			Help:     "load <reg> <[reg]/[imm]> - загрузка из памяти (reg = mem[addr])",
			Exec:     loadCommand,
		},
		{
			Name:     "store",
			OpCode:   OpStore,
			Operands: 2,
			Help:     "store <[reg]/[imm]> <reg> - запись в память (mem[addr] = reg)",
			Exec:     storeCommand,
		},
		{
			Name:     "jmp",
			OpCode:   OpJmp,
			Operands: 1,
			Help:     "jmp <reg/[imm]/label> - безусловный переход",
			Exec:     jmpCommand,
		},
		{
			Name:     "jz",
			OpCode:   OpJz,
			Operands: 1,
			Help:     "jz <reg/[imm]/label> - переход если Z=1 (результат был 0)",
			Exec:     jzCommand,
		},
		{
			Name:     "jnz",
			OpCode:   OpJnz,
			Operands: 1,
			Help:     "jnz <reg/[imm]/label> - переход если Z=0 (результат не 0)",
			Exec:     jnzCommand,
		},
		{
			Name:     "memrange",
			OpCode:   OpMemRange,
			Operands: 2,
			Help: "Просмотр содержимого диапазона памяти\n" +
				"Формат: memrange <начало> <конец>\n" +
				"Примеры:\n" +
				"  mem_range 0x0A 0x0F  # Просмотр адресов от 0A до 0F в hex\n" +
				"  mem_range 5 10      # Просмотр адресов от 5 до 10 в decimal",
			Exec: MemRanges,
		},

		{
			Name:     "jc",
			OpCode:   OpJc,
			Operands: 1,
			Help:     "jc <reg/[imm]/label> - переход если C=1 (было переполнение)",
			Exec:     jcCommand,
		},
		{
			Name:     "call",
			OpCode:   OpCall,
			Operands: 1,
			Help:     "call <reg/[imm]/label> - вызов подпрограммы (адрес возврата в R3)",
			Exec:     callCommand,
		},
		{
			Name:     "ret",
			OpCode:   OpRet,
			Operands: 0,
			Help:     "ret - возврат из подпрограммы (адрес из R3)",
			Exec:     retCommand,
		},
		{
			Name:     "irq",
			OpCode:   OpIRQ,
			Operands: 1,
			Help:     "irq <0-3> - инициировать прерывание",
			Exec:     irqCommand,
		},
		{
			Name:     "regs",
			OpCode:   -1,
			Operands: 0,
			Help:     "regs - показать состояние регистров и флагов",
			Exec:     regsCommand,
		},
		{
			Name:     "run",
			OpCode:   -1,
			Operands: 0,
			Help:     "run - запустить выполнение программы",
			Exec:     runCommand,
		},
		{
			Name:     "step",
			OpCode:   -1,
			Operands: 0,
			Help:     "step - выполнить одну инструкцию(временно недоступна в pipe. Доступна только в shell)",
			Exec:     stepCommand,
		},
		{
			Name:     "mem",
			OpCode:   -1,
			Operands: 1,
			Help:     "mem <[reg]/[imm]> - показать содержимое памяти по адресу",
			Exec:     memCommand,
		},
		{
			Name:     "save",
			OpCode:   -1,
			Operands: 1,
			Help:     "save <file> - сохранить состояние процессора в файл",
			Exec:     saveCommand,
		},
		{
			Name:     "loadf",
			OpCode:   -1,
			Operands: 1,
			Help:     "load <file> - загрузить состояние процессора из файла",
			Exec:     loadfCommand,
		},
		{
			Name:     "help",
			OpCode:   -1,
			Operands: 0,
			Help:     "help - показать эту справку",
			Exec:     helpCommand,
		},
		{
			Name:     "perf",
			OpCode:   -1,
			Operands: 0,
			Help:     "perf - показать статистику производительности(доступна только в shell режиме. Но обрабатывает запросы и pipe)",
			Exec:     perf,
		},
		{
			Name:     "hlt",
			OpCode:   OpHalt,
			Operands: 0,
			Help:     "hlt - остановка процессора",
			Exec:     hltCommand,
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

func showProgress(total, current int) {
	width := 50
	percent := float64(current) / float64(total)
	filled := int(float64(width) * percent)

	fmt.Printf("\r[%-*s] %3.0f%%",
		width,
		strings.Repeat("=", filled)+">",
		percent*100)
}

func movCommand(cpu *CPUContext, args []string) error {

	reg, err := parseAddress(args[1])
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
func executeJumpCommand(cpu *CPUContext, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("требуется 1 аргумент (регистр, адрес или метка)")
	}
	if strings.HasPrefix(args[0], "r") {
		reg, err := parseRegister(args[0])
		if err != nil {
			return fmt.Errorf("неверный аргумент: %s (ожидается регистр, адрес или метка)", args[0])
		}

		target := cpu.regFile.Read(reg)
		if len(target) < 4 {
			return fmt.Errorf("неверный адрес в регистре R%d", reg)
		}

		var newPC [4]bool
		copy(newPC[:], target[:4])
		cpu.pc.Write(newPC)
		return nil
	}
	if strings.HasPrefix(args[0], "@") {
		label := args[0][1:]
		if addr, ok := cpu.labels[label]; ok {
			cpu.pc.Write(addr)
			return nil
		}
		return fmt.Errorf("неизвестная метка: %s", label)
	}
	addr, err := parseAddress(args[1])
	cpu.pc.Write(intTo4Bits(addr))
	if err != nil {
		return fmt.Errorf("неверный адрес памяти: %v", err)
	}

	return fmt.Errorf("неверный аргумент: %s (ожидается регистр, адрес или метка)", args[0])
}

func jcCommand(cpu *CPUContext, args []string) error {
	if !cpu.alu.flags.carry {
		return nil
	}
	return executeJumpCommand(cpu, args)
}

func jzCommand(cpu *CPUContext, args []string) error {
	if !cpu.alu.flags.zero {
		return nil
	}
	return executeJumpCommand(cpu, args)
}

func storeCommand(cpu *CPUContext, args []string) error {
	addr, err := parseAddress(args[1])
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

	addr, err := parseAddress(args[1])
	if err != nil {
		return fmt.Errorf("неверный адрес в регистре R%d", addr)
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

func saveCommand(cpu *CPUContext, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("использование: save <filename>")
	}
	return cpu.SaveState(args[0])
}

func jnzCommand(cpu *CPUContext, args []string) error {
	if cpu.alu.flags.zero {
		return nil
	}
	return executeJumpCommand(cpu, args)
}

func loadfCommand(cpu *CPUContext, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("использование: load <filename>")
	}
	return cpu.LoadState(args[0])
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

func perf(cpu *CPUContext, _ []string) error {
	fmt.Printf("Последняя операция: %v\n", cpu.alu.lastOpTime)
	return nil
}

func memCommand(cpu *CPUContext, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("не указан адрес памяти\nИспользуйте: mem <адрес> или mem <начало> <конец>")
	}

	if len(args) == 1 {
		addr, err := parseAddress(args[0])
		if err != nil {
			return fmt.Errorf("неверный адрес памяти '%s': %v\nДопустимые форматы: [rX], 0x0F, 0b1010, 10", args[0], err)
		}
		if addr < 0 || addr > 15 {
			return fmt.Errorf("адрес должен быть от 0 до 15 (0x0 до 0xF)")
		}

		data := cpu.bus.Read(intTo4Bits(addr))
		fmt.Printf("Память по адресу %02X: %s\n", addr, bitsToStr(data))
		return nil
	}

	if len(args) == 2 {
		start, err1 := parseAddress(args[0])
		end, err2 := parseAddress(args[1])

		if err1 != nil || err2 != nil {
			var errMessages []string
			if err1 != nil {
				errMessages = append(errMessages, fmt.Sprintf("начальный адрес: %v", err1))
			}
			if err2 != nil {
				errMessages = append(errMessages, fmt.Sprintf("конечный адрес: %v", err2))
			}
			return fmt.Errorf("ошибки парсинга адресов:\n%s", strings.Join(errMessages, "\n"))
		}

		if start < 0 || start > 15 || end < 0 || end > 15 {
			return fmt.Errorf("адреса должны быть в диапазоне 0-15 (0x0-0xF)")
		}
		if start > end {
			return fmt.Errorf("неверный диапазон: начальный адрес (%02X) должен быть меньше конечного (%02X)", start, end)
		}
		fmt.Println(" Адрес | Бинарно | Дес. | Hex | Символ")
		fmt.Println("-------------------------------------")
		for addr := start; addr <= end; addr++ {
			data := cpu.bus.Read(intTo4Bits(addr))
			value := bitsToInt(data)
			char := " "
			if value >= 32 && value <= 126 {
				char = string(rune(value))
			}
			fmt.Printf(" 0x%02X | %04b    | %3d | %02X  | %s\n",
				addr, data, value, value, char)
		}
		return nil
	}

	return fmt.Errorf("неверное количество аргументов (%d)\nИспользуйте:\n  mem <адрес> - просмотр одного адреса\n  mem <начало> <конец> - просмотр диапазона", len(args))
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
func printCommandHelp(name string) {
	for _, cmd := range commands {
		if cmd.Name == name {
			fmt.Printf("%-10s %s\n", cmd.Name, cmd.Help)
			return
		}
	}
}

func parseRegister(arg string) (int, error) {
	if len(arg) != 2 {
		return -1, fmt.Errorf("неверный формат регистра: %s (должен быть r0-r3)", arg)
	}
	if arg[0] != 'r' {
		return -1, fmt.Errorf("неверный префикс регистра: %s (должен быть r0-r3)", arg)
	}
	regNum, err := strconv.Atoi(arg[1:])
	if err != nil {
		return -1, fmt.Errorf("неверный номер регистра: %s (должен быть 0-3)", arg)
	}
	if arg[0] > unicode.MaxASCII || arg[1] > unicode.MaxASCII {
		return -1, fmt.Errorf("неверные символы в регистре: %s (используйте ASCII)", arg)
	}
	if regNum < 0 || regNum > 3 {
		return -1, fmt.Errorf("неверный номер регистра: %s (должен быть 0-3)", arg)
	}

	return regNum, nil
}

func parseAddress(arg string) (int, error) {
	arg = strings.TrimSpace(arg)
	if strings.HasPrefix(arg, "[") && strings.HasSuffix(arg, "]") {
		regStr := arg[1 : len(arg)-1]
		reg, err := parseRegister(regStr)
		if err != nil {
			return -1, fmt.Errorf("неверный формат регистра в адресе [%s]: %v", regStr, err)
		}
		return reg, nil
	}
	if len(arg) > 2 && strings.HasPrefix(strings.ToUpper(arg), "0X") {
		arg = "0x" + arg[2:]
	}
	var num int64
	var err error

	switch {
	case strings.HasPrefix(arg, "0x"):
		num, err = strconv.ParseInt(arg[2:], 16, 8)
	case strings.HasPrefix(arg, "0b"):
		num, err = strconv.ParseInt(arg[2:], 2, 8)
	default:
		num, err = strconv.ParseInt(arg, 10, 8)
	}
	if strings.HasPrefix(arg, "r") {
		reg, err := parseRegister(arg)
		if err != nil {
			return -1, fmt.Errorf("неверный регистр: %s (должен быть r0-r3)", arg)
		}
		return reg, nil
	}

	if err != nil {
		return -1, fmt.Errorf("неверный аргумент: %s (допустимые форматы: r0-r3, 0-15, 0b0101, 0x0F, @label, hlt)", arg)
	}

	if num < 0 || num > 15 {
		return -1, fmt.Errorf("адрес должен быть от 0 до 15")
	}

	return int(num), nil
}
func CommandShell(cpu *CPUContext) {
	initCommands()
	cpu.terminal.cpu = cpu

	fmt.Println("Эмулятор процессора. Введите 'help' для списка команд")
	fmt.Println("Ctrl+Enter для переключения режимов")
	fmt.Println("В pipeline-режиме завершайте команды символом |")
	fmt.Println("Для переключения между режимами введите:")
	fmt.Println("  'shell'    - для интерактивного режима (гибкий ввод, десятичные числа)")
	fmt.Println("  'pipeline' или ':'/';' - для пакетного режима (строгий 4-битный формат, команды через |)")
	fmt.Println("  'exit'     - для выхода из эмулятора")
	for {
		modePrompt := map[int]string{
			0: "SHELL> ",
			1: "PIPE|> ",
		}[cpu.interfaceMode]
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
	commands := strings.Split(line, "|")
	for _, cmd := range commands {
		cmd = strings.TrimSpace(cmd)
		if cmd == "" {
			continue
		}
		if cmd == "run" {
			cpu.Run()
			continue
		}

		if strings.HasSuffix(cmd, ":") {
			if cpu.labels == nil {
				cpu.labels = make(map[string][4]bool)
			}
			label := strings.TrimSuffix(cmd, ":")
			addr := cpu.pc.Read()
			cpu.labels[label] = addr
			continue
		}

		if strings.HasPrefix(cmd, ";") {
			continue
		}

		parts := strings.Fields(cmd)
		if len(parts) == 0 {
			continue
		}

		instr, err := convertToInstruction(parts, cpu.labels)
		if err != nil {
			return err
		}
		if err := cpu.executeInstruction(instr); err != nil {
			return err
		}
	}
	return nil
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

func callCommand(cpu *CPUContext, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("требуется 1 аргумент (метка или регистр)")
	}
	currentPC := cpu.pc.Read()
	nextPC := increment4Bit(currentPC)
	cpu.regFile.Write(3, boolToByteSlice(nextPC[:]))
	if strings.HasPrefix(args[0], "r") {

		reg, err := parseAddress(args[1])
		if err != nil {
			return fmt.Errorf("неверный адрес памяти: %v", err)
		}
		target := cpu.regFile.Read(reg)
		var targetAddr [4]bool
		copy(targetAddr[:], target[:4])
		cpu.pc.Write(targetAddr)
	} else {
		if addr, ok := cpu.labels[args[0]]; ok {
			cpu.pc.Write(addr)
		} else {
			return fmt.Errorf("неизвестная метка: %s", args[0])
		}
	}

	return nil
}

func retCommand(cpu *CPUContext, _ []string) error {
	returnAddr := cpu.regFile.Read(3)
	var addr [4]bool
	copy(addr[:], returnAddr[:4])
	cpu.pc.Write(addr)
	return nil
}

func MemRanges(cpu *CPUContext, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("неверное количество аргументов (%d). Используйте:\n"+
			"  memrange <адрес>        - просмотр одного адреса\n"+
			"  memrange <начало> <конец> - просмотр диапазона", len(args))
	}
	start, err := parseAddress(args[0])
	if err != nil {
		return fmt.Errorf("ошибка начального адреса '%s': %v", args[0], err)
	}
	if start >= 0 && start <= 3 {
		regBits := cpu.regFile.Read(start)
		start = bitsToInt(boolSliceTo4Bits(regBits))
	}

	end, err := parseAddress(args[1])
	if err != nil {
		return fmt.Errorf("ошибка конечного адреса '%s': %v", args[1], err)
	}

	if end >= 0 && end <= 3 {
		regBits := cpu.regFile.Read(end)
		end = bitsToInt(boolSliceTo4Bits(regBits))
	}

	if start < 0 || start > 15 || end < 0 || end > 15 {
		return fmt.Errorf("адреса должны быть в диапазоне 0-15 (0x0-0xF)\n"+
			"Получено: начало=0x%X, конец=0x%X", start, end)
	}

	if start > end {
		return fmt.Errorf("неверный диапазон: начальный адрес (0x%X) должен быть меньше конечного (0x%X)", start, end)
	}

	fmt.Println(" Адрес | Бинарно | Дес. | Hex | Символ")
	fmt.Println("-------------------------------------")

	for addr := start; addr <= end; addr++ {
		data := cpu.bus.Read(intTo4Bits(addr))
		value := bitsToInt(data)
		char := " "
		if value >= 32 && value <= 126 {
			char = string(rune(value))
		}

		fmt.Printf(" 0x%02X | %04b    | %3d | %02X  | %s\n",
			addr, data, value, value, char)
	}

	return nil
}
