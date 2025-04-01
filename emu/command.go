package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Command struct {
	Name     string
	OpCode   int
	Operands int
	Help     string
	Exec     func(cpu *CPUContext, args []string) error
}

var commands []Command

func initCommands() {
	commands = []Command{
		{
			Name:     "add",
			OpCode:   OpAdd,
			Operands: 2,
			Help:     "add <reg1> <reg2> - сложение регистров (reg1 = reg1 + reg2)",
			Exec:     genALUCommand(OpAdd),
		},
		{
			Name:     "sub",
			OpCode:   OpSub,
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
	}
}

func genALUCommand(op int) func(cpu *CPUContext, args []string) error {
	return func(cpu *CPUContext, args []string) error {
		if len(args) != 2 {
			return fmt.Errorf("требуется 2 аргумента")
		}

		reg1, err := parseRegister(args[0])
		if err != nil {
			return fmt.Errorf("неверный регистр-назначение: %v", err)
		}

		reg2, err := parseRegister(args[1])
		if err != nil {
			return fmt.Errorf("неверный регистр-источник: %v", err)
		}

		val1 := cpu.regFile.Read(reg1)
		val2 := cpu.regFile.Read(reg2)

		cpu.alu.aBits = val1
		cpu.alu.bBits = val2
		cpu.alu.op = op

		result, _ := cpu.alu.Execute()

		var resultBits [4]bool
		for i := 0; i < 4; i++ {
			resultBits[i] = result[i] == 1
		}
		cpu.regFile.Write(reg1, boolSliceToBytes(resultBits[:]))

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

	instr := encodeInstruction(OpJmp, addr, 0)
	execute(cpu.bus, cpu.regFile, cpu.alu, cpu.pc, instr)
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

func parseRegister(arg string) (int, error) {
	reg, err := strconv.Atoi(arg)
	if err != nil || reg < 0 || reg > 3 {
		return 0, fmt.Errorf("ожидается номер регистра (0-3)")
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

func encodeInstruction(op, reg1, reg2 int) [4]bool {
	return [4]bool{
		(op>>3)&1 == 1,
		(op>>2)&1 == 1,
		(op>>1)&1 == 1,
		op&1 == 1,
	}
}

func CommandShell(cpu *CPUContext) {
	initCommands()

	reader := bufio.NewReader(os.Stdin)
	fmt.Println("Эмулятор процессора. Введите 'help' для списка команд")

	for {
		fmt.Print("> ")
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		if input == "exit" {
			fmt.Println("Выход из эмулятора")
			return
		}

		parts := strings.Split(input, " ")
		cmdFound := false

		for _, cmd := range commands {
			if cmd.Name == parts[0] {
				cmdFound = true
				if len(parts[1:]) != cmd.Operands {
					fmt.Printf("Ошибка: команда '%s' требует %d аргументов\n", cmd.Name, cmd.Operands)
					break
				}

				if err := cmd.Exec(cpu, parts[1:]); err != nil {
					fmt.Printf("Ошибка выполнения: %v\n", err)
				}
				break
			}
		}

		if !cmdFound {
			fmt.Println("Неизвестная команда. Введите 'help' для списка команд")
		}
	}
}
