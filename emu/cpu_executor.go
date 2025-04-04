package main

import (
	"fmt"
)

func (cpu *CPUContext) executeALUOp(instr Instruction) error {

	aBits := cpu.regFile.Read(instr.Reg1)
	cpu.alu.aBits = aBits[:]
	var bBits [4]bool
	if instr.Reg2 >= 0 {
		regBits := cpu.regFile.Read(instr.Reg2)
		copy(bBits[:], regBits[:4])
	} else if instr.Imm != [4]bool{} {
		bBits = instr.Imm
	} else if instr.IsMemSrc {
		addrBits := cpu.regFile.Read(instr.Reg2)
		var addr [4]bool
		copy(addr[:], addrBits[:4])
		bBits = cpu.bus.Read(addr)
	} else {
		return fmt.Errorf("invalid second operand")
	}

	cpu.alu.bBits = bBits[:]
	cpu.alu.op = instr.OpCode

	result, flags, err := cpu.alu.Execute()
	if err != nil {
		return err
	}
	if instr.OpCode != OpCmp {
		cpu.regFile.Write(instr.Reg1, boolToByteSlice(result[:]))
	}
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
	if cpu.interfaceMode == 1 && instr.Address != [4]bool{} {
		addr = instr.Address
	} else {
		addrBits := cpu.regFile.Read(instr.Reg2)
		copy(addr[:], addrBits[:4])
	}

	dataBits := cpu.regFile.Read(instr.Reg1)
	var data [4]bool
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

func (cpu *CPUContext) executeHalt(instr Instruction) error {
	cpu.running = false
	if cpu.logger != nil {
		cpu.logger.Println("CPU остановлен по команде HLT")
	}
	cpu.pipeline = Pipeline{}
	return nil
}

func (cpu *CPUContext) executeSave(instr Instruction) error {
	if instr.Filename == "" {
		return fmt.Errorf("filename not specified for save")
	}
	if err := cpu.SaveState(instr.Filename); err != nil {
		return fmt.Errorf("save failed: %v", err)
	}
	return nil
}

func (cpu *CPUContext) executeLoadf(instr Instruction) error {
	if instr.Filename == "" {
		return fmt.Errorf("filename not specified for loadf")
	}
	if err := cpu.LoadfProgram(instr.Filename); err != nil {
		return fmt.Errorf("load failed: %v", err)
	}
	return nil
}
