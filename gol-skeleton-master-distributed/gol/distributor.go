package gol

import (
	"fmt"
	"log"
	"net/rpc"
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

func handleOutput(p Params, c distributorChannels, world [][]uint8, t int) {
	c.ioCommand <- ioOutput
	outFilename := fmt.Sprintf("%vx%vx%v", p.ImageWidth, p.ImageHeight, t)
	c.ioFilename <- outFilename
	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			c.ioOutput <- world[y][x]
		}
	}
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	c.events <- ImageOutputComplete{
		CompletedTurns: t,
		Filename:       outFilename,
	}
}

func distributor(p Params, c distributorChannels, keyPresses <-chan rune) {
	world := make([][]uint8, p.ImageHeight)
	for i := range world {
		world[i] = make([]uint8, p.ImageWidth)
	}

	filename := fmt.Sprintf("%vx%v", p.ImageWidth, p.ImageHeight)

	c.ioCommand <- ioInput
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

	client, err := rpc.Dial("tcp", "54.160.191.70:8030") // Connect to Broker
	if err != nil {
		log.Fatal("Failed connecting:", err)
	}
	defer client.Close()

	request := &stubs.EngineRequest{
		World:       world,
		ImageWidth:  p.ImageWidth,
		ImageHeight: p.ImageHeight,
		Turns:       p.Turns,
	}
	response := new(stubs.EngineResponse)

	err = client.Call(stubs.Process, request, response)
	if err != nil {
		log.Fatal("Error calling Process:", err)
	}

	paused := false
	done := false
	processingDone := false

	for !done && !processingDone {
		select {
		case key := <-keyPresses:
			switch key {
			case 's':
				// Save the current state
				getWorldRequest := &stubs.GetWorldRequest{}
				getWorldResponse := new(stubs.GetWorldResponse)
				err := client.Call(stubs.GetWorld, getWorldRequest, getWorldResponse)
				if err != nil {
					log.Println("Error calling GetWorld:", err)
				} else {
					worldSnapshot := getWorldResponse.World
					turn := getWorldResponse.CompletedTurns
					handleOutput(p, c, worldSnapshot, turn)
				}
			case 'q':
				done = true
				break
			case 'k':
				shutdownRequest := &stubs.ShutdownRequest{}
				shutdownResponse := new(stubs.ShutdownResponse)
				err := client.Call(stubs.Shutdown, shutdownRequest, shutdownResponse)
				if err != nil {
					log.Println("Error calling Shutdown:", err)
				}
				getWorldRequest := &stubs.GetWorldRequest{}
				getWorldResponse := new(stubs.GetWorldResponse)
				err = client.Call(stubs.GetWorld, getWorldRequest, getWorldResponse)
				if err != nil {
					log.Println("Error calling GetWorld:", err)
				} else {
					worldSnapshot := getWorldResponse.World
					turn := getWorldResponse.CompletedTurns
					handleOutput(p, c, worldSnapshot, turn)
				}
				done = true
				break
			case 'p':
				if !paused {
					pauseRequest := &stubs.PauseRequest{}
					pauseResponse := new(stubs.PauseResponse)
					err := client.Call(stubs.Pause, pauseRequest, pauseResponse)
					if err != nil {
						log.Println("Error calling Pause:", err)
					} else {
						fmt.Printf("Paused at turn %d\n", pauseResponse.Turn)
						paused = true
						c.events <- StateChange{
							CompletedTurns: pauseResponse.Turn,
							NewState:       Paused,
						}
					}
				} else {
					resumeRequest := &stubs.ResumeRequest{}
					resumeResponse := new(stubs.ResumeResponse)
					err := client.Call(stubs.Resume, resumeRequest, resumeResponse)
					if err != nil {
						log.Println("Error calling Resume:", err)
					} else {
						fmt.Println("Continuing")
						paused = false
						getWorldRequest := &stubs.GetWorldRequest{}
						getWorldResponse := new(stubs.GetWorldResponse)
						err = client.Call(stubs.GetWorld, getWorldRequest, getWorldResponse)
						if err == nil {
							c.events <- StateChange{
								CompletedTurns: getWorldResponse.CompletedTurns,
								NewState:       Executing,
							}
						}
					}
				}
			}
		default:
			if !paused {
				processTurnRequest := &stubs.ProcessTurnRequest{}
				processTurnResponse := new(stubs.ProcessTurnResponse)
				err := client.Call(stubs.ProcessTurn, processTurnRequest, processTurnResponse)
				if err != nil {
					log.Println("Error calling ProcessTurn:", err)
					processingDone = true
					break
				}

				// Send CellsFlipped event
				if len(processTurnResponse.FlippedCells) > 0 {
					c.events <- CellsFlipped{
						CompletedTurns: processTurnResponse.CompletedTurns,
						Cells:          processTurnResponse.FlippedCells,
					}
				}

				// Send AliveCellsCount event
				aliveReport := AliveCellsCount{
					CompletedTurns: processTurnResponse.CompletedTurns,
					CellsCount:     processTurnResponse.AliveCells,
				}
				c.events <- aliveReport

				if !processTurnResponse.Processing || processTurnResponse.CompletedTurns >= p.Turns {
					processingDone = true
					break
				}
			} else {
				time.Sleep(100 * time.Millisecond)
			}
		}
	}

	// After processing is done
	finalWorldRequest := &stubs.GetWorldRequest{}
	finalWorldResponse := new(stubs.GetWorldResponse)
	err = client.Call(stubs.GetWorld, finalWorldRequest, finalWorldResponse)
	if err != nil {
		log.Println("Error calling GetWorld:", err)
	} else {
		world = finalWorldResponse.World
		turn := finalWorldResponse.CompletedTurns

		aliveCells := []util.Cell{}
		for y := 0; y < p.ImageHeight; y++ {
			for x := 0; x < p.ImageWidth; x++ {
				if world[y][x] == 255 {
					aliveCells = append(aliveCells, util.Cell{X: x, Y: y})
				}
			}
		}

		handleOutput(p, c, world, turn)

		c.events <- FinalTurnComplete{
			CompletedTurns: turn,
			Alive:          aliveCells,
		}

		c.events <- StateChange{
			CompletedTurns: turn,
			NewState:       Quitting,
		}
	}

	close(c.events)
}
