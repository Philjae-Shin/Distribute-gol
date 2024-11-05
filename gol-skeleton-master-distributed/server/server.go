// server/server.go
package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/rpc"
	"sync"

	"uk.ac.bris.cs/gameoflife/stubs"
)

type GolWorker struct {
	mu                sync.Mutex
	prevWorkerAddr    string
	nextWorkerAddr    string
	prevWorkerClient  *rpc.Client
	nextWorkerClient  *rpc.Client
	topHaloRowChan    chan []uint8
	bottomHaloRowChan chan []uint8
	worldSlice        [][]uint8
	startY            int
	endY              int
	width             int
	height            int
	turns             int
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
			ny := y + deltaY
			if ny < 0 || ny >= height {
				continue
			}
			if world[ny][nx] == 255 {
				count++
			}
		}
	}
	return count
}

func (g *GolWorker) SetNeighbors(req *stubs.NeighborRequest, res *stubs.NeighborResponse) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.prevWorkerAddr = req.PrevWorkerAddr
	g.nextWorkerAddr = req.NextWorkerAddr

	if g.prevWorkerAddr != "" {
		prevClient, err := rpc.Dial("tcp", g.prevWorkerAddr)
		if err != nil {
			return fmt.Errorf("failed to connect to previous worker at %s: %v", g.prevWorkerAddr, err)
		}
		g.prevWorkerClient = prevClient
	}

	if g.nextWorkerAddr != "" {
		nextClient, err := rpc.Dial("tcp", g.nextWorkerAddr)
		if err != nil {
			return fmt.Errorf("failed to connect to next worker at %s: %v", g.nextWorkerAddr, err)
		}
		g.nextWorkerClient = nextClient
	}

	return nil
}

func (g *GolWorker) StartWorker(req *stubs.StartWorkerRequest, res *stubs.StartWorkerResponse) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.startY = req.StartY
	g.endY = req.EndY
	g.worldSlice = req.WorldSlice
	g.width = req.ImageWidth
	g.height = len(g.worldSlice)
	g.turns = req.Turns

	// Initialize channels
	g.topHaloRowChan = make(chan []uint8)
	g.bottomHaloRowChan = make(chan []uint8)

	go g.runWorker()
	return nil
}

func (g *GolWorker) runWorker() {
	for t := 0; t < g.turns; t++ {
		// Send own top row to previous worker
		if g.prevWorkerClient != nil {
			req := stubs.HaloDataRequest{Row: g.worldSlice[0]}
			res := new(stubs.HaloDataResponse)
			go g.prevWorkerClient.Call(stubs.ReceiveBottomHalo, req, res)
		}

		// Send own bottom row to next worker
		if g.nextWorkerClient != nil {
			req := stubs.HaloDataRequest{Row: g.worldSlice[len(g.worldSlice)-1]}
			res := new(stubs.HaloDataResponse)
			go g.nextWorkerClient.Call(stubs.ReceiveTopHalo, req, res)
		}

		// Receive halo rows from neighbors
		var topHaloRow, bottomHaloRow []uint8

		if g.prevWorkerClient != nil {
			bottomHaloRow = <-g.bottomHaloRowChan
		} else {
			// For edge workers, wrap around
			bottomHaloRow = g.worldSlice[len(g.worldSlice)-1]
		}

		if g.nextWorkerClient != nil {
			topHaloRow = <-g.topHaloRowChan
		} else {
			// For edge workers, wrap around
			topHaloRow = g.worldSlice[0]
		}

		// Process the iteration
		extendedWorld := make([][]uint8, len(g.worldSlice)+2)
		copy(extendedWorld[1:len(extendedWorld)-1], g.worldSlice)
		extendedWorld[0] = topHaloRow
		extendedWorld[len(extendedWorld)-1] = bottomHaloRow

		newWorldSlice := make([][]uint8, len(g.worldSlice))
		for y := 1; y < len(extendedWorld)-1; y++ {
			newRow := make([]uint8, g.width)
			for x := 0; x < g.width; x++ {
				neighbours := calculateNeighbours(extendedWorld, x, y, g.width, len(extendedWorld))
				if extendedWorld[y][x] == 255 {
					if neighbours == 2 || neighbours == 3 {
						newRow[x] = 255
					} else {
						newRow[x] = 0
					}
				} else {
					if neighbours == 3 {
						newRow[x] = 255
					} else {
						newRow[x] = 0
					}
				}
			}
			newWorldSlice[y-1] = newRow
		}
		g.mu.Lock()
		g.worldSlice = newWorldSlice
		g.mu.Unlock()
	}
}

func (g *GolWorker) ReceiveTopHalo(req *stubs.HaloDataRequest, res *stubs.HaloDataResponse) error {
	g.topHaloRowChan <- req.Row
	return nil
}

func (g *GolWorker) ReceiveBottomHalo(req *stubs.HaloDataRequest, res *stubs.HaloDataResponse) error {
	g.bottomHaloRowChan <- req.Row
	return nil
}

func (g *GolWorker) GetFinalSlice(req *stubs.GetFinalSliceRequest, res *stubs.GetFinalSliceResponse) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	res.WorldSlice = g.worldSlice
	return nil
}

func main() {
	pAddr := flag.String("port", "8031", "Port to listen on")
	flag.Parse()

	golWorker := new(GolWorker)
	rpc.RegisterName("GolWorker", golWorker)

	listener, err := net.Listen("tcp", ":"+*pAddr)
	if err != nil {
		log.Fatal("Error starting Gol worker:", err)
	}
	defer listener.Close()
	fmt.Println("Gol Worker listening on port", *pAddr)
	rpc.Accept(listener)
}
