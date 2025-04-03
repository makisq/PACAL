package main

type Instruction struct {
	OpCode    int
	Reg1      int
	Reg2      int
	Imm       [4]bool
	Label     string
	Address   [4]bool
	isImm     [4]bool
	isMemDest bool
	IsMemDest bool
	IsMemSrc  bool
}

func DecodeInstruction(instr [4]bool) Instruction {
	op := boolToInt(instr[0])<<3 | boolToInt(instr[1])<<2 |
		boolToInt(instr[2])<<1 | boolToInt(instr[3])

	decoded := Instruction{OpCode: op}

	switch op {
	case OpHalt:
		return decoded

	case OpCall:
		decoded.Reg1 = boolToInt(instr[2])
		decoded.Reg2 = -1
		return decoded

	case OpMov:
		if instr[0] {
			decoded.Imm = [4]bool{instr[1], instr[2], instr[3], false}
			decoded.Reg2 = -1
		} else {
			decoded.Reg1 = boolToInt(instr[1])
			decoded.Reg2 = boolToInt(instr[2])
		}
		return decoded

	case OpLoad, OpStore:
		decoded.Reg1 = boolToInt(instr[1])
		decoded.Reg2 = boolToInt(instr[2])
		decoded.Address = [4]bool{false, instr[1], instr[2], instr[3]}
		return decoded

	case OpJmp, OpJz, OpJnz, OpJc:
		decoded.Reg1 = boolToInt(instr[1])
		decoded.Address = [4]bool{false, instr[1], instr[2], instr[3]}
		return decoded

	default:
		decoded.Reg1 = boolToInt(instr[1])
		decoded.Reg2 = boolToInt(instr[2])
		return decoded
	}
}
