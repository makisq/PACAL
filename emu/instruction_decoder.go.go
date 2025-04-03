package main

type Instruction struct {
	OpCode int
	Reg1   int
	Reg2   int
	Imm    [4]bool
	Label  string
}

func DecodeInstruction(instr [4]bool) Instruction {
	op := boolToInt(instr[0])<<3 | boolToInt(instr[1])<<2 |
		boolToInt(instr[2])<<1 | boolToInt(instr[3])

	if op <= OpMul || (op >= OpAnd && op <= OpCmp) {
		return Instruction{
			OpCode: op,
			Reg1:   boolToInt(instr[2]),
			Reg2:   boolToInt(instr[3]),
		}
	}

	if op == OpMov || op == OpLoad || op == OpStore {
		return Instruction{
			OpCode: op,
			Reg1:   boolToInt(instr[2]),
			Imm:    [4]bool{instr[0], instr[1], false, false},
		}
	}

	return Instruction{OpCode: op}
}
