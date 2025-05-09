package main

import (
	"flag"
	"fmt"
	"log"
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
	fmt.Println("Добр пожаловать в эмулятор 'Emu4'!")
	fmt.Println("CPUx4 ")
	cpu := InitializeCPU()
	err := cpu.LoadProgram("start:\nmov r0, 5\nhlt")
	if err != nil {
		log.Fatal(err)
	}
	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, syscall.SIGINT, syscall.SIGTERM)
	go cpu.Run()

	go CommandShell(cpu)

	<-stopChan
	fmt.Println("\nЗавершение работы эмулятора...")
}
