package gol

import (
	"flag"
	"log"
	"net/rpc"
	"strconv"
	"uk.ac.bris.cs/gameoflife/stubs"

	"uk.ac.bris.cs/gameoflife/util"
)

type distributorChannels struct {
	events     chan<- Event
	ioCommand  chan<- ioCommand
	ioIdle     <-chan bool
	ioFilename chan<- string
	ioOutput   chan<- uint8
	ioInput    <-chan uint8
}

func calculateAliveCells(p Params, world [][]byte) (int, []util.Cell) {

	var aliveCells []util.Cell
	count := 0
	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			if world[y][x] == 255 {
				count++
				aliveCells = append(aliveCells, util.Cell{X: x, Y: y})
			}
		}
	}
	return count, aliveCells
}

func handleOutput(p Params, c distributorChannels, world [][]uint8, t int) {
	c.ioCommand <- 0
	outFilename := strconv.Itoa(p.ImageHeight) + "x" + strconv.Itoa(p.ImageWidth) + "x" + strconv.Itoa(t)
	c.ioFilename <- outFilename
	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			c.ioOutput <- world[y][x]
		}
	}
}

/*func makeCall(client *rpc.Client, message string) {
	request := stubs.Request{Message: message}
	response := new(stubs.Response)
	client.Call(stubs.ReverseHandler, request, response)
	fmt.Println("Responded: " + response.Message)
}*/

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels, keyPresses <-chan rune) {
	// TODO: Create a 2D slice to store the world.
	world := make([][]uint8, p.ImageHeight)
	prevWorld := make([][]uint8, p.ImageHeight)
	for i := range world {
		world[i] = make([]uint8, p.ImageWidth)
		prevWorld[i] = make([]uint8, p.ImageWidth)
	}

	filename := strconv.Itoa(p.ImageHeight) + "x" + strconv.Itoa(p.ImageWidth)

	// Commands IO to read the initial file, giving the filename via the channel.
	c.ioCommand <- 1
	c.ioFilename <- filename
	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			num := <-c.ioInput
			world[y][x] = num
			if num == 255 {
				c.events <- CellFlipped{
					CompletedTurns: 0,
					Cell:           util.Cell{X: x, Y: y},
				}
			}
		}
	}

	// TODO: Execute all turns of the Game of Life.
	turn := 0
	//ticker := time.NewTicker(2 * time.Second)
	//done := make(chan bool)
	//pause := false
	//quit := false
	//waitToUnpause := make(chan bool)
	/*go func() {
		for {
			if !quit {
				select {
				case <-done:
					return
				case <-ticker.C:
					aliveCount, _ := calculateAliveCells(p, prevWorld)
					aliveReport := AliveCellsCount{
						CompletedTurns: turn,
						CellsCount:     aliveCount,
					}
					c.events <- aliveReport
				}
			} else {
				return
			}
		}
	}()*/

	//server := flag.String("server", "127.0.0.1:8030", "IP:port string to connect to as server")
	flag.Parse()
	client, err := rpc.Dial("tcp", "127.0.0.1:8030")
	if err != nil {
		log.Fatal("dialing:", err)
	}
	defer client.Close()
	//makeCall(client, t)
	request := stubs.Request{World: world,
		PrevWorld:   prevWorld,
		Turns:       p.Turns,
		ImageWidth:  p.ImageWidth,
		ImageHeight: p.ImageHeight}
	response := new(stubs.Response)
	client.Call(stubs.ProcessTurnsHandler, request, response)
	world = response.World
	turn = response.TurnsDone

	//ticker.Stop()
	//done <- true

	//handleOutput(p, c, world, p.Turns)

	// Send the output and invoke writePgmImage() in io.go
	// Sends the world slice to io.go
	// TODO: Report the final state using FinalTurnCompleteEvent.

	aliveCells := make([]util.Cell, p.ImageHeight*p.ImageWidth)
	_, aliveCells = calculateAliveCells(p, world)
	report := FinalTurnComplete{
		CompletedTurns: p.Turns,
		Alive:          aliveCells,
	}
	c.events <- report
	// Make sure that the Io has finished any output before exiting.
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	c.events <- StateChange{turn, Quitting}

	// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
	close(c.events)
}
