package main

import (
	"flag"
	"fmt"
	"net"
	"net/rpc"
	"sync"

	"uk.ac.bris.cs/gameoflife/stubs"
)

type GolEngine struct {
	mu         sync.Mutex
	world      [][]uint8
	height     int
	width      int
	turn       int
	totalTurns int
	stop       bool
	processing bool
	paused     bool
	shutdown   bool
}

var cond = sync.NewCond(&sync.Mutex{})

func mod(a, b int) int {
	return (a%b + b) % b
}

func calculateNeighbours(world [][]uint8, x, y, width, height int) int {
	count := 0
	for deltaY := -1; deltaY <= 1; deltaY++ {
		for deltaX := -1; deltaX <= 1; deltaX++ {
			if deltaX == 0 && deltaY == 0 {
				continue
			}
			nx := mod(x+deltaX, width)
			ny := mod(y+deltaY, height)
			if world[ny][nx] == 255 {
				count++
			}
		}
	}
	return count
}

func (g *GolEngine) Process(req *stubs.EngineRequest, res *stubs.EngineResponse) error {
	g.mu.Lock()
	if g.processing {
		g.mu.Unlock()
		return fmt.Errorf("Already processing")
	}
	g.world = req.World
	g.height = req.ImageHeight
	g.width = req.ImageWidth
	g.turn = 0
	g.totalTurns = req.Turns
	g.stop = false
	g.processing = true
	g.paused = false
	g.shutdown = false
	g.mu.Unlock()

	go func() {
		for t := 0; t < g.totalTurns; t++ {
			g.mu.Lock()
			if g.stop || g.shutdown {
				g.processing = false
				g.mu.Unlock()
				break
			}
			for g.paused {
				cond.L.Lock()
				g.mu.Unlock()
				cond.Wait()
				cond.L.Unlock()
				g.mu.Lock()
			}
			g.mu.Unlock()

			// 한 턴 처리
			newWorld := make([][]uint8, g.height)
			for y := 0; y < g.height; y++ {
				newWorld[y] = make([]uint8, g.width)
				for x := 0; x < g.width; x++ {
					g.mu.Lock()
					if g.stop || g.shutdown {
						g.processing = false
						g.mu.Unlock()
						return
					}
					g.mu.Unlock()
					
					neighbours := calculateNeighbours(g.world, x, y, g.width, g.height)
					if g.world[y][x] == 255 {
						if neighbours == 2 || neighbours == 3 {
							newWorld[y][x] = 255
						} else {
							newWorld[y][x] = 0
						}
					} else {
						if neighbours == 3 {
							newWorld[y][x] = 255
						} else {
							newWorld[y][x] = 0
						}
					}
				}
			}
			g.mu.Lock()
			g.world = newWorld
			g.turn = t + 1
			g.mu.Unlock()
		}

		g.mu.Lock()
		g.processing = false
		g.mu.Unlock()
	}()

	res.World = nil
	res.CompletedTurns = 0
	return nil
}

func (g *GolEngine) Pause(req *stubs.PauseRequest, res *stubs.PauseResponse) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.processing || g.paused {
		return nil
	}
	g.paused = true
	res.Turn = g.turn
	return nil
}

func (g *GolEngine) Resume(req *stubs.ResumeRequest, res *stubs.ResumeResponse) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.processing || !g.paused {
		return nil
	}
	g.paused = false
	cond.Broadcast()
	return nil
}

func (g *GolEngine) Shutdown(req *stubs.ShutdownRequest, res *stubs.ShutdownResponse) error {
	g.mu.Lock()
	g.shutdown = true
	g.stop = true
	g.processing = false
	g.paused = false
	cond.Broadcast()
	g.mu.Unlock()
	return nil
}

func (g *GolEngine) GetWorld(req *stubs.GetWorldRequest, res *stubs.GetWorldResponse) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	res.World = g.world
	res.CompletedTurns = g.turn
	res.Processing = g.processing
	return nil
}

func (g *GolEngine) GetAliveCells(req *stubs.AliveCellsCountRequest, res *stubs.AliveCellsCountResponse) error {
	g.mu.Lock()
	count := 0
	for y := 0; y < g.height; y++ {
		for x := 0; x < g.width; x++ {
			if g.world[y][x] == 255 {
				count++
			}
		}
	}
	res.CellsCount = count
	res.CompletedTurns = g.turn
	g.mu.Unlock()
	return nil
}

func (g *GolEngine) StopProcessing(req *stubs.StopRequest, res *stubs.StopResponse) error {
	g.mu.Lock()
	g.stop = true
	g.processing = false
	g.mu.Unlock()
	return nil
}

func main() {
	pAddr := flag.String("port", "8030", "Port to listen on")
	flag.Parse()

	golEngine := new(GolEngine)
	rpc.Register(golEngine)
	listener, err := net.Listen("tcp", ":"+*pAddr)
	if err != nil {
		fmt.Println("Error starting server:", err)
		return
	}
	defer listener.Close()
	fmt.Println("Gol Engine listening on port", *pAddr)
	rpc.Accept(listener)
}
