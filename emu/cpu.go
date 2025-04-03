package main

import (
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
	OpJz
	OpJnz
	OpJc
	OpIRQ
	OpIRET
	OpEI
	OpDI
)

var mus sync.RWMutex
var pcCounter uint32

type DFlipFlop struct {
	data bool

	clock bool

	reset bool

	out bool
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

func (cpu *CPUContext) executeCommand(line string) error {
	if strings.Contains(line, ":") || strings.HasPrefix(line, ";") {
		cpu.interfaceMode = 1
	}

	if cpu.interfaceMode == 1 {
		parts := strings.Fields(line)
		if len(parts) == 0 {
			return nil
		}

		if strings.HasSuffix(parts[0], ":") {
			label := strings.TrimSuffix(parts[0], ":")
			addr := cpu.pc.Read()
			cpu.labels[label] = addr
			return nil
		}

		if strings.HasPrefix(parts[0], ";") {
			return nil
		}

		instr, err := convertToInstruction(parts)
		if err != nil {
			return err
		}
		return cpu.adaptToTextExecutor(instr)
	} else {
		// Shell-режим
		return cpu.shellExecuteCommand(line)
	}
}

func convertToInstruction(parts []string) (Instruction, error) {
	var instr Instruction

	if len(parts) == 0 {
		return instr, fmt.Errorf("недостаточно аргументов")
	}

	switch {
	case strings.HasPrefix(parts[0], "r"):
		reg, err := parseRegister(parts[0][1:])
		if err != nil {
			return instr, err
		}
		instr.Reg1 = reg
	case strings.HasPrefix(parts[0], "["):
		regStr := strings.Trim(parts[0], "[]")
		if strings.HasPrefix(regStr, "r") {
			reg, err := parseRegister(regStr[1:])
			if err != nil {
				return instr, err
			}
			instr.Reg2 = reg
		}
	default:
		instr.Label = parts[0]
	}

	if len(parts) > 1 {
		switch {
		case strings.HasPrefix(parts[1], "r"):
			reg, err := parseRegister(parts[1][1:])
			if err != nil {
				return instr, err
			}
			instr.Reg2 = reg
		case strings.HasPrefix(parts[1], "["):
			regStr := strings.Trim(parts[1], "[]")
			if strings.HasPrefix(regStr, "r") {
				reg, err := parseRegister(regStr[1:])
				if err != nil {
					return instr, err
				}
				instr.Reg1 = reg
			}
		default:
			if num, err := strconv.Atoi(parts[1]); err == nil {
				for i := 0; i < 4 && i < len(instr.Imm); i++ {
					instr.Imm[i] = (num>>i)&1 == 1
				}
			} else {
				instr.Label = parts[1]
			}
		}
	}

	return instr, nil
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

	default:
		return fmt.Errorf("неизвестный код операции: %d", instr.OpCode)
	}
}

func (cpu *CPUContext) executeMovInstr(instr Instruction) error {
	var value [4]bool
	if instr.Reg2 >= 0 {
		val := cpu.regFile.Read(instr.Reg2)
		value = boolSliceTo4Bits(val)
	} else {
		value = instr.Imm
	}
	cpu.regFile.Write(instr.Reg1, boolToByteSlice(value[:]))
	return nil
}

func convertTextToInstruction(cmd string, args []string) (Instruction, error) {
	var instr Instruction
	switch cmd {
	case "mov":
		instr.OpCode = OpMov
	case "add":
		instr.OpCode = OpAdd
	case "sub":
		instr.OpCode = OpSub
	case "mul":
		instr.OpCode = OpMul
	case "and":
		instr.OpCode = OpAnd
	case "or":
		instr.OpCode = OpOr
	case "xor":
		instr.OpCode = OpXor
	case "cmp":
		instr.OpCode = OpCmp
	case "load":
		instr.OpCode = OpLoad
	case "store":
		instr.OpCode = OpStore
	case "jmp":
		instr.OpCode = OpJmp
	case "jz":
		instr.OpCode = OpJz
	case "jnz":
		instr.OpCode = OpJnz
	case "jc":
		instr.OpCode = OpJc
	default:
		return instr, fmt.Errorf("неизвестная команда: %s", cmd)
	}
	switch instr.OpCode {
	case OpMov, OpAdd, OpSub, OpMul, OpAnd, OpOr, OpXor, OpCmp:
		if len(args) < 2 {
			return instr, fmt.Errorf("недостаточно аргументов")
		}
		reg1, err := parseRegister(args[0])
		if err != nil {
			return instr, err
		}
		reg2, err := parseRegister(args[1])
		if err != nil {
			return instr, err
		}
		instr.Reg1 = reg1
		instr.Reg2 = reg2

	case OpLoad, OpStore:
		if len(args) < 2 {
			return instr, fmt.Errorf("недостаточно аргументов")
		}
		reg, err := parseRegister(args[0])
		if err != nil {
			return instr, err
		}
		addr, err := parseAddress(args[1])
		if err != nil {
			return instr, err
		}
		instr.Reg1 = reg
		instr.Reg2 = addr

	case OpJmp, OpJz, OpJnz, OpJc:
		if len(args) < 1 {
			return instr, fmt.Errorf("недостаточно аргументов")
		}
		if strings.HasPrefix(args[0], "r") {
			reg, err := parseRegister(args[0][1:])
			if err != nil {
				return instr, err
			}
			instr.Reg1 = reg
		} else {
			instr.Label = args[0]
		}
	}

	return instr, nil
}
func (cpu *CPUContext) Run() {
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
	cpu.labels = make(map[string][4]bool)

	currentAddr := [4]bool{false, false, false, false}
	for _, line := range cpu.program {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasSuffix(line, ":") {
			label := strings.TrimSuffix(line, ":")
			cpu.labels[label] = currentAddr
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

func decodeInstruction(instr [4]bool) (op int, reg1 int, reg2 int) {
	op = boolToInt(instr[0])<<3 | boolToInt(instr[1])<<2 |
		boolToInt(instr[2])<<1 | boolToInt(instr[3])

	op = boolToInt(instr[0])<<3 | boolToInt(instr[1])<<2 |

		boolToInt(instr[2])<<1 | boolToInt(instr[3])

	if op >= OpIRQ {
		return op, 0, 0
	}

	if op < 8 {

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
