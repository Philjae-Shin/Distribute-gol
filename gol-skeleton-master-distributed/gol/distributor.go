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

// 1 : Pass the keypress to the annonymous goroutine function below in the distributor (dis -> dis)
// 2 : Get the report from the broker. (Ticker request (dis -> broker))
// 3 : Connect with the broker, not the worker (Register (dis -> broker))
// 4 : Pass the keypress arguments to the broker. (Keypress (dis -> broker))

// TODO : Pass the keypress to the annonymous goroutine function below in the distributor
func handleKeyPress(p Params, c distributorChannels, keyPresses <-chan rune, world <-chan [][]uint8, t <-chan int, action chan int) {
	paused := false
	for {
		input := <-keyPresses

		switch input {
		case 's':
			action <- stubs.Save
			w := <-world
			turn := <-t
			go handleOutput(p, c, w, turn)

		case 'q':
			action <- stubs.Quit
			w := <-world
			turn := <-t
			go handleOutput(p, c, w, turn)

			newState := StateChange{CompletedTurns: turn, NewState: State(Quitting)}
			fmt.Println(newState.String())

			c.events <- newState
			c.events <- FinalTurnComplete{CompletedTurns: turn}
		case 'p':
			if paused {
				action <- stubs.UnPause
				turn := <-t
				paused = false
				newState := StateChange{CompletedTurns: turn, NewState: State(Executing)}
				fmt.Println(newState.String())
				c.events <- newState
			} else {
				action <- stubs.Pause
				turn := <-t
				paused = true
				newState := StateChange{CompletedTurns: turn, NewState: State(Paused)}
				fmt.Println(newState.String())
				c.events <- newState
			}

		case 'k':
			action <- stubs.Kill
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

// Commands IO to read the initial file, giving the filename via the channel.
func handleInput(p Params, c distributorChannels, world [][]uint8) [][]uint8 {
	filename := strconv.Itoa(p.ImageHeight) + "x" + strconv.Itoa(p.ImageWidth)
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
	return world
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

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels, keyPresses <-chan rune) {

	world := make([][]uint8, p.ImageHeight)
	for i := range world {
		world[i] = make([]uint8, p.ImageWidth)
	}

	world = handleInput(p, c, world)

	turn := 0
	ticker := time.NewTicker(2 * time.Second)
	done := make(chan bool)
	//pause := false
	quit := false

	//server := flag.String("server", "127.0.0.1:8030", "IP:port string to connect to as server")
	// Use Register Request (via RPC)
	flag.Parse()
	client, err := rpc.Dial("tcp", "127.0.0.1:8030")
	if err != nil {
		log.Fatal("dialing:", err)
	}
	defer client.Close()

	go func() {
		for {
			if !quit {
				select {
				case <-done:
					return
					// TODO : Get the report from the broker.
					// The distributor doesn't know the exact turn every time,
					// But only knows when some events happen (Ticker, save, ..)
				case <-ticker.C:

					// TODO : Get the report from the broker.
					tickerRequest := stubs.TickerRequest{}
					response := new(stubs.Response)
					errr := client.Call(stubs.Publish, tickerRequest, response)
					if errr != nil {
						fmt.Println("RPC client returned error:")
						fmt.Println(errr)
						fmt.Println("Shutting down miner.")
						break
					}
					world = response.World
					turn = response.TurnsDone
					fmt.Println("ticker")
					if turn == 0 {
						aliveReport := AliveCellsCount{
							CompletedTurns: turn,
							CellsCount:     0,
						}
						c.events <- aliveReport
					} else {
						aliveCount, _ := calculateAliveCells(p, world)
						aliveReport := AliveCellsCount{
							CompletedTurns: turn,
							CellsCount:     aliveCount,
						}
						c.events <- TurnComplete{
							CompletedTurns: turn,
						}
						c.events <- aliveReport
					}

					//fmt.Println("At turn", turn, "there are", aliveCount, "alive cells")
					//c.events <- aliveReport
				}
			} else {
				return
			}
		}
	}()

	turnChan := make(chan int)
	worldChan := make(chan [][]uint8)
	action := make(chan int)

	// TODO : Pass the keypress arguments to the broker.
	go handleKeyPress(p, c, keyPresses, worldChan, turnChan, action)
	go func() {
		for {
			select {
			case command := <-action:
				switch command {
				case stubs.Pause:
					//turnChan <- turn
					client.Call(stubs.ActionHandler, stubs.StateRequest{State: stubs.Pause}, new(stubs.StatusReport))

					turnChan <- turn
				case stubs.UnPause:
					//turnChan <- turn
					client.Call(stubs.ActionHandler, stubs.StateRequest{State: stubs.UnPause}, new(stubs.StatusReport))
					turnChan <- turn
				case stubs.Quit:
					res := new(stubs.Response)
					client.Call(stubs.ActionReport, stubs.StateRequest{State: stubs.Quit}, res)
					worldChan <- res.World
					turnChan <- res.TurnsDone
				case stubs.Save:
					res := new(stubs.Response)
					client.Call(stubs.ActionReport, stubs.StateRequest{State: stubs.Save}, res)
					worldChan <- res.World
					turnChan <- res.TurnsDone
				case stubs.Kill:
					res := new(stubs.Response)
					client.Go(stubs.ActionReport, stubs.StateRequest{State: stubs.Kill}, res, nil)
					worldChan <- world
					turnChan <- turn
				}
			}
		}
	}()

	chanreq := stubs.ChannelRequest{Threads: p.Threads}
	chanres := new(stubs.StatusReport)
	client.Call(stubs.MakeChannel, chanreq, chanres)

	request := stubs.Request{World: world,
		Threads:     p.Threads,
		Turns:       p.Turns,
		ImageWidth:  p.ImageWidth,
		ImageHeight: p.ImageHeight}
	response := new(stubs.Response)
	client.Call(stubs.ConnectDistributor, request, response)
	//time.Sleep(4 * time.Second)
	world = response.World
	turn = response.TurnsDone

	ticker.Stop()
	done <- true

	handleOutput(p, c, world, p.Turns)

	// Send the output and invoke writePgmImage() in io.go
	// Sends the world slice to io.go
	// TODO: Report the final state using FinalTurnCompleteEvent.

	aliveCells := make([]util.Cell, p.ImageHeight*p.ImageWidth)
	_, aliveCells = calculateAliveCells(p, world)
	report := FinalTurnComplete{
		CompletedTurns: turn,
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
