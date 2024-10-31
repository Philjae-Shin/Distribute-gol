package main

import (
	"flag"
	"math/rand"
	"net"
	"net/rpc"
	"os"
	"time"

	"uk.ac.bris.cs/gameoflife/stubs"
	"uk.ac.bris.cs/gameoflife/util"
)

// analogue to updateWorld function
/** Super-Secret `reversing a string' method we can't allow clients to see. **/
/*func ReverseString(s string, i int) string {
time.Sleep(time.DurationCall runes[j], runes[i]
}
return string(runes)
}*/

var listeners net.Listener
var pause bool
var waitToUnpause chan bool

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

func (s *GolOperations) ListenToQuit(req stubs.KillRequest, res *stubs.Response) (err error) {
	listeners.Close()
	os.Exit(0)
	return
}

func (s *GolOperations) ListenToPause(req stubs.PauseRequest, res *stubs.Response) (err error) {
	pause = req.Pause
	if !pause {
		waitToUnpause <- true
	}
	return
}

func (s *GolOperations) Process(req stubs.Request, res *stubs.Response) (err error) {

	if req.Turns == 0 {
		res.World = req.World
		res.TurnsDone = 0
		return
	}
	pause = false
	threads := 1
	turn := 0
	for t := 0; t < req.Turns; t++ {
		if pause {
			<-waitToUnpause
		}
		if !pause /*&& !quit*/ {
			turn = t
			if threads == 1 {
				res.World, _ = CalculateNextState(req.ImageHeight, req.ImageWidth, 0, req.ImageHeight, req.World)
			}
		} /*else {
			if quit {
				break
			} else {
				continue
			}
		}*/
	}

	res.TurnsDone = turn
	return
}

// kill := make(chan bool)

func main() {
	pAddr := flag.String("port", "8030", "Port to listen on")
	flag.Parse()
	rand.Seed(time.Now().UnixNano())
	rpc.Register(&GolOperations{})
	listener, _ := net.Listen("tcp", ":"+*pAddr)
	listeners = listener
	defer listener.Close()
	rpc.Accept(listener)

}
