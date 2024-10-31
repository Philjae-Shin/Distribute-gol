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
}

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

func (g *GolEngine) Process(req stubs.EngineRequest, res *stubs.EngineResponse) error {
	g.mu.Lock()
	g.world = req.World
	g.height = req.ImageHeight
	g.width = req.ImageWidth
	g.turn = 0
	g.totalTurns = req.Turns
	g.stop = false
	g.mu.Unlock()

	for t := 0; t < g.totalTurns; t++ {
		g.mu.Lock()
		if g.stop {
			g.mu.Unlock()
			break
		}
		newWorld := make([][]uint8, g.height)
		for y := 0; y < g.height; y++ {
			newWorld[y] = make([]uint8, g.width)
			for x := 0; x < g.width; x++ {
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
		g.world = newWorld
		g.turn = t + 1
		g.mu.Unlock()
	}

	g.mu.Lock()
	res.World = g.world
	res.CompletedTurns = g.turn
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
