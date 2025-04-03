package main

import (
	"fmt"
	"strings"
)

func (cpu *CPUContext) adaptToInstructionExecutor(cmd string, args []string) error {
	instr, err := convertTextToInstruction(cmd, args)
	if err != nil {
		return err
	}

	if cpu.interfaceMode == 1 {
		// Pipeline режим - используем общий executeInstruction
		return cpu.executeInstruction(instr)
	} else {
		// Shell режим - вызываем конкретные функции выполнения
		switch instr.OpCode {
		case OpAdd, OpSub, OpMul, OpAnd, OpOr, OpXor, OpCmp:
			return cpu.executeALUOp(instr)
		case OpLoad:
			return cpu.executeLoad(instr)
		case OpStore:
			return cpu.executeStore(instr)
		case OpJmp, OpJz, OpJnz, OpJc:
			return cpu.executeJump(instr)
		case OpMov:
			// Для MOV в shell режиме используем executeMovInstr
			return cpu.executeMov(instr)
		default:
			return fmt.Errorf("unknown command: %s", cmd)
		}
	}
}

func (cpu *CPUContext) adaptToTextExecutor(instr Instruction) error {
	var cmd string
	var args []string

	switch instr.OpCode {
	case OpMov:
		cmd = "mov"
		args = []string{fmt.Sprintf("r%d", instr.Reg1)}
		if instr.Reg2 >= 0 {
			args = append(args, fmt.Sprintf("r%d", instr.Reg2))
		} else {
			args = append(args, bitsToStr(instr.Imm))
		}
	case OpAdd:
		cmd = "add"
		args = []string{fmt.Sprintf("r%d", instr.Reg1), fmt.Sprintf("r%d", instr.Reg2)}
	case OpSub:
		cmd = "sub"
		args = []string{fmt.Sprintf("r%d", instr.Reg1), fmt.Sprintf("r%d", instr.Reg2)}
	case OpMul:
		cmd = "mul"
		args = []string{fmt.Sprintf("r%d", instr.Reg1), fmt.Sprintf("r%d", instr.Reg2)}
	case OpAnd:
		cmd = "and"
		args = []string{fmt.Sprintf("r%d", instr.Reg1), fmt.Sprintf("r%d", instr.Reg2)}
	case OpOr:
		cmd = "or"
		args = []string{fmt.Sprintf("r%d", instr.Reg1), fmt.Sprintf("r%d", instr.Reg2)}
	case OpXor:
		cmd = "xor"
		args = []string{fmt.Sprintf("r%d", instr.Reg1), fmt.Sprintf("r%d", instr.Reg2)}
	case OpCmp:
		cmd = "cmp"
		args = []string{fmt.Sprintf("r%d", instr.Reg1), fmt.Sprintf("r%d", instr.Reg2)}
	case OpLoad:
		cmd = "load"
		args = []string{fmt.Sprintf("r%d", instr.Reg1), fmt.Sprintf("[r%d]", instr.Reg2)}
	case OpStore:
		cmd = "store"
		args = []string{fmt.Sprintf("[r%d]", instr.Reg1), fmt.Sprintf("r%d", instr.Reg2)}
	case OpJmp:
		cmd = "jmp"
		if instr.Label != "" {
			args = []string{instr.Label}
		} else {
			args = []string{fmt.Sprintf("r%d", instr.Reg1)}
		}
	case OpJz:
		cmd = "jz"
		if instr.Label != "" {
			args = []string{instr.Label}
		} else {
			args = []string{fmt.Sprintf("r%d", instr.Reg1)}
		}
	case OpJnz:
		cmd = "jnz"
		if instr.Label != "" {
			args = []string{instr.Label}
		} else {
			args = []string{fmt.Sprintf("r%d", instr.Reg1)}
		}
	case OpJc:
		cmd = "jc"
		if instr.Label != "" {
			args = []string{instr.Label}
		} else {
			args = []string{fmt.Sprintf("r%d", instr.Reg1)}
		}
	default:
		return fmt.Errorf("unknown opcode: %d", instr.OpCode)
	}
	fullCmd := strings.Join(append([]string{cmd}, args...), " ")
	return cpu.executeCommand(fullCmd)
}
