package main

import (
	"flag"
	"fmt"
	"math/rand"
	"net"
	"net/rpc"
	"time"

	"uk.ac.bris.cs/gameoflife/stubs"
	"uk.ac.bris.cs/gameoflife/util"
)

func mod(a, b int) int {
	return (a%b + b) % b
}

func calculateNeighbours(height, width int, world [][]byte, y int, x int) int {

	h := height
	w := width
	noOfNeighbours := 0

	neighbour := []byte{world[mod(y+1, h)][mod(x, w)], world[mod(y+1, h)][mod(x+1, w)], world[mod(y, h)][mod(x+1, w)],
		world[mod(y-1, h)][mod(x+1, w)], world[mod(y-1, h)][mod(x, w)], world[mod(y-1, h)][mod(x-1, w)],
		world[mod(y, h)][mod(x-1, w)], world[mod(y+1, h)][mod(x-1, w)]}

	for i := 0; i < 8; i++ {
		if neighbour[i] == 255 {
			noOfNeighbours++
		}
	}

	return noOfNeighbours
}

func CalculateNextState(height, width, startY, endY int, world [][]byte) ([][]byte, []util.Cell) {

	newWorld := make([][]byte, endY-startY)
	flipCell := make([]util.Cell, height, width)
	for i := 0; i < endY-startY; i++ {
		newWorld[i] = make([]byte, len(world[0]))
		// copy(newWorld[i], world[startY+i])
	}

	for y := 0; y < endY-startY; y++ {
		for x := 0; x < width; x++ {
			noOfNeighbours := calculateNeighbours(height, width, world, startY+y, x)
			if world[startY+y][x] == 255 {
				if noOfNeighbours < 2 {
					newWorld[y][x] = 0
					flipCell = append(flipCell, util.Cell{X: x, Y: startY + y})
				} else if noOfNeighbours == 2 || noOfNeighbours == 3 {
					newWorld[y][x] = 255
				} else if noOfNeighbours > 3 {
					newWorld[y][x] = 0
					flipCell = append(flipCell, util.Cell{X: x, Y: startY + y})
				}
			} else if world[startY+y][x] == 0 && noOfNeighbours == 3 {
				newWorld[y][x] = 255
				flipCell = append(flipCell, util.Cell{X: x, Y: startY + y})
			}
		}
	}

	return newWorld, flipCell
}

type GolOperations struct{}

func (s *GolOperations) Process(req stubs.Request, res *stubs.Response) (err error) {
	fmt.Println(req.Turns)
	if req.Turns == 0 {
		res.World = req.World
		res.TurnsDone = 0
		return
	}

	threads := 1
	turn := 0
	pause := false
	quit := false
	waitToUnpause := make(chan bool)
	/*go func() {
		for {

			select {
			case command := <-action:
				switch command {
				case Pause:
					pause = true
					turnChan <- turn
				case unPause:
					pause = false
					turnChan <- turn
					waitToUnpause <- true
				case Quit:
					worldChan <- world
					turnChan <- turn
					quit = true
					//return
				case Save:
					worldChan <- world
					turnChan <- turn
				}
			}
			//}
		}
	}()*/

	for t := 0; t < req.Turns; t++ {
		//cellFlip := make([]util.Cell, req.ImageHeight*req.ImageWidth)
		if pause {
			<-waitToUnpause
		}
		if !pause && !quit {
			turn = t

			if threads == 1 {
				req.World, _ = CalculateNextState(req.ImageHeight, req.ImageWidth, 0, req.ImageHeight, req.World)
				//req.World, cellFlip = CalculateNextState(req.ImageHeight, req.ImageWidth, 0, req.ImageHeight, req.World)
			}

			/*for _, cell := range cellFlip {
				// defer wg.Done()
				c.events <- CellFlipped{
					CompletedTurns: turn,
					Cell:           cell,
				}
			}

			c.events <- TurnComplete{
				CompletedTurns: turn,
			}*/

		} else {
			if quit {
				break
			} else {
				continue
			}
		}
		//fmt.Println(cellFlip)
	}

	res.World = req.World
	res.TurnsDone = turn
	return
}

func main() {
	pAddr := flag.String("port", "8030", "Port to listen on")
	flag.Parse()
	rand.Seed(time.Now().UnixNano())
	rpc.Register(&GolOperations{})
	listener, _ := net.Listen("tcp", ":"+*pAddr)
	defer listener.Close()
	rpc.Accept(listener)
}
