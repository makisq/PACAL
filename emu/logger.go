package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

var (
	instance *log.Logger
	file     *os.File
	mu       sync.Mutex
	logLevel = 3 // 1=Error, 2=Warning, 3=Info, 4=Debug
)

const (
	LevelError = 1
	LevelWarn  = 2
	LevelInfo  = 3
	LevelDebug = 4
)

func Init() error {
	mu.Lock()
	defer mu.Unlock()
	multiWriter := io.MultiWriter(os.Stdout, file)
	instance = log.New(multiWriter, "", log.LstdFlags)
	if instance != nil {
		return nil
	}

	var err error
	file, err = os.OpenFile("cpu_pipeline.log",
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}

	instance = log.New(file, "", log.LstdFlags|log.Lmicroseconds)
	instance.Println("===== Logger initialized =====")
	return nil
}

func Close() {
	mu.Lock()
	defer mu.Unlock()

	if file != nil {
		file.Sync()
		file.Close()
		instance.Println("===== Logger closed =====")
		instance = nil
		file = nil
	}
}

func SetLevel(level int) {
	logLevel = level
}

func getCallerInfo() string {
	pc, file, line, ok := runtime.Caller(2)
	if !ok {
		return "unknown:0"
	}

	fn := runtime.FuncForPC(pc)
	fnName := "unknown"
	if fn != nil {
		fnName = strings.TrimPrefix(fn.Name(), "main.")
	}

	return fnName + "()@" + file + ":" + strconv.Itoa(line)
}

func logMessage(level int, prefix, msg string) {
	if level > logLevel {
		return
	}

	mu.Lock()
	defer mu.Unlock()

	if instance == nil {
		Init()
	}

	caller := getCallerInfo()
	instance.Printf("[%s] %-25s %s", prefix, caller, msg)

	if file != nil {
		file.Sync()
	}
}

func FuncStart(name string) {
	logMessage(LevelDebug, "START", name)
}

func FuncEnd(name string) {
	logMessage(LevelDebug, "END", name)
}

func PipelineStages(stage string, data interface{}) {
	logMessage(LevelInfo, "PIPELINE", fmt.Sprintf("%s: %+v", stage, data))
}

func PipelineError(stage string, err error) {
	logMessage(LevelError, "PIPELINE-ERR", fmt.Sprintf("%s: %v", stage, err))
}

func RegistersState(regs [4][4]bool) {
	logMessage(LevelDebug, "REGS", fmt.Sprintf("R0:%v R1:%v R2:%v R3:%v",
		regs[0], regs[1], regs[2], regs[3]))
}

func PCState(pc [4]bool) {
	logMessage(LevelDebug, "PC", fmt.Sprintf("%v (0b%b)", pc, bitsToInt(pc)))
}
func ALUDebug(msg string) {
	logMessage(LevelDebug, "ALU-DEBUG", msg)
}

func ALUOperation(op string, a, b, res [4]bool) {
	logMessage(LevelInfo, "ALU", fmt.Sprintf("%s %v + %v = %v", op, a, b, res))
}
