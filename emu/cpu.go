package main

// allowManual = true - только строгий разбор 4-битных строк ("1010")
// allowManual = false - поддерживает все форматы (числа, hex, binary)
import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"

	"syscall"

	"time"
)

const (
	OpAdd = iota
	OpSub
	OpMul
	OpHalt = iota + 18
	OpAnd
	OpOr
	OpXor
	OpMov
	OpCmp
	OpLoad
	OpStore
	OpJmp
	OpRet
	OpJz
	OpJnz
	OpJc
	OpMem = -3
	OpIRQ
	OpMemRange = -6
	OpIRET
	OpEI
	OpDI
	OpCode = iota
	OpCall = iota + 19
)

var mus sync.RWMutex
var pcCounter uint32

type DFlipFlop struct {
	data bool

	clock bool

	reset bool

	out bool
}

type parsedArg struct {
	isReg  bool
	value  int
	isImm  bool
	bits   [4]bool
	isHalt bool
}

type Register4bit struct {
	triggers     [4]DFlipFlop
	we           bool
	floating     bool
	dirty        [4]bool
	cacheEnabled bool
}

type RegisterFile struct {
	registers [4]Register4bit
	cache     [4][4]bool
	dirty     [4]bool
	//mu           sync.RWMutex
	cacheEnabled bool
}

type PipelineStage struct {
	valid bool

	done bool

	op int

	reg1 int

	reg2 int

	value [4]bool

	rawInstr [4]bool
}

type Pipeline struct {
	fetchStage [4]bool

	decodeStage PipelineStage

	executeStage PipelineStage
}

type CPUState struct {
	Registers [4][4]bool
	PC        [4]bool
	Memory    [16][4]bool
	Labels    map[string][4]bool
	CallStack [][4]bool
	Flags     struct {
		Zero  bool
		Carry bool
	}
	Interrupts InterruptState
}

type InterruptState struct {
	irqMask    [4]bool
	irqStatus  [4]bool
	enabled    bool
	inHandler  bool
	savedPC    [4]bool
	savedFlags struct {
		carry bool
		zero  bool
	}
}

type CPUContext struct {
	bus      *MemoryBus
	regFile  *RegisterFile
	terminal *Terminal
	//mu       sync.Mutex
	labels        map[string][4]bool
	program       []string
	alu           *ALU
	pc            *PCRegister
	callStack     [][4]bool
	pipeline      Pipeline
	pcLine        int
	interfaceMode int // 0 = Shell, 1 = Pipeline
	stopChan      chan os.Signal
	clock         chan bool
	running       bool
	logger        *log.Logger
	com           *Command
}

func (ctx *CPUContext) DumpState() {
	fmt.Println("=== CPU State Dump ===")
	fmt.Printf("PC: %v\n", ctx.pc.Read())
	fmt.Println("Registers:")
	for i := 0; i < 4; i++ {
		fmt.Printf("R%d: %v\n", i, ctx.regFile.Read(i))
	}
	fmt.Println("=====================")
}

func (p *Pipeline) Tick(bus *MemoryBus, pc *PCRegister, alu *ALU) {
	if !alu.interrupts.inHandler && alu.CheckInterrupts() >= 0 {
		return
	}

	if p.fetchStage != [4]bool{false, false, false, false} {
		PipelineStages("FETCH", p.fetchStage)
	}

	if p.executeStage.done {
		PipelineStages("EXECUTE_DONE", p.executeStage)
		p.executeStage = PipelineStage{}
	}

	if p.decodeStage.valid {
		PipelineStages("DECODE_TO_EXECUTE", p.decodeStage)
		p.executeStage = p.decodeStage
		p.executeStage.done = true
		p.decodeStage = PipelineStage{}
	}

	if p.fetchStage != [4]bool{false, false, false, false} {
		op, reg1, reg2 := decodeInstruction(p.fetchStage)
		newStage := PipelineStage{
			valid:    true,
			op:       op,
			reg1:     reg1,
			reg2:     reg2,
			rawInstr: p.fetchStage,
		}
		PipelineStages("FETCH_TO_DECODE", newStage)
		p.decodeStage = newStage
	}

	pcValue := pc.Read()
	if pcInt := bitsToInt(pcValue); pcInt < 16 {
		newFetch := bus.Read(pcValue)
		if newFetch != p.fetchStage {
			p.fetchStage = newFetch
		}
	} else {
		p.fetchStage = [4]bool{false, false, false, false}
	}
}

func bitsToInt(bits [4]bool) int {
	val := 0
	for i := 0; i < 4; i++ {
		if bits[i] {
			val |= 1 << i
		}
	}
	return val
}

func (r *Register4bit) Read() [4]bool {
	if r.floating {
		return [4]bool{
			rand.Intn(2) == 1,
			rand.Intn(2) == 1,
			rand.Intn(2) == 1,
			rand.Intn(2) == 1,
		}
	}
	var result [4]bool
	for i := 0; i < 4; i++ {
		result[i] = r.triggers[i].out
	}
	return result
}

type Memory16x4 struct {
	registers [16]Register4bit

	decoder [4]bool
}

func (r *Register4bit) Write(data [4]bool, clock bool, reset bool) {
	if !r.we {
		return
	}
	for i := 0; i < 4; i++ {
		r.triggers[i].data = data[i]
		r.triggers[i].clock = clock
		r.triggers[i].reset = reset
		r.triggers[i].Update()
	}
}

func (cpu *CPUContext) ExecuteTextCommand(cmd string, args []string) error {
	instr, err := convertTextToInstruction(cmd, args)
	if err != nil {
		return err
	}

	switch instr.OpCode {
	case OpMov:
		return cpu.executeMov(instr)
	case OpAdd, OpSub, OpMul, OpAnd, OpOr, OpXor, OpCmp:
		return cpu.executeALUOp(instr)
	case -3:
		return cpu.executeSave(instr)
	case -5:
		return cpu.executeLoadf(instr)
	case OpJmp, OpJz, OpJnz, OpJc:
		return cpu.executeJump(instr)
	case OpLoad:
		return cpu.executeLoad(instr)
	case OpStore:
		return cpu.executeStore(instr)
	default:
		return fmt.Errorf("unknown command: %s", cmd)
	}
}

func (a *ALU) CheckInterrupts() int {
	if !a.interrupts.enabled || a.interrupts.inHandler {
		return -1
	}

	for i := 0; i < 4; i++ {
		if a.interrupts.irqStatus[i] && a.interrupts.irqMask[i] {
			return i
		}
	}
	return -1
}

func (a *ALU) HandleInterrupt(pc *PCRegister, irqNum int) {
	a.interrupts.inHandler = true
	a.interrupts.savedPC = pc.Read()
	a.interrupts.savedFlags.carry = a.flags.carry
	a.interrupts.savedFlags.zero = a.flags.zero
	pc.Write(intTo4Bits(irqNum * 4))
}

func (a *ALU) ReturnFromInterrupt(pc *PCRegister) {
	pc.Write(a.interrupts.savedPC)
	a.flags.carry = a.interrupts.savedFlags.carry
	a.flags.zero = a.interrupts.savedFlags.zero
	a.interrupts.inHandler = false
}

func (m *Memory16x4) Read(addr [4]bool) [4]bool {

	addrInt := 0

	for i := 0; i < 4; i++ {

		if addr[i] {

			addrInt |= 1 << i

		}

	}

	return m.registers[addrInt].Read()

}

func (cpu *CPUContext) Reset() {
	cpu.pc.Write([4]bool{false, false, false, false})
	for i := 0; i < 4; i++ {
		reg := cpu.regFile.registers[i]
		reg.floating = true
		time.AfterFunc(1*time.Millisecond, func() {
			reg.floating = false
		})
	}
}
func (d *DFlipFlop) Update() {

	if d.reset {

		d.out = false

	} else if d.clock {

		d.out = d.data

	}

}

func NewALU() *ALU {

	return &ALU{
		aBits: make([]bool, 4),
		bBits: make([]bool, 4),
		flags: struct {
			carry bool
			zero  bool
		}{false, false},
	}

}

var cmpCache struct {
	a      [4]bool
	b      [4]bool
	result struct {
		carry bool
		zero  bool
	}
	timestamp time.Time
}

func sliceTo4Bool(s []bool) [4]bool {
	var result [4]bool
	for i := 0; i < 4 && i < len(s); i++ {
		result[i] = s[i]
	}
	return result
}

func BatchCompare(alu *ALU, pairs [][2][4]bool) {
	for i, pair := range pairs {
		alu.aBits = arrayToBoolSlice(pair[0])
		alu.bBits = arrayToBoolSlice(pair[1])
		alu.compare()
		showProgress(len(pairs), i+1)
	}
}

func arrayToBoolSlice(arr [4]bool) []bool {
	return []bool{arr[0], arr[1], arr[2], arr[3]}
}

func boolToInt(b bool) int {

	if b {

		return 1

	}

	return 0

}

func (r *RegisterFile) Write(regNum int, data []byte) {
	//r.mu.Lock()
	//defer r.mu.Unlock()

	if regNum < 0 || regNum > 3 {
		return
	}
	var dataBits [4]bool
	for i := 0; i < 4 && i < len(data); i++ {
		dataBits[i] = data[i] != 0
	}
	r.registers[regNum].Write(dataBits, true, false)
	r.dirty[regNum] = true
}

func NewRegisterFile() *RegisterFile {
	rf := &RegisterFile{}
	for i := range rf.registers {
		rf.registers[i] = Register4bit{
			triggers:     [4]DFlipFlop{},
			we:           true,
			cacheEnabled: true,
			dirty:        [4]bool{true, true, true, true},
		}
	}
	return rf
}

type PCRegister struct {
	triggers [4]DFlipFlop
	//mu       sync.Mutex
}

func (p *PCRegister) Read() [4]bool {

	var value [4]bool
	for i := 0; i < 4; i++ {
		value[i] = p.triggers[i].out
	}
	return value
}

func (p *PCRegister) Write(value [4]bool) {
	for i := 0; i < 4; i++ {
		p.triggers[i].data = value[i]
		p.triggers[i].clock = true
		p.triggers[i].Update()
	}
}

func NewPCRegister() *PCRegister {

	return &PCRegister{}

}

func (r *RegisterFile) Read(regNum int) []bool {
	//r.mu.RLock()
	//defer r.mu.RUnlock()

	if regNum < 0 || regNum > 3 {
		return make([]bool, 4)
	}

	if r.cacheEnabled && !r.dirty[regNum] {
		return r.cache[regNum][:]
	}

	val := r.registers[regNum].Read()

	if r.cacheEnabled {
		r.cache[regNum] = val
		r.dirty[regNum] = false
	}

	return val[:]
}

func (p *PCRegister) Increment() {
	//p.mu.Lock()
	//defer p.mu.Unlock()

	current := p.Read()
	carry := true
	var newValue [4]bool
	for i := 0; i < 4; i++ {
		sum := current[i] != carry
		carry = current[i] && carry
		newValue[i] = sum
	}
	p.Write(newValue)
}

func NewMemory16x4() *Memory16x4 {

	return &Memory16x4{}

}

func InitializeCPU() *CPUContext {

	stopChan := make(chan os.Signal, 1)
	clockChan := make(chan bool, 1)
	signal.Notify(stopChan, syscall.SIGINT, syscall.SIGTERM)

	terminal := &Terminal{
		ScreenBuffer: [80 * 25]byte{},
		CursorPos:    0,
		KeyBuffer:    []byte{},
		InputMode:    true,
		Echo:         true,
	}

	rom := NewROM16x4([16][4]bool{})
	ram := &RAM16x4{}
	bus := NewMemoryBus(rom, ram, terminal, 16, 16)

	ctx := &CPUContext{
		bus:      bus,
		regFile:  NewRegisterFile(),
		alu:      NewALU(),
		pc:       NewPCRegister(),
		terminal: terminal,
		stopChan: stopChan,
		clock:    clockChan,
		running:  true,
		labels:   make(map[string][4]bool),
	}

	for i := 0; i < 4; i++ {
		randomBits := [4]bool{
			rand.Intn(2) == 1,
			rand.Intn(2) == 1,
			rand.Intn(2) == 1,
			rand.Intn(2) == 1,
		}
		ctx.regFile.Write(i, []byte{
			boolToByte(randomBits[0]),
			boolToByte(randomBits[1]),
			boolToByte(randomBits[2]),
			boolToByte(randomBits[3]),
		})
	}

	ctx.pc.Write([4]bool{false, false, false, false})

	ctx.initLogger()
	signal.Notify(ctx.stopChan, syscall.SIGINT, syscall.SIGTERM)
	loadProgram(bus)
	go ctx.clockGenerator(10 * time.Millisecond)

	ctx.logger.Println("CPU инициализирован")
	for i := 0; i < 4; i++ {
		ctx.logger.Printf("R%d: %v\n", i, ctx.regFile.Read(i))
	}
	if _, err := os.Stat("cpu_state.json"); err == nil {
		if err := ctx.LoadState("cpu_state.json"); err != nil {
			ctx.logger.Printf("Не удалось загрузить состояние: %v", err)
		} else {
			ctx.logger.Println("Состояние загружено из cpu_state.json")
		}
	}
	go func() {
		<-ctx.stopChan
		if err := ctx.SaveState("cpu_state.json"); err != nil {
			ctx.logger.Printf("Не удалось сохранить состояние: %v", err)
		} else {
			ctx.logger.Println("Состояние сохранено в cpu_state.json")
		}
		os.Exit(0)
	}()

	return ctx
}
func (ctx *CPUContext) initLogger() {
	logFile, err := os.OpenFile("cpu.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal(err)
	}
	ctx.logger = log.New(logFile, "CPU: ", log.LstdFlags)
}

func (ctx *CPUContext) clockGenerator(interval time.Duration) {
	for ctx.running {
		time.Sleep(interval)
		ctx.clock <- true
	}
}

func (cpu *CPUContext) SaveProgram(filename string) error {
	data := strings.Join(cpu.program, "\n")
	return os.WriteFile(filename, []byte(data), 0644)
}

func (cpu *CPUContext) LoadfProgram(filename string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return err
	}
	return cpu.LoadfProgram(string(data))
}

func (cpu *CPUContext) executeCommand(line string) error {
	if cpu.labels == nil {
		cpu.labels = make(map[string][4]bool)
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}
	cleanLine := strings.TrimSuffix(line, "|")
	cleanLine = strings.TrimSpace(cleanLine)

	if cpu.interfaceMode == 1 {
		commands := strings.Split(line, "|")
		for _, cmd := range commands {
			cmd = strings.TrimSpace(cmd)
			if cmd == "" {
				continue
			}

			if strings.HasPrefix(cmd, "save ") {
				filename := strings.TrimPrefix(cmd, "save ")
				filename = strings.TrimSpace(filename)
				if filename == "" {
					return fmt.Errorf("не указано имя файла для сохранения")
				}
				if err := cpu.SaveState(filename); err != nil {
					return fmt.Errorf("ошибка сохранения: %v", err)
				}
				continue
			}

			if strings.HasPrefix(cmd, "loadf ") {
				filename := strings.TrimPrefix(cmd, "loadf ")
				filename = strings.TrimSpace(filename)
				if filename == "" {
					return fmt.Errorf("не указано имя файла для загрузки")
				}
				if err := cpu.LoadfProgram(filename); err != nil {
					return fmt.Errorf("ошибка загрузки: %v", err)
				}
				continue
			}
		}
	}

	switch cleanLine {
	case "help":
		return helpCommand(cpu, nil)
	case "regs":
		return regsCommand(cpu, nil)
	case "shell":
		cpu.interfaceMode = 0
		fmt.Println("Переключено в SHELL-режим")
		return nil
	case "pipeline":
		cpu.interfaceMode = 1
		fmt.Println("Переключено в PIPE-режим")
		return nil
	}

	if strings.Contains(line, ":") || strings.HasPrefix(line, ";") {
		cpu.interfaceMode = 1
	}
	if cpu.interfaceMode == 1 {
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
		}
	}

	if cpu.interfaceMode == 1 {
		commands := strings.Split(line, "|")
		for _, cmd := range commands {
			cmd = strings.TrimSpace(cmd)
			if cmd == "" {
				continue
			}
			cmd = strings.TrimSpace(cmd)
			if strings.HasSuffix(cmd, ":") {
				label := strings.TrimSuffix(cmd, ":")
				cpu.labels[label] = cpu.pc.Read()
				if cpu.logger != nil {
					cpu.logger.Printf("Сохранена метка '%s' по адресу %v\n",
						label, cpu.pc.Read())
				}
				continue
			}

			if strings.HasPrefix(cmd, ";") {
				continue
			}

			parts := strings.Fields(cmd)
			if len(parts) == 0 {
				continue
			}

			switch parts[0] {
			case "help":
				return helpCommand(cpu, parts[1:])
			case "regs":
				return regsCommand(cpu, parts[1:])
			case "shell":
				cpu.interfaceMode = 0
				fmt.Println("Переключено в SHELL-режим")
				return nil
			}

			instr, err := convertToInstruction(parts, cpu.labels)
			if err != nil {
				return err
			}

			tmpCmd := &Command{OpCode: instr.OpCode}
			if err := cpu.adaptToInstructionExecutor(tmpCmd.String(), parts[1:]); err != nil {
				return err
			}
		}
		return nil
	} else {
		return cpu.shellExecuteCommand(line)
	}
}

func (cpu *CPUContext) handleInterrupts() {
	if irqNum := cpu.alu.CheckInterrupts(); irqNum >= 0 {
		cpu.alu.HandleInterrupt(cpu.pc, irqNum)
	}
}

func convertToInstruction(parts []string, labels map[string][4]bool) (Instruction, error) {
	if len(parts) < 1 {
		return Instruction{}, fmt.Errorf("пустая команда")
	}

	pipelineMode := strings.Contains(parts[0], "|") || strings.HasPrefix(parts[0], "PIPE")

	opcode, ok := map[string]int{
		"mov": OpMov, "add": OpAdd, "sub": OpSub, "mul": OpMul,
		"and": OpAnd, "or": OpOr, "xor": OpXor, "cmp": OpCmp,
		"load": OpLoad, "store": OpStore, "jmp": OpJmp,
		"jz": OpJz, "jnz": OpJnz, "jc": OpJc, "hlt": OpHalt,
		"call": OpCall, "ret": OpRet, "run": -1, "save": -3, "loadf": -5,
		"mem": -4, "memrange": -6,
	}[parts[0]]
	if !ok {
		return Instruction{}, fmt.Errorf("неизвестная команда: %s", parts[0])
	}

	instr := Instruction{OpCode: opcode}

	if opcode == OpHalt || opcode == OpRet || opcode == -1 {
		if len(parts) > 1 {
			return instr, fmt.Errorf("команда %s не принимает аргументов", parts[0])
		}
		return instr, nil
	}

	if opcode == OpHalt || opcode == OpRet {
		if len(parts) > 1 {
			return instr, fmt.Errorf("команда %s не принимает аргументов", parts[0])
		}
		return instr, nil
	}
	if opcode == OpCall {
		if len(parts) < 2 {
			return instr, fmt.Errorf("недостаточно аргументов для CALL")
		}
		arg, err := parseArg(parts[1], true, pipelineMode, labels)
		if err != nil {
			return instr, fmt.Errorf("неверный аргумент для CALL: %v", err)
		}

		if arg.isReg {
			instr.Reg1 = arg.value
		} else if arg.isImm {
			instr.Imm = arg.bits
		} else {
			instr.Label = parts[1]
		}

		return instr, nil
	}
	if len(parts) > 1 {
		if reg, err := parseArg(parts[1], opcode == OpMov, pipelineMode, labels); err == nil {
			if reg.isReg {
				instr.Reg1 = reg.value
			} else {
				instr.Imm = reg.bits
			}
		} else {
			return instr, err
		}

		if len(parts) > 2 {
			if reg, err := parseArg(parts[2], opcode != OpStore, pipelineMode, labels); err == nil {
				if reg.isReg {
					instr.Reg2 = reg.value
				} else {
					instr.Imm = reg.bits
				}
			} else {
				return instr, err
			}
		}
	}

	return instr, nil
}

func parseArg(arg string, allowValue bool, pipelineMode bool, labels map[string][4]bool) (parsedArg, error) {
	if arg == "hlt" {
		return parsedArg{
			isReg: false,
			isImm: false,
			value: -1,
			bits:  [4]bool{false, false, false, false},
		}, nil
	}
	if strings.HasPrefix(arg, "r") {
		reg, err := strconv.Atoi(arg[1:])
		if err != nil {
			return parsedArg{}, fmt.Errorf("неверный формат регистра: %s (должен быть r0-r3)", arg)
		}
		if reg < 0 || reg > 3 {
			return parsedArg{}, fmt.Errorf("неверный регистр: %s (должен быть 0-3)", arg)
		}
		return parsedArg{isReg: true, value: reg}, nil
	}
	if !pipelineMode && strings.HasPrefix(arg, "@") {
		label := arg[1:]
		if addr, ok := labels[label]; ok {
			return parsedArg{isImm: true, bits: addr}, nil
		}
		return parsedArg{}, fmt.Errorf("неизвестная метка: %s", label)
	}
	if pipelineMode {
		if allowValue && len(arg) == 4 {
			bits, err := parseImmediate(arg, false)
			if err == nil {
				return parsedArg{isReg: false, bits: bits}, nil
			}
			valid := true
			for i, c := range arg {
				if c != '0' && c != '1' {
					valid = false
					break
				}
				bits[i] = c == '1'
			}
			if valid {
				return parsedArg{isReg: false, bits: bits}, nil
			}
		}
		return parsedArg{}, fmt.Errorf("неверный аргумент: %s (в pipeline режиме используйте r0-r3 или 4-битные двоичные значения)", arg)
	}
	if allowValue {
		if len(arg) == 4 {
			var bits [4]bool
			valid := true
			for i, c := range arg {
				if c != '0' && c != '1' {
					valid = false
					break
				}
				bits[i] = c == '1'
			}
			if valid {
				return parsedArg{isReg: false, bits: bits}, nil
			}
		}
		if num, err := strconv.ParseInt(arg, 10, 16); err == nil {
			if num < 0 || num > 15 {
				return parsedArg{}, fmt.Errorf("число должно быть от 0 до 15")
			}
			var bits [4]bool
			for i := 0; i < 4; i++ {
				bits[i] = (num>>i)&1 == 1
			}
			return parsedArg{isReg: false, bits: bits}, nil
		}
		if strings.HasPrefix(arg, "0x") {
			if num, err := strconv.ParseInt(arg[2:], 16, 16); err == nil && num >= 0 && num < 16 {
				var bits [4]bool
				for i := 0; i < 4; i++ {
					bits[i] = (num>>i)&1 == 1
				}
				return parsedArg{isReg: false, bits: bits}, nil
			}
		}
	}
	return parsedArg{}, fmt.Errorf("неверный аргумент: %s (допустимые форматы: r0-r3, 0-15, 0b0101, 0x0F, @label, hlt)", arg)
}
func (cpu *CPUContext) executeInstruction(instr Instruction) error {
	switch instr.OpCode {
	case OpMov:
		var value [4]bool
		if instr.Reg2 >= 0 {
			srcVal := cpu.regFile.Read(instr.Reg2)
			copy(value[:], srcVal[:4])
		} else {
			value = instr.Imm
		}
		cpu.regFile.Write(instr.Reg1, boolToByteSlice(value[:]))
		return nil

	case OpAdd, OpSub, OpMul, OpAnd, OpOr, OpXor, OpCmp:
		aBits := cpu.regFile.Read(instr.Reg1)
		bBits := cpu.regFile.Read(instr.Reg2)

		cpu.alu.aBits = aBits[:]
		cpu.alu.bBits = bBits[:]
		cpu.alu.op = instr.OpCode

		result, flags, err := cpu.alu.Execute()
		if err != nil {
			return err
		}

		cpu.regFile.Write(instr.Reg1, boolToByteSlice(result[:]))
		cpu.alu.flags = flags
		return nil
	case OpLoad:
		// LOAD Rd, [Rs]
		addrBits := cpu.regFile.Read(instr.Reg2)
		addr := boolSliceTo4Bits(addrBits[:4])
		data := cpu.bus.Read(addr)
		cpu.regFile.Write(instr.Reg1, boolToByteSlice(data[:]))
		return nil

	case OpStore:
		// STORE [Rd], Rs
		addrBits := cpu.regFile.Read(instr.Reg1)
		dataBits := cpu.regFile.Read(instr.Reg2)
		addr := boolSliceTo4Bits(addrBits[:4])
		data := boolSliceTo4Bits(dataBits[:4])
		cpu.bus.Write(addr, data, true)
		return nil

	case OpJmp:
		// JMP label/Rs
		return cpu.executeJump(instr)
	case OpJz:
		// JZ label/Rs
		if cpu.alu.flags.zero {
			return cpu.executeJump(instr)
		}
		return nil
	case OpJnz:
		// JNZ label/Rs
		if !cpu.alu.flags.zero {
			return cpu.executeJump(instr)
		}
		return nil
	case OpJc:
		// JC label/Rs
		if cpu.alu.flags.carry {
			return cpu.executeJump(instr)
		}
		return nil
	case OpHalt:
		return cpu.executeHalt(instr)
	case OpCall:
		return cpu.executeCall(instr)
	case OpRet:
		return cpu.executeRet(instr)
	case -1:
		cpu.logger.Println("Запуск выполнения программы")
		cpu.Run()
		return nil
	case -3:
		addr := instr.MemAddr
		if addr < 0 || addr > 15 {
			return fmt.Errorf("адрес памяти должен быть от 0 до 15")
		}
		data := cpu.bus.Read(intTo4Bits(addr))
		fmt.Printf("Память по адресу %02X: %s\n", addr, bitsToStr(data))
		return nil

	case -6:
		start := instr.MemAddr
		end := instr.Reg1
		if start < 0 || start > 15 || end < 0 || end > 15 {
			return fmt.Errorf("адреса должны быть от 0 до 15")
		}
		if start > end {
			return fmt.Errorf("начальный адрес должен быть меньше конечного")
		}

		fmt.Println("Адрес | Бинарно | Десятично | Hex | Символ")
		fmt.Println("----------------------------------------")
		for addr := start; addr <= end; addr++ {
			data := cpu.bus.Read(intTo4Bits(addr))
			value := bitsToInt(data)
			char := " "
			if value >= 32 && value <= 126 {
				char = string(rune(value))
			}
			fmt.Printf(" %02X  | %04b    | %3d      | %02X  | %s\n",
				addr, data, value, value, char)
		}
		return nil

	default:
		return fmt.Errorf("неизвестный код операции: %d", instr.OpCode)
	}
}

func (cpu *CPUContext) executeRet(instr Instruction) error {
	if len(cpu.callStack) > 0 {
		returnAddr := cpu.callStack[len(cpu.callStack)-1]
		cpu.callStack = cpu.callStack[:len(cpu.callStack)-1]
		cpu.pc.Write(returnAddr)
		return nil
	}
	returnAddr := cpu.regFile.Read(3)
	if len(returnAddr) < 4 {
		return fmt.Errorf("неверный адрес возврата в R3")
	}

	var addr [4]bool
	copy(addr[:], returnAddr[:4])

	if bitsToInt(addr) >= len(cpu.program) {
		return fmt.Errorf("недопустимый адрес возврата: %v", addr)
	}

	cpu.pc.Write(addr)
	return nil
}

func (cpu *CPUContext) executeCall(instr Instruction) error {
	currentPC := cpu.pc.Read()
	nextPC := increment4Bit(currentPC)
	cpu.callStack = append(cpu.callStack, nextPC)
	cpu.regFile.Write(3, boolToByteSlice(nextPC[:]))
	if instr.Reg1 >= 0 {
		target := cpu.regFile.Read(instr.Reg1)
		var targetAddr [4]bool
		copy(targetAddr[:], target[:4])
		cpu.pc.Write(targetAddr)
	} else if instr.Label != "" {
		if addr, ok := cpu.labels[instr.Label]; ok {
			cpu.pc.Write(addr)
		} else {
			return fmt.Errorf("неизвестная метка: %s", instr.Label)
		}
	}

	return nil
}
func (cpu *CPUContext) executeMov(instr Instruction) error {
	var value [4]bool

	switch {
	case instr.Reg2 >= 0:
		val := cpu.regFile.Read(instr.Reg2)
		value = boolSliceTo4Bits(val)
	case instr.Imm != [4]bool{}:
		value = instr.Imm
	case instr.Address != [4]bool{}:
		data := cpu.bus.Read(instr.Address)
		value = data
	default:
		return fmt.Errorf("неверный источник данных для MOV")
	}

	if instr.isMemDest {
		cpu.bus.Write(instr.Address, value, true)
	} else {

		cpu.regFile.Write(instr.Reg1, boolToByteSlice(value[:]))
	}

	return nil
}

func (cpu *CPUContext) SaveState(filename string) error {
	state := CPUState{}
	for i := 0; i < 4; i++ {
		reg := cpu.regFile.Read(i)
		copy(state.Registers[i][:], reg[:4])
	}
	state.PC = cpu.pc.Read()
	for i := 0; i < 16; i++ {
		state.Memory[i] = cpu.bus.Read(intTo4Bits(i))
	}
	state.Labels = make(map[string][4]bool)
	for k, v := range cpu.labels {
		state.Labels[k] = v
	}
	state.CallStack = make([][4]bool, len(cpu.callStack))
	copy(state.CallStack, cpu.callStack)

	state.Flags.Zero = cpu.alu.flags.zero
	state.Flags.Carry = cpu.alu.flags.carry
	state.Interrupts = cpu.alu.interrupts
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("ошибка сериализации: %v", err)
	}
	err = os.WriteFile(filename, data, 0644)
	if err != nil {
		return fmt.Errorf("ошибка записи в файл: %v", err)
	}

	return nil
}

func (cpu *CPUContext) LoadState(filename string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("ошибка чтения файла: %v", err)
	}
	var state CPUState
	err = json.Unmarshal(data, &state)
	if err != nil {
		return fmt.Errorf("ошибка десериализации: %v", err)
	}
	for i := 0; i < 4; i++ {
		cpu.regFile.Write(i, boolToByteSlice(state.Registers[i][:]))
	}
	cpu.pc.Write(state.PC)
	for i := 0; i < 16; i++ {
		cpu.bus.Write(intTo4Bits(i), state.Memory[i], true)
	}
	cpu.labels = make(map[string][4]bool)
	for k, v := range state.Labels {
		cpu.labels[k] = v
	}
	cpu.callStack = make([][4]bool, len(state.CallStack))
	copy(cpu.callStack, state.CallStack)
	cpu.alu.flags.zero = state.Flags.Zero
	cpu.alu.flags.carry = state.Flags.Carry
	cpu.alu.interrupts = state.Interrupts

	return nil
}

func convertTextToInstruction(cmd string, args []string) (Instruction, error) {
	var instr Instruction
	pipelineMode := strings.Contains(cmd, "|")

	switch cmd {
	case "mov":
		instr.OpCode = OpMov
		if len(args) < 2 {
			return instr, fmt.Errorf("недостаточно аргументов для MOV")
		}

		if strings.HasPrefix(args[0], "[") && strings.HasSuffix(args[0], "]") {

			regStr := strings.TrimSuffix(strings.TrimPrefix(args[0], "["), "]")
			reg, err := parseRegister(regStr)
			if err != nil {
				return instr, fmt.Errorf("неверный адрес назначения: %v", err)
			}
			instr.IsMemDest = true
			instr.Address = intTo4Bits(reg)
		} else {

			reg, err := parseRegister(args[0])
			if err != nil {
				return instr, fmt.Errorf("неверный регистр назначения: %v", err)
			}
			instr.Reg1 = reg
		}
		if strings.HasPrefix(args[1], "[") && strings.HasSuffix(args[1], "]") {

			regStr := strings.TrimSuffix(strings.TrimPrefix(args[1], "["), "]")
			reg, err := parseRegister(regStr)
			if err != nil {
				return instr, fmt.Errorf("неверный адрес источника: %v", err)
			}
			instr.IsMemSrc = true
			instr.Address = intTo4Bits(reg)
		} else if strings.HasPrefix(args[1], "r") {
			bits, err := parseImmediate(args[1], false)
			if err != nil {
				return instr, err
			}
			instr.Imm = bits

			reg, err := parseRegister(args[1])
			if err != nil {
				return instr, fmt.Errorf("неверный регистр источника: %v", err)
			}
			instr.Reg2 = reg
		} else {

			if pipelineMode {

				if len(args[1]) != 4 {
					return instr, fmt.Errorf("в pipeline режиме immediate-значение должно быть 4 бита (например '1011')")
				}
				var bits [4]bool
				for i, c := range args[1] {
					if c != '0' && c != '1' {
						return instr, fmt.Errorf("недопустимый символ в immediate-значении, только '0' или '1'")
					}
					bits[i] = c == '1'
				}
				instr.Imm = bits
			} else {
				if num, err := strconv.ParseInt(args[1], 10, 16); err == nil {
					if num < 0 || num > 15 {
						return instr, fmt.Errorf("immediate-значение должно быть от 0 до 15")
					}
					instr.Imm = intTo4Bits(int(num))
				} else if len(args[1]) == 4 {
					var bits [4]bool
					valid := true
					for i, c := range args[1] {
						if c != '0' && c != '1' {
							valid = false
							break
						}
						bits[i] = c == '1'
					}
					if !valid {
						return instr, fmt.Errorf("недопустимое immediate-значение, только '0' или '1'")
					}
					instr.Imm = bits
				} else {
					return instr, fmt.Errorf("недопустимое immediate-значение, укажите число (0-15) или 4 бита (например '1011')")
				}
			}
			instr.Reg2 = -1
		}
	case "mem":
		if len(args) == 1 {
			addr, err := parseAddress(args[0])
			if err != nil {
				return Instruction{}, fmt.Errorf("неверный адрес памяти: %v", err)
			}
			return Instruction{
				OpCode:  OpMem,
				MemAddr: addr,
			}, nil
		}

		if len(args) == 2 {
			start, err1 := parseAddress(args[0])
			end, err2 := parseAddress(args[1])
			if err1 != nil || err2 != nil {
				var errs []string
				if err1 != nil {
					errs = append(errs, fmt.Sprintf("начальный адрес: %v", err1))
				}
				if err2 != nil {
					errs = append(errs, fmt.Sprintf("конечный адрес: %v", err2))
				}
				return Instruction{}, fmt.Errorf("ошибки парсинга: %s", strings.Join(errs, ", "))
			}

			if start > end {
				return Instruction{}, fmt.Errorf("начальный адрес (%d) должен быть меньше конечного (%d)", start, end)
			}

			return Instruction{
				OpCode:  OpMemRange,
				MemAddr: start,
				Reg1:    end,
			}, nil
		}

		return Instruction{}, fmt.Errorf("неверное количество аргументов для mem, требуется 1 или 2")
	case "memrange":
		if len(args) == 2 {
			start, err1 := parseAddress(args[0])
			end, err2 := parseAddress(args[1])
			if err1 != nil || err2 != nil {
				var errs []string
				if err1 != nil {
					errs = append(errs, fmt.Sprintf("начальный адрес: %v", err1))
				}
				if err2 != nil {
					errs = append(errs, fmt.Sprintf("конечный адрес: %v", err2))
				}
				return Instruction{}, fmt.Errorf("ошибки парсинга: %s", strings.Join(errs, ", "))
			}

			if start > end {
				return Instruction{}, fmt.Errorf("начальный адрес (%d) должен быть меньше конечного (%d)", start, end)
			}

			return Instruction{
				OpCode:  OpMemRange,
				MemAddr: start,
				Reg1:    end,
			}, nil
		}

		return Instruction{}, fmt.Errorf("неверное количество аргументов для mem, требуется 1 или 2")

	case "save":
		instr.OpCode = -3
		if len(args) < 1 {
			return instr, fmt.Errorf("filename required for save")
		}
		instr.Filename = args[0]
		return instr, nil

	case "loadf":
		instr.OpCode = -5
		if len(args) < 1 {
			return instr, fmt.Errorf("filename required for loadf")
		}
		instr.Filename = args[0]
		return instr, nil

	case "add", "sub", "mul", "and", "or", "xor":
		instr.OpCode = map[string]int{
			"add": OpAdd,
			"sub": OpSub,
			"mul": OpMul,
			"and": OpAnd,
			"or":  OpOr,
			"xor": OpXor,
		}[cmd]

		if len(args) < 2 {
			return instr, fmt.Errorf("not enough arguments for %s", cmd)
		}
		reg1, err := parseRegister(args[0])
		if err != nil {
			return instr, fmt.Errorf("invalid first argument for %s: %v", cmd, err)
		}
		instr.Reg1 = reg1
		if strings.HasPrefix(args[1], "[") {
			addrStr := strings.Trim(args[1], "[]")
			if reg, err := parseRegister(addrStr); err == nil {
				instr.Reg2 = reg
				instr.IsMemSrc = true
			} else if addr, err := parseAddress(addrStr); err == nil {
				instr.Imm = intTo4Bits(addr)
				instr.IsMemSrc = true
			} else {
				return instr, fmt.Errorf("invalid memory address: %s", args[1])
			}
		} else if strings.HasPrefix(args[1], "r") {
			reg2, err := parseRegister(args[1])
			if err != nil {
				return instr, fmt.Errorf("invalid second argument for %s: %v", cmd, err)
			}
			instr.Reg2 = reg2
		} else {
			bits, err := parseImmediate(args[1], false)
			if err != nil {
				return instr, fmt.Errorf("invalid immediate value: %v", err)
			}
			instr.Imm = bits
			instr.Reg2 = -1
		}
		return instr, nil
	case "cmp":
		instr.OpCode = OpCmp
		if len(args) < 2 {
			return instr, fmt.Errorf("недостаточно аргументов для CMP")
		}
		reg1, err := parseRegister(args[0])
		if err != nil {
			return instr, fmt.Errorf("неверный первый аргумент для CMP: %v", err)
		}
		instr.Reg1 = reg1
		if strings.HasPrefix(args[1], "[") {
			addrStr := strings.Trim(args[1], "[]")
			if reg, err := parseRegister(addrStr); err == nil {
				instr.Reg2 = reg
				instr.IsMemSrc = true
			} else if addr, err := parseAddress(addrStr); err == nil {
				instr.Imm = intTo4Bits(addr)
				instr.IsMemSrc = true
			} else {
				return instr, fmt.Errorf("неверный адрес памяти: %s", args[1])
			}
		} else if strings.HasPrefix(args[1], "r") {
			reg2, err := parseRegister(args[1])
			if err != nil {
				return instr, fmt.Errorf("неверный второй аргумент для CMP: %v", err)
			}
			instr.Reg2 = reg2
		} else {
			bits, err := parseImmediate(args[1], false)
			if err != nil {
				return instr, fmt.Errorf("неверное immediate-значение: %v", err)
			}
			instr.Imm = bits
			instr.Reg2 = -1
		}
		return instr, nil

	case "jz":
		instr.OpCode = OpJz
		if len(args) < 1 {
			return instr, fmt.Errorf("недостаточно аргументов для JZ")
		}
		if strings.HasPrefix(args[0], "r") {
			reg, err := parseRegister(args[0])
			if err != nil {
				return instr, fmt.Errorf("неверный регистр для JZ: %v", err)
			}
			instr.Reg1 = reg
		} else {
			instr.Label = args[0]
		}

	case "jnz":
		instr.OpCode = OpJnz
		if len(args) < 1 {
			return instr, fmt.Errorf("недостаточно аргументов для JNZ")
		}
		if strings.HasPrefix(args[0], "r") {
			reg, err := parseRegister(args[0])
			if err != nil {
				return instr, fmt.Errorf("неверный регистр для JNZ: %v", err)
			}
			instr.Reg1 = reg
		} else {
			instr.Label = args[0]
		}

	case "jc":
		instr.OpCode = OpJc
		if len(args) < 1 {
			return instr, fmt.Errorf("недостаточно аргументов для JC")
		}
		if strings.HasPrefix(args[0], "r") {
			reg, err := parseRegister(args[0])
			if err != nil {
				return instr, fmt.Errorf("неверный регистр для JC: %v", err)
			}
			instr.Reg1 = reg
		} else {
			instr.Label = args[0]
		}

	case "load":
		instr.OpCode = OpLoad
		if len(args) < 2 {
			return instr, fmt.Errorf("недостаточно аргументов для LOAD")
		}
		reg, err := parseRegister(args[0])
		if err != nil {
			return instr, fmt.Errorf("неверный регистр для LOAD: %v", err)
		}
		addr, err := parseAddress(args[1])
		if err != nil {
			return instr, fmt.Errorf("неверный адрес для LOAD: %v", err)
		}
		instr.Reg1 = reg
		instr.Reg2 = addr

	case "store":
		instr.OpCode = OpStore
		if len(args) < 2 {
			return instr, fmt.Errorf("недостаточно аргументов для STORE")
		}
		addr, err := parseAddress(args[0])
		if err != nil {
			return instr, fmt.Errorf("неверный адрес для STORE: %v", err)
		}
		reg, err := parseRegister(args[1])
		if err != nil {
			return instr, fmt.Errorf("неверный регистр для STORE: %v", err)
		}
		instr.Reg1 = addr
		instr.Reg2 = reg
	case "hlt":
		instr.OpCode = OpHalt
		if len(args) > 0 {
			return instr, fmt.Errorf("команда HLT не принимает аргументов (получено %d)", len(args))
		}

		instr.Reg1 = -1
		instr.Reg2 = -1
		instr.Imm = [4]bool{false, false, false, false}
	case "call":
		instr.OpCode = OpCall
		if len(args) < 1 {
			return instr, fmt.Errorf("недостаточно аргументов для CALL")
		}
		if strings.HasPrefix(args[0], "r") {
			reg, err := parseRegister(args[0])
			if err != nil {
				return instr, fmt.Errorf("неверный регистр для CALL: %v", err)
			}
			instr.Reg1 = reg
		} else {
			instr.Label = args[0]
		}
	case "ret":
		instr.OpCode = OpRet
		if len(args) > 0 {
			return instr, fmt.Errorf("команда ret не принимает аргументов")
		}
		instr.Reg1 = -1
		instr.Reg2 = -1
		instr.Imm = [4]bool{false, false, false, false}
	case "run":
		instr.OpCode = -1
		if len(args) > 0 {
			return instr, fmt.Errorf("команда RUN не принимает аргументов")
		}
		instr.Reg1 = -1
		instr.Reg2 = -1

	default:
		return instr, fmt.Errorf("неизвестная команда: %s", cmd)
	}

	return instr, nil
}
func (cpu *CPUContext) Run() {
	if cpu.running {
		return
	}
	cpu.running = true
	cpu.pcLine = 0
	for cpu.running && cpu.pcLine < len(cpu.program) {
		line := strings.TrimSpace(cpu.program[cpu.pcLine])
		if line == "" || strings.HasPrefix(line, ";") {
			cpu.pcLine++
			continue
		}
		if strings.HasSuffix(line, ":") {
			cpu.pcLine++
			continue
		}
		err := cpu.executeCommand(line)
		if err != nil {
			log.Printf("Ошибка в строке %d: %v", cpu.pcLine, err)
			cpu.handleStop()
			return
		}
		cpu.pcLine++
		select {
		case <-cpu.clock:
			continue
		default:
			time.Sleep(1 * time.Millisecond)
		}
	}
}

func (ctx *CPUContext) handleStop() {
	ctx.logger.Println("CPU остановлен")
	ctx.running = false
}

func (ctx *CPUContext) handleClockTick() {
	//ctx.mu.Lock()
	//defer ctx.mu.Unlock()
	if !ctx.running {
		return
	}
	if ctx.pipeline.executeStage.done {
		ctx.executeCurrentInstruction()
		ctx.pipeline.executeStage.done = false
	}
	ctx.pipeline.Tick(ctx.bus, ctx.pc, ctx.alu)
	if !ctx.alu.interrupts.inHandler &&
		ctx.pipeline.executeStage.op != OpJmp &&
		ctx.pipeline.executeStage.op != OpJz &&
		ctx.pipeline.executeStage.op != OpJnz &&
		ctx.pipeline.executeStage.op != OpJc {
		ctx.pc.Increment()
	}
}
func parseImmediate(arg string, allowManual bool) ([4]bool, error) {
	arg = strings.TrimPrefix(arg, "0b")
	arg = strings.TrimPrefix(arg, "0x")
	if num, err := strconv.ParseInt(arg, 10, 16); err == nil {
		if num < 0 || num > 15 {
			return [4]bool{}, fmt.Errorf("число должно быть от 0 до 15")
		}
		return intTo4Bits(int(num)), nil
	}
	if len(arg) == 4 {
		var bits [4]bool
		for i, c := range arg {
			if c != '0' && c != '1' {
				return [4]bool{}, fmt.Errorf("недопустимый бит '%c'", c)
			}
			bits[i] = c == '1'
		}
		return bits, nil
	}

	return [4]bool{}, fmt.Errorf("неверный формат immediate-значения: %s", arg)
}

func (cpu *CPUContext) SwitchInterfaceMode() {
	cpu.interfaceMode = 1 - cpu.interfaceMode
	fmt.Printf("\nРежим переключен на: %s\n",
		map[int]string{0: "Shell", 1: "Pipeline"}[cpu.interfaceMode])
}

func (cpu *CPUContext) executeCurrentInstruction() error {
	instr := cpu.pipeline.executeStage.rawInstr
	decoded := DecodeInstruction(instr)
	// switch decoded.OpCode {
	// case OpAdd, OpSub, OpMul, OpAnd, OpOr, OpXor, OpCmp:
	// 	return cpu.executeALUOp(decoded)
	// case OpLoad:
	// 	return cpu.executeLoad(decoded)
	// case OpStore:
	// 	return cpu.executeStore(decoded)
	// case OpMov:
	// 	return cpu.executeMov(decoded)
	// case OpJmp:
	// 	return cpu.executeJump(decoded)
	// case OpJz:
	// 	return cpu.executeJz(decoded)
	// case OpJnz:
	// 	return cpu.executeJnz(decoded)
	// case OpJc:
	// 	return cpu.executeJc(decoded)
	// default:
	// 	return fmt.Errorf("unknown opcode: %d", decoded.OpCode)
	// }
	return cpu.adaptToTextExecutor(decoded)
}
func (ctx *CPUContext) handleLoad() {
	addr := ctx.regFile.Read(ctx.pipeline.executeStage.reg2)

	var addr4 [4]bool
	copy(addr4[:], addr)

	data := ctx.bus.Read(addr4)
	var dataBytes [4]byte
	for i := 0; i < 4; i++ {
		dataBytes[i] = boolToByte(data[i])
	}
	ctx.regFile.Write(ctx.pipeline.executeStage.reg1, dataBytes[:])
}

func (ctx *CPUContext) handleStore() {
	addr := ctx.regFile.Read(ctx.pipeline.executeStage.reg2)
	data := ctx.regFile.Read(ctx.pipeline.executeStage.reg1)
	ctx.bus.Write([4]bool(addr[:4]), [4]bool(data[:4]), true)
}

func (ctx *CPUContext) Cleanup() {
	ctx.logger.Println("Завершение работы")
}

func (cpu *CPUContext) LoadProgram(code string) error {
	cpu.program = strings.Split(code, "\n")
	if cpu.labels == nil {
		cpu.labels = make(map[string][4]bool)
	} else {
		for k := range cpu.labels {
			delete(cpu.labels, k)
		}
	}
	currentAddr := [4]bool{false, false, false, false}
	for _, line := range cpu.program {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, ";") {
			continue
		}

		if strings.HasSuffix(line, ":") {
			label := strings.TrimSuffix(line, ":")
			addrCopy := [4]bool{}
			copy(addrCopy[:], currentAddr[:])
			cpu.labels[label] = addrCopy
			continue
		}

		currentAddr = increment4Bit(currentAddr)
	}
	return nil
}

func increment4Bit(addr [4]bool) [4]bool {
	carry := true
	for i := 0; i < 4; i++ {
		newBit := addr[i] != carry
		carry = addr[i] && carry
		addr[i] = newBit
	}
	return addr
}

func loadProgram(bus *MemoryBus) {
	program := []struct {
		addr [4]bool
		data [4]bool
	}{
		// LOAD R1, [R2] (OpLoad = 8)
		{
			[4]bool{false, false, false, false}, // Адрес 0000
			[4]bool{true, false, false, false},  // 1000 (OpLoad=8, reg1=0, reg2=0)
		},
		// ADD R1, R2 (OpAdd = 0)
		{
			[4]bool{false, false, false, true},  // Адрес 0001
			[4]bool{false, false, false, false}, // 0000 (OpAdd=0, reg1=0, reg2=0)
		},
		// JMP R3 (OpJmp = 10)
		{
			[4]bool{false, false, true, false}, // Адрес 0010
			[4]bool{true, false, true, false},  // 1010 (OpJmp=10, reg1=0, reg2=2)
		},
		// CMP R1, R2 (OpCmp = 9)
		{
			[4]bool{false, false, true, true}, // Адрес 0011
			[4]bool{true, false, false, true}, // 1001 (OpCmp=9, reg1=0, reg2=1)
		},
	}

	for _, prog := range program {
		bus.Write(prog.addr, prog.data, true)
		log.Printf("Loaded instruction %v at address %v",
			bitsToStr(prog.data), bitsToStr(prog.addr))
	}
}

func intTo4Bits(n int) [4]bool {

	return [4]bool{

		n&8 == 8,

		n&4 == 4,

		n&2 == 2,

		n&1 == 1,
	}

}
func (c *Command) String() string {

	opCodeNames := map[int]string{
		OpAdd:   "add",
		OpSub:   "sub",
		OpMul:   "mul",
		OpAnd:   "and",
		OpOr:    "or",
		OpXor:   "xor",
		OpMov:   "mov",
		OpCmp:   "cmp",
		OpLoad:  "load",
		OpStore: "store",
		OpJmp:   "jmp",
		OpJz:    "jz",
		OpJnz:   "jnz",
		OpJc:    "jc",
		OpHalt:  "hlt",
		-1:      "run",
		OpCall:  "call",
		OpRet:   "ret",
		-3:      "mem",
		-2:      "step",
		-4:      "save",
		-5:      "loadf",
		-6:      "memrange",
	}
	if name, ok := opCodeNames[c.OpCode]; ok {
		return name
	}
	return fmt.Sprintf("unknown_opcode_%d", c.OpCode)
}

func decodeInstruction(instr [4]bool) (op int, reg1 int, reg2 int) {
	op = boolToInt(instr[0])<<3 | boolToInt(instr[1])<<2 |
		boolToInt(instr[2])<<1 | boolToInt(instr[3])

	op = boolToInt(instr[0])<<3 | boolToInt(instr[1])<<2 |

		boolToInt(instr[2])<<1 | boolToInt(instr[3])

	if op == -1 {
		return op, 0, 0
	}

	if op >= OpIRQ {
		return op, 0, 0
	}

	if op < 8 {

		return op, boolToInt(instr[2]), boolToInt(instr[3])

	}
	if op == OpCall {
		return op, boolToInt(instr[2]), boolToInt(instr[3])
	}

	return op, 0, 0

}

func boolToByte(b bool) byte {

	if b {

		return 1

	}

	return 0

}

func bitsToStr(bits interface{}) string {
	var s string
	switch v := bits.(type) {
	case []bool:
		for _, b := range v {
			if b {
				s += "1"
			} else {
				s += "0"
			}
		}
	case [4]bool:
		for _, b := range v {
			if b {
				s += "1"
			} else {
				s += "0"
			}
		}
	default:
		return "invalid type"
	}
	return s
}

func boolToByteSlice(bits []bool) []byte {
	res := make([]byte, len(bits))
	for i, b := range bits {
		res[i] = boolToByte(b)
	}
	return res
}

func boolSliceTo4Bits(bits []bool) [4]bool {
	var result [4]bool
	for i := 0; i < 4 && i < len(bits); i++ {
		result[i] = bits[i]
	}
	return result
}
