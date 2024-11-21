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

	//client, err := rpc.Dial("tcp", "localhost:8030") Connect to AWS instance
	client, err := rpc.Dial("tcp", "34.204.50.250:8030") // Connect to Broker
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

	ticker := time.NewTicker(2 * time.Second)
	done := make(chan bool)
	processingDone := make(chan bool)
	paused := false

	go func() {
		for {
			select {
			case key := <-keyPresses:
				switch key {
				case 's':
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
					done <- true
					return
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
					done <- true
					return
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
			}
		}
	}()

	go func() {
		for {
			select {
			case <-ticker.C:
				countRequest := &stubs.AliveCellsCountRequest{}
				countResponse := new(stubs.AliveCellsCountResponse)
				err := client.Call(stubs.GetAliveCells, countRequest, countResponse)
				if err != nil {
					return
				} else {
					if countResponse.CompletedTurns > 0 {
						aliveReport := AliveCellsCount{
							CompletedTurns: countResponse.CompletedTurns,
							CellsCount:     countResponse.CellsCount,
						}
						c.events <- aliveReport
					}
				}
			case <-done:
				ticker.Stop()
				return
			}
		}
	}()

	go func() {
		for {
			time.Sleep(1 * time.Second)
			getWorldRequest := &stubs.GetWorldRequest{}
			getWorldResponse := new(stubs.GetWorldResponse)
			err := client.Call(stubs.GetWorld, getWorldRequest, getWorldResponse)
			if err != nil {
				log.Println("Error calling GetWorld:", err)
			} else {
				if !getWorldResponse.Processing {
					processingDone <- true
					return
				}
			}
		}
	}()

	select {
	case <-done:
		stopRequest := &stubs.StopRequest{}
		stopResponse := new(stubs.StopResponse)
		err := client.Call(stubs.StopProcessing, stopRequest, stopResponse)
		if err != nil {
			log.Println("Error calling StopProcessing:", err)
		}
	case <-processingDone:
	}

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
