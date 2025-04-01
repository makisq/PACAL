package main

import (
	"fmt"
	"log"
	"math/rand"
	"os"

	"os/signal"

	"syscall"

	"time"
)

const (
	OpAdd = iota
	OpSub
	OpMul
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

type ALU struct {
	op    int
	aBits []bool
	bBits []bool
	flags struct {
		carry bool
		zero  bool
	}
	interrupts InterruptState
}

type DFlipFlop struct {
	data bool

	clock bool

	reset bool

	out bool
}

type Register4bit struct {
	triggers [4]DFlipFlop
	we       bool
	floating bool
}

type RegisterFile struct {
	registers [4]Register4bit
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
	alu      *ALU
	pc       *PCRegister
	pipeline Pipeline
	stopChan chan os.Signal
	clock    chan bool
	running  bool
	logger   *log.Logger
}

func (p *Pipeline) Tick(bus *MemoryBus, pc *PCRegister, alu *ALU) {
	if !alu.interrupts.inHandler {
		if irqNum := alu.CheckInterrupts(); irqNum >= 0 {
			alu.HandleInterrupt(pc, irqNum)
			p.fetchStage = [4]bool{false, false, false, false}
			p.decodeStage = PipelineStage{}
			p.executeStage = PipelineStage{}
			return
		}
	}

	pcValue := pc.Read()
	if pcInt := bitsToInt(pcValue); pcInt < 16 {
		p.fetchStage = bus.Read(pcValue)
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
		time.AfterFunc(100*time.Millisecond, func() {
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
	}

}

func boolSliceTo4Bits(bits []bool) [4]bool {
	var result [4]bool
	for i := 0; i < 4 && i < len(bits); i++ {
		result[i] = bits[i]
	}
	return result
}

func byteToBool(b []byte) []bool {

	res := make([]bool, len(b))

	for i := range b {

		res[i] = b[i] != 0

	}

	return res

}

func (a *ALU) subtract() ([]byte, []byte) {
	invertedB := make([]bool, 4)
	for i := 0; i < 4; i++ {
		invertedB[i] = !a.bBits[i]
	}

	carry := true
	bTwosComplement := make([]bool, 4)
	for i := 0; i < 4; i++ {
		sum, newCarry := summator1(invertedB[i], false, carry)
		bTwosComplement[i] = sum == 1
		carry = newCarry == 1
	}

	result := make([]byte, 4)
	carry = false
	var overflow bool
	for i := 0; i < 4; i++ {
		sum, newCarry := summator1(a.aBits[i], bTwosComplement[i], carry)
		result[i] = sum
		carry = newCarry == 1
	}
	overflow = carry

	a.flags.carry = !overflow
	a.flags.zero = true
	for _, b := range result {
		if b != 0 {
			a.flags.zero = false
			break
		}
	}

	return result, nil
}

func (a *ALU) compare() ([]byte, []byte) {
	a.subtract()
	return nil, nil
}

func (a *ALU) Execute() ([]byte, []byte) {

	switch a.op {

	case OpAdd:

		return a.add()

	case OpMul:

		return a.multiply()

	case OpAnd:

		return a.and()

	case OpOr:

		return a.or()

	case OpXor:

		return a.xor()

	case OpSub:

		return a.subtract()

	case OpCmp:

		return a.compare()

	default:

		return nil, nil

	}

}

func (a *ALU) and() ([]byte, []byte) {
	result := make([]byte, 4)
	for i := 0; i < 4; i++ {
		result[i] = boolToByte(a.aBits[i] && a.bBits[i])
	}

	a.flags.zero = true
	for _, b := range result {
		if b != 0 {
			a.flags.zero = false
			break
		}
	}
	a.flags.carry = false

	return result, nil
}

func (a *ALU) or() ([]byte, []byte) {
	result := make([]byte, 4)
	for i := 0; i < 4; i++ {
		result[i] = boolToByte(a.aBits[i] || a.bBits[i])
	}

	a.flags.zero = true
	for _, b := range result {
		if b != 0 {
			a.flags.zero = false
			break
		}
	}
	a.flags.carry = false

	return result, nil
}

func (a *ALU) xor() ([]byte, []byte) {
	result := make([]byte, 4)
	for i := 0; i < 4; i++ {
		result[i] = boolToByte(a.aBits[i] != a.bBits[i])
	}
	a.flags.zero = true
	for _, b := range result {
		if b != 0 {
			a.flags.zero = false
			break
		}
	}
	a.flags.carry = false

	return result, nil
}

func (a *ALU) add() ([]byte, []byte) {
	result := make([]byte, 4)
	carry := false

	for i := 0; i < 4; i++ {
		sum, newCarry := summator1(a.aBits[i], a.bBits[i], carry)
		result[i] = sum
		carry = newCarry == 1
	}

	a.flags.carry = carry
	a.flags.zero = (result[0] == 0 && result[1] == 0 && result[2] == 0 && result[3] == 0)

	return result, []byte{boolToByte(carry)}
}

func (a *ALU) multiply() ([]byte, []byte) {

	aVal := 0
	bVal := 0
	for i := 0; i < 4; i++ {
		if a.aBits[i] {
			aVal |= 1 << i
		}
		if a.bBits[i] {
			bVal |= 1 << i
		}
	}

	res := aVal * bVal

	result := make([]byte, 4)
	for i := 0; i < 4; i++ {
		if res&(1<<i) != 0 {
			result[i] = 1
		}
	}

	a.flags.carry = res > 15
	a.flags.zero = res == 0

	carries := make([]byte, 4)
	for i := 0; i < 4; i++ {
		if res&(1<<(i+4)) != 0 {
			carries[i] = 1
		}
	}

	return result, carries
}

func boolToInt(b bool) int {

	if b {

		return 1

	}

	return 0

}

func NewRegisterFile() *RegisterFile {
	rf := &RegisterFile{}
	for i := range rf.registers {
		rf.registers[i] = Register4bit{
			triggers: [4]DFlipFlop{},
			we:       true,
		}
	}
	return rf
}

type PCRegister struct {
	triggers [4]DFlipFlop
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
	if regNum < 0 || regNum > 3 {
		return make([]bool, 4)
	}
	reg := r.registers[regNum].Read()
	return reg[:]
}
func (r *RegisterFile) Write(regNum int, data []byte) {
	if regNum < 0 || regNum > 3 {
		return
	}
	var dataBits [4]bool
	for i := 0; i < 4 && i < len(data); i++ {
		dataBits[i] = data[i] != 0
	}
	r.registers[regNum].Write(dataBits, true, false)
}
func (p *PCRegister) Increment() {
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

func boolSliceToBytes(bits []bool) []byte {

	result := make([]byte, len(bits))

	for i, b := range bits {

		result[i] = boolToByte(b)

	}

	return result

}

func InitializeCPU() *CPUContext {
	rand.Seed(time.Now().UnixNano())

	rom := NewROM16x4([16][4]bool{})
	ram := &RAM16x4{}
	terminal := &Terminal{
		ScreenBuffer: [80 * 25]byte{},
		CursorPos:    0,
		KeyBuffer:    []byte{},
	}

	bus := NewMemoryBus(rom, ram, terminal, 16, 16)

	ctx := &CPUContext{
		bus:      bus,
		regFile:  NewRegisterFile(),
		alu:      NewALU(),
		pc:       NewPCRegister(),
		pipeline: Pipeline{},
		stopChan: make(chan os.Signal, 1),
		clock:    make(chan bool),
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
	go ctx.clockGenerator(1 * time.Second)

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

func (ctx *CPUContext) Run() {
	for ctx.running {
		select {
		case <-ctx.stopChan:
			ctx.handleStop()
		case <-ctx.clock:
			ctx.handleClockTick()
		}
	}
}

func (ctx *CPUContext) handleStop() {
	ctx.logger.Println("CPU остановлен")
	ctx.running = false
}

func (ctx *CPUContext) handleClockTick() {
	ctx.pipeline.Tick(ctx.bus, ctx.pc, ctx.alu)
	ctx.handleDataHazards()
	ctx.handlePCIncrement()

	if ctx.pipeline.executeStage.done {
		ctx.executeCurrentInstruction()
	}
}

func (ctx *CPUContext) handleDataHazards() {
	if ctx.pipeline.decodeStage.valid && ctx.pipeline.executeStage.valid {
		if ctx.pipeline.decodeStage.reg1 == ctx.pipeline.executeStage.reg1 ||
			ctx.pipeline.decodeStage.reg2 == ctx.pipeline.executeStage.reg1 {
			ctx.pipeline.decodeStage = PipelineStage{
				valid:    false,
				rawInstr: [4]bool{false, false, false, false},
			}
		}
	}
}

func (ctx *CPUContext) handlePCIncrement() {
	if !ctx.alu.interrupts.inHandler &&
		ctx.pipeline.executeStage.op != OpJmp &&
		ctx.pipeline.executeStage.op != OpJz &&
		ctx.pipeline.executeStage.op != OpJnz &&
		ctx.pipeline.executeStage.op != OpJc {
		ctx.pc.Increment()
	}
}

func (ctx *CPUContext) executeCurrentInstruction() {
	jumpTaken := false
	var nextPC [4]bool

	switch ctx.pipeline.executeStage.op {
	case OpAdd, OpSub, OpMul, OpAnd, OpOr, OpXor, OpMov, OpCmp:
		execute(ctx.bus, ctx.regFile, ctx.alu, ctx.pc, ctx.pipeline.executeStage.value)
	case OpLoad:
		ctx.handleLoad()
	case OpStore:
		ctx.handleStore()
	case OpJmp, OpJz, OpJnz, OpJc:
		jumpTaken, nextPC = ctx.handleJump()
	}

	if jumpTaken {
		ctx.handleJumpTaken(nextPC)
	}
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

func (ctx *CPUContext) handleJump() (bool, [4]bool) {
	var jumpTaken bool
	var nextPC [4]bool

	switch {
	case ctx.pipeline.executeStage.op == OpJmp:
		jumpTaken = true
	case ctx.pipeline.executeStage.op == OpJz && ctx.alu.flags.zero:
		jumpTaken = true
	case ctx.pipeline.executeStage.op == OpJnz && !ctx.alu.flags.zero:
		jumpTaken = true
	case ctx.pipeline.executeStage.op == OpJc && ctx.alu.flags.carry:
		jumpTaken = true
	}

	if jumpTaken {
		newPC := ctx.regFile.Read(ctx.pipeline.executeStage.reg1)
		nextPC = [4]bool(newPC[:4])
	}

	return jumpTaken, nextPC
}

func (ctx *CPUContext) handleJumpTaken(nextPC [4]bool) {
	ctx.pc.Write(nextPC)
	ctx.pipeline = Pipeline{}
	ctx.logger.Println("Прыжок выполнен, новый PC:", bitsToStr(nextPC[:]))
}

func (ctx *CPUContext) Cleanup() {
	ctx.logger.Println("Завершение работы")
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

func byteSliceTo4Bits(data []byte) [4]bool {
	var res [4]bool
	for i := 0; i < 4 && i < len(data); i++ {
		res[i] = data[i] != 0
	}
	return res
}

func bool4ToByteSlice(bits [4]bool) []byte {
	res := make([]byte, 4)
	for i := 0; i < 4; i++ {
		if bits[i] {
			res[i] = 1
		}
	}
	return res
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

func summator1(a, b, p bool) (byte, byte) {

	S := (a != b) != p

	P := (a && b) || (p && (a != b))

	p1 := map[bool]byte{false: 0, true: 1}[P]

	s1 := map[bool]byte{false: 0, true: 1}[S]

	return p1, s1

}

func binaryIncrement(bits [4]bool) [4]bool {

	carry := true

	var result [4]bool

	for i := 0; i < 4; i++ {

		sum, newCarry := summator1(bits[i], false, carry)

		result[i] = sum == 1

		carry = newCarry == 1

	}

	return result

}

func execute(bus *MemoryBus, regFile *RegisterFile, alu *ALU, pc *PCRegister, instruction [4]bool) {
	op, reg1, reg2 := decodeInstruction(instruction)

	switch op {
	case OpAdd, OpSub, OpMul, OpAnd, OpOr, OpXor:
		aBits := regFile.Read(reg1)
		bBits := regFile.Read(reg2)

		alu.aBits = aBits
		alu.bBits = bBits
		alu.op = OpAdd

		result, _ := alu.Execute()
		resultBits := byteSliceTo4Bits(result)
		regFile.Write(reg1, bool4ToByteSlice(resultBits))
		fmt.Printf("Result: %v\n", result)

		regFile.Write(reg1, result)
		fmt.Printf("After ADD: R%d=%v\n", reg1, regFile.Read(reg1))
	case OpMov:
		val := regFile.Read(reg2)
		regFile.Write(reg1, boolSliceToBytes(val))

	case OpLoad:
		addr := regFile.Read(reg2)
		addr4 := boolSliceTo4Bits(addr)
		data := bus.Read(addr4)
		regFile.Write(reg1, boolToByteSlice(data[:]))

	case OpStore:
		addr := regFile.Read(reg2)
		data := regFile.Read(reg1)
		addr4 := boolSliceTo4Bits(addr)
		data4 := boolSliceTo4Bits(data)
		fmt.Printf("Storing %v at address %v\n", data4, addr4)
		bus.Write(addr4, data4, true)

	case OpJmp:
		newPC := regFile.Read(reg1)
		pc.Write(boolSliceTo4Bits(newPC))
		return

	case OpJz:
		if alu.flags.zero {
			newPC := regFile.Read(reg1)
			pc.Write(boolSliceTo4Bits(newPC))
			return
		}

	case OpCmp:

		aBits := regFile.Read(reg1)
		bBits := regFile.Read(reg2)

		invertedB := make([]bool, 4)
		for i := range bBits {
			invertedB[i] = !bBits[i]
		}

		alu.aBits = invertedB
		alu.bBits = []bool{true, false, false, false}
		alu.op = OpAdd
		bTwosComplement, _ := alu.Execute()
		alu.aBits = aBits
		alu.bBits = byteToBool(bTwosComplement)
		alu.op = OpAdd
		result, _ := alu.Execute()

		alu.flags.zero = true
		for _, b := range result {
			if b != 0 {
				alu.flags.zero = false
				break
			}
		}

	case OpIRQ:
		if reg1 >= 0 && reg1 < 4 {
			alu.interrupts.irqStatus[reg1] = true
		}

	case OpIRET:
		alu.ReturnFromInterrupt(pc)

	case OpEI:
		alu.interrupts.enabled = true

	case OpDI:
		alu.interrupts.enabled = false

	default:
		log.Printf("Неизвестная инструкция: %v", instruction)
		pc.Increment()
	}
}
func boolToByteSlice(bits []bool) []byte {
	res := make([]byte, len(bits))
	for i, b := range bits {
		res[i] = boolToByte(b)
	}
	return res
}
