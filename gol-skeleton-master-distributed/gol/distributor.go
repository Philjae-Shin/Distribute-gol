package gol

import (
	"flag"
	"fmt"
	"log"
	"net/rpc"
	"strconv"
	"time"

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

const Save int = 0
const Quit int = 1
const Pause int = 2
const unPause int = 3
const Kill int = 4

func handleKeyPress(p Params, c distributorChannels, keyPresses <-chan rune, world <-chan [][]uint8, t <-chan int, action chan int) {
	paused := false
	for {
		input := <-keyPresses

		switch input {
		case 's':
			action <- Save
			w := <-world
			turn := <-t
			go handleOutput(p, c, w, turn)

		case 'q':
			action <- Quit
			w := <-world
			turn := <-t
			go handleOutput(p, c, w, turn)

			newState := StateChange{CompletedTurns: turn, NewState: State(Quitting)}
			fmt.Println(newState.String())

			c.events <- newState
			c.events <- FinalTurnComplete{CompletedTurns: turn}
		case 'p':
			if paused {
				action <- unPause
				turn := <-t
				paused = false
				newState := StateChange{CompletedTurns: turn, NewState: State(Executing)}
				fmt.Println(newState.String())
				c.events <- newState
			} else {
				action <- Pause
				turn := <-t
				paused = true
				newState := StateChange{CompletedTurns: turn, NewState: State(Paused)}
				fmt.Println(newState.String())
				c.events <- newState
			}

		case 'k':
			action <- Kill
			w := <-world
			turn := <-t
			go handleOutput(p, c, w, turn)
			newState := StateChange{CompletedTurns: turn, NewState: State(Quitting)}
			fmt.Println(newState.String())
			c.events <- newState
			c.events <- FinalTurnComplete{CompletedTurns: turn}
		}

	}

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
	for i := range world {
		world[i] = make([]uint8, p.ImageWidth)
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
	ticker := time.NewTicker(2 * time.Second)
	done := make(chan bool)
	// pause := false
	quit := false
	//waitToUnpause := make(chan bool)
	go func() {
		for {
			if !quit {
				select {
				case <-done:
					return
				case <-ticker.C:
					aliveCount, _ := calculateAliveCells(p, world)
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
	}()

	//server := flag.String("server", "127.0.0.1:8030", "IP:port string to connect to as server")
	flag.Parse()
	client, err := rpc.Dial("tcp", "127.0.0.1:8030")
	if err != nil {
		log.Fatal("dialing:", err)
	}
	defer client.Close()

	turnChan := make(chan int)
	worldChan := make(chan [][]uint8)
	action := make(chan int)

	go handleKeyPress(p, c, keyPresses, worldChan, turnChan, action)
	go func() {
		for {
			select {
			case command := <-action:
				switch command {
				case Pause:
					turnChan <- turn
				case unPause:
					turnChan <- turn
				case Quit:
					worldChan <- world
					turnChan <- turn
				case Save:
					worldChan <- world
					turnChan <- turn
				case Kill:
					worldChan <- world
					turnChan <- turn
					client.Go(stubs.KillingHandler, stubs.KillRequest{Kill: 0}, new(stubs.Response), nil)

				}
			}
		}
	}()

	for t := 0; t < p.Turns; t++ {
		turn = t
		//makeCall(client, t)
		request := stubs.Request{World: world,
			Turns:       p.Turns,
			ImageWidth:  p.ImageWidth,
			ImageHeight: p.ImageHeight}
		response := new(stubs.Response)
		client.Call(stubs.ProcessTurnsHandler, request, response)

		world = response.World
		// turn = response.TurnsDone
	}

	ticker.Stop()
	done <- true

	handleOutput(p, c, world, p.Turns)

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
