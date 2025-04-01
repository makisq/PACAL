package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	clockSpeed := flag.Duration("clock", time.Second, "Тактовая частота процессора")
	debugMode := flag.Bool("debug", false, "Режим отладки")
	flag.Parse()

	fmt.Printf("Эмулятор процессора v%s (commit %s, built %s)\n", version, commit, date)
	fmt.Printf("Тактовая частота: %v, Режим отладки: %v\n", *clockSpeed, *debugMode)
	cpu := InitializeCPU()
	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, syscall.SIGINT, syscall.SIGTERM)
	go cpu.Run()

	go CommandShell(cpu)

	<-stopChan
	fmt.Println("\nЗавершение работы эмулятора...")
}
