package main

func main() {
	cpu := InitializeCPU()
	defer cpu.Cleanup()
	go cpu.Run()
	CommandShell(cpu)
}
