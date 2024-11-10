package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/rpc"
	"sync"

	"uk.ac.bris.cs/gameoflife/stubs"
	"uk.ac.bris.cs/gameoflife/util"
)

type GolWorker struct {
	mu sync.Mutex
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

func (g *GolWorker) CalculateNextState(req *stubs.WorkerRequest, res *stubs.WorkerResponse) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	worldSlice := req.WorldSlice
	height := len(worldSlice)
	width := req.ImageWidth

	newWorldSlice := make([][]uint8, height-2)
	cellsFlipped := []util.Cell{}

	for y := 1; y < height-1; y++ {
		newRow := make([]uint8, width)
		for x := 0; x < width; x++ {
			neighbours := calculateNeighbours(worldSlice, x, y, width, height)
			newValue := worldSlice[y][x]
			if worldSlice[y][x] == 255 {
				if neighbours == 2 || neighbours == 3 {
					newValue = 255
				} else {
					newValue = 0
				}
			} else {
				if neighbours == 3 {
					newValue = 255
				} else {
					newValue = 0
				}
			}
			newRow[x] = newValue
			// Compare old and new values
			if newValue != worldSlice[y][x] {
				globalY := req.StartY + (y - 1)
				cell := util.Cell{X: x, Y: globalY}
				cellsFlipped = append(cellsFlipped, cell)
			}
		}
		newWorldSlice[y-1] = newRow
	}

	res.WorldSlice = newWorldSlice
	res.CellsFlipped = cellsFlipped
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
