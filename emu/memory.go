package main

import (
	"fmt"
	"sync"
	"time"
)

type MemoryDevice interface {
	Read(addr [4]bool) [4]bool
	Write(addr [4]bool, data [4]bool, clock bool)
}

type RAM16x4 struct {
	cells       [16]Register4bit
	writeEnable bool
}

type ROM16x4 struct {
	data [16][4]bool
}

type MemoryBus struct {
	rom         *ROM16x4
	ram         *RAM16x4
	terminal    *Terminal
	devices     []MemoryDevice
	clock       chan bool
	romSize     int
	ramSize     int
	cache       map[int][4]bool
	cacheSize   int
	cacheLock   sync.Mutex
	cacheHits   float32
	cacheMisses float32
}

type cacheEntry struct {
	data     [4]bool
	lastUsed time.Time
}

func (mb *MemoryBus) Stats() string {
	total := mb.cacheHits + mb.cacheMisses
	if total == 0 {
		return "Cache: no accesses yet"
	}
	return fmt.Sprintf("Cache: %d/%d (%.1f%%)",
		mb.cacheHits, total, 100*float32(mb.cacheHits)/float32(total))
}

func (mb *MemoryBus) adjustCacheSize() {
	hitRate := mb.cacheHits / (mb.cacheHits + mb.cacheMisses)
	if hitRate > 0.7 {
		mb.cacheSize += 10
	} else if hitRate < 0.3 {
		mb.cacheSize -= 10
	}
}

type Syncable interface {
	Sync()
}

func (r *RAM16x4) Sync() {
	r.writeEnable = false
}

func NewMemoryBus(rom *ROM16x4, ram *RAM16x4, terminal *Terminal, romSize int, ramSize int) *MemoryBus {
	mb := &MemoryBus{
		rom:       rom,
		ram:       ram,
		terminal:  terminal,
		devices:   []MemoryDevice{rom, ram},
		romSize:   romSize,
		ramSize:   ramSize,
		cache:     make(map[int][4]bool),
		cacheSize: 32,
	}

	mb.devices = append(mb.devices, terminal)

	return mb
}

func (mb *MemoryBus) Sync(clock chan bool) {
	go func() {
		for {
			<-clock
			mb.clock <- true
			for _, dev := range mb.devices {
				if syncDev, ok := dev.(Syncable); ok {
					syncDev.Sync()
				}
			}
		}
	}()
}

func (r *ROM16x4) Write(addr [4]bool, data [4]bool, clock bool) {

}

func (r *RAM16x4) Write(addr [4]bool, data [4]bool, clock bool) {
	addrInt := bitsToInt(addr)
	if addrInt >= 0 && addrInt < 16 && r.writeEnable {
		r.cells[addrInt].Write(data, clock, false)
	}
}

func (r *RAM16x4) Read(addr [4]bool) [4]bool {
	addrInt := bitsToInt(addr)
	if addrInt >= 0 && addrInt < 16 {
		return r.cells[addrInt].Read()
	}
	return [4]bool{false, false, false, false}
}

func NewROM16x4(initialData [16][4]bool) *ROM16x4 {
	return &ROM16x4{data: initialData}
}

func (r *ROM16x4) Read(addr [4]bool) [4]bool {
	addrInt := bitsToInt(addr)
	if addrInt >= 0 && addrInt < 16 {
		return r.data[addrInt]
	}
	return [4]bool{false, false, false, false}
}

func (mb *MemoryBus) Read(addr [4]bool) [4]bool {
	addrInt := bitsToInt(addr)
	if addrInt < 0 {
		return [4]bool{false, false, false, false}
	}

	switch addrInt {
	case 0xF000:
		return byteTo4Bits(mb.terminal.readKeyUnsafe())
	case 0xF001:
		return [4]bool{len(mb.terminal.KeyBuffer) > 0, false, false, false}
	}

	if addrInt < mb.romSize {
		return mb.rom.Read(addr)
	} else if addrInt < mb.romSize+mb.ramSize {
		adjustedAddr := intTo4Bits(addrInt - mb.romSize)
		return mb.ram.Read(adjustedAddr)
	}

	return [4]bool{false, false, false, false}
}

func (mb *MemoryBus) Write(addr [4]bool, data [4]bool, clock bool) {
	addrInt := bitsToInt(addr)

	switch addrInt {
	case 0xF002:
		mb.terminal.WriteChar(bitsToByte(data))
		return
	}

	if addrInt >= mb.romSize && addrInt < mb.romSize+mb.ramSize {
		adjustedAddr := intTo4Bits(addrInt - mb.romSize)
		mb.ram.Write(adjustedAddr, data, clock)
	}
}

func bitsToByte(bits [4]bool) byte {
	var b byte
	for i, bit := range bits {
		if bit {
			b |= 1 << (3 - i)
		}
	}
	return b
}

func byteTo4Bits(b byte) [4]bool {
	return [4]bool{
		(b & 8) > 0,
		(b & 4) > 0,
		(b & 2) > 0,
		(b & 1) > 0,
	}
}
