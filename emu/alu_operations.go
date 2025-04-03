package main

import (
	"errors"
	"fmt"
	"time"
)

type ALU struct {
	debugMode  bool
	lastOpTime time.Duration
	op         int
	aBits      []bool
	bBits      []bool
	flags      struct {
		carry bool
		zero  bool
	}
	interrupts InterruptState
}

func (a *ALU) Execute() (result [4]bool, flags struct{ carry, zero bool }, err error) {
	if len(a.aBits) != 4 || len(a.bBits) != 4 {
		return [4]bool{}, struct{ carry, zero bool }{}, errors.New("invalid operand length")
	}

	start := time.Now()
	defer func() {
		a.lastOpTime = time.Since(start)
	}()

	switch a.op {
	case OpAdd:
		return a.add()
	case OpSub:
		return a.subtract()
	case OpMul:
		return a.multiply()
	case OpAnd, OpOr, OpXor:
		return a.logicalOp()
	case OpCmp:
		return a.compare()
	default:
		return [4]bool{}, struct{ carry, zero bool }{}, errors.New("unsupported ALU operation")
	}
}

func (a *ALU) add() ([4]bool, struct{ carry, zero bool }, error) {
	const maxSteps = 10
	step := 0

	if len(a.aBits) != 4 || len(a.bBits) != 4 {
		return [4]bool{}, struct{ carry, zero bool }{},
			fmt.Errorf("invalid input length: a=%d bits, b=%d bits", len(a.aBits), len(a.bBits))
	}

	var result [4]bool
	carry := false

	for i := 0; i < 4; i++ {
		step++
		if step > maxSteps {
			return [4]bool{}, struct{ carry, zero bool }{},
				fmt.Errorf("timeout after %d steps", maxSteps)
		}

		aBit := a.aBits[i]
		bBit := a.bBits[i]
		sum := (aBit != bBit) != carry
		newCarry := (aBit && bBit) || (carry && (aBit != bBit))

		result[i] = sum
		carry = newCarry
	}

	zero := !(result[0] || result[1] || result[2] || result[3])
	return result, struct{ carry, zero bool }{carry, zero}, nil
}

func (a *ALU) logicalOp() ([4]bool, struct{ carry, zero bool }, error) {
	var result [4]bool

	for i := 0; i < 4; i++ {
		switch a.op {
		case OpAnd:
			result[i] = a.aBits[i] && a.bBits[i]
		case OpOr:
			result[i] = a.aBits[i] || a.bBits[i]
		case OpXor:
			result[i] = a.aBits[i] != a.bBits[i]
		}
	}

	zero := !(result[0] || result[1] || result[2] || result[3])
	return result, struct{ carry, zero bool }{false, zero}, nil
}

func (a *ALU) subtract() ([4]bool, struct{ carry, zero bool }, error) {
	var bComplement [4]bool
	carry := true
	for i := 0; i < 4; i++ {
		sum := (!a.bBits[i] != carry)
		carry = (!a.bBits[i] && carry)
		bComplement[i] = sum
	}

	var result [4]bool
	carry = false
	for i := 0; i < 4; i++ {
		sum := (a.aBits[i] != bComplement[i]) != carry
		carry = (a.aBits[i] && bComplement[i]) || (carry && (a.aBits[i] != bComplement[i]))
		result[i] = sum
	}

	zero := !(result[0] || result[1] || result[2] || result[3])
	return result, struct{ carry, zero bool }{carry, zero}, nil
}

func (a *ALU) multiply() ([4]bool, struct{ carry, zero bool }, error) {
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
	result := [4]bool{
		res&1 != 0,
		res&2 != 0,
		res&4 != 0,
		res&8 != 0,
	}

	carry := res > 15
	zero := res == 0

	return result, struct{ carry, zero bool }{carry, zero}, nil
}

func (a *ALU) compare() ([4]bool, struct{ carry, zero bool }, error) {
	start := time.Now()
	defer func() {
		a.lastOpTime = time.Since(start)
		if a.debugMode {
			fmt.Printf("\r[CMP: %v]", a.lastOpTime)
		}
	}()

	currentA := sliceTo4Bool(a.aBits)
	currentB := sliceTo4Bool(a.bBits)
	if currentA == cmpCache.a && currentB == cmpCache.b &&
		time.Since(cmpCache.timestamp) < time.Second {
		a.flags.carry = cmpCache.result.carry
		a.flags.zero = cmpCache.result.zero
		return [4]bool{}, struct{ carry, zero bool }{a.flags.carry, a.flags.zero}, nil
	}
	originalA := make([]bool, len(a.aBits))
	originalB := make([]bool, len(a.bBits))
	copy(originalA, a.aBits)
	copy(originalB, a.bBits)
	_, flags, err := a.subtract()
	if err != nil {
		return [4]bool{}, struct{ carry, zero bool }{}, err
	}
	cmpCache.a = sliceTo4Bool(originalA)
	cmpCache.b = sliceTo4Bool(originalB)
	cmpCache.result.carry = flags.carry
	cmpCache.result.zero = flags.zero
	cmpCache.timestamp = time.Now()
	a.aBits = originalA
	a.bBits = originalB
	return [4]bool{}, flags, nil
}
