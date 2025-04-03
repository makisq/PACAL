package main

import (
	"fmt"
)

func (cpu *CPUContext) executeALUOp(instr Instruction) error {
	// Работает одинаково в обоих режимах
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
}

func (cpu *CPUContext) executeLoad(instr Instruction) error {
	var addr [4]bool

	if cpu.interfaceMode == 1 && instr.Address != [4]bool{} {
		addr = instr.Address
	} else {
		addrBits := cpu.regFile.Read(instr.Reg2)
		copy(addr[:], addrBits[:4])
	}

	data := cpu.bus.Read(addr)
	cpu.regFile.Write(instr.Reg1, boolToByteSlice(data[:]))
	return nil
}

func (cpu *CPUContext) executeStore(instr Instruction) error {
	var addr [4]bool
	var data [4]bool

	if cpu.interfaceMode == 1 && instr.Address != [4]bool{} {
		// Pipeline режим
		addr = instr.Address
	} else {
		// Shell режим
		addrBits := cpu.regFile.Read(instr.Reg2)
		copy(addr[:], addrBits[:4])
	}

	dataBits := cpu.regFile.Read(instr.Reg1)
	copy(data[:], dataBits[:4])

	cpu.bus.Write(addr, data, true)
	return nil
}

func (cpu *CPUContext) executeJump(instr Instruction) error {
	var target [4]bool

	if instr.Label != "" {
		if labelAddr, ok := cpu.labels[instr.Label]; ok {
			target = labelAddr
		} else {
			return fmt.Errorf("неизвестная метка: %s", instr.Label)
		}
	} else {
		if cpu.interfaceMode == 1 && instr.Address != [4]bool{} {
			target = instr.Address
		} else {
			if instr.Reg1 < 0 || instr.Reg1 > 3 {
				return fmt.Errorf("неверный регистр: R%d", instr.Reg1)
			}
			regValue := cpu.regFile.Read(instr.Reg1)
			target = boolSliceTo4Bits(regValue[:4])
		}
	}

	switch instr.OpCode {
	case OpJmp:
		cpu.pc.Write(target)
	case OpJz:
		if cpu.alu.flags.zero {
			cpu.pc.Write(target)
		}
	case OpJnz:
		if !cpu.alu.flags.zero {
			cpu.pc.Write(target)
		}
	case OpJc:
		if cpu.alu.flags.carry {
			cpu.pc.Write(target)
		}
	}
	return nil
}
