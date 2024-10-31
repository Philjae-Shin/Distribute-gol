package gol

import (
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

func handleOutput(p Params, c distributorChannels, world [][]uint8, t int) {
	c.ioCommand <- ioOutput
	outFilename := strconv.Itoa(p.ImageHeight) + "x" + strconv.Itoa(p.ImageWidth) + "x" + strconv.Itoa(t)
	c.ioFilename <- outFilename
	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			c.ioOutput <- world[y][x]
		}
	}
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle
}

func distributor(p Params, c distributorChannels, keyPresses <-chan rune) {
	world := make([][]uint8, p.ImageHeight)
	for i := range world {
		world[i] = make([]uint8, p.ImageWidth)
	}

	filename := strconv.Itoa(p.ImageHeight) + "x" + strconv.Itoa(p.ImageWidth)

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

	client, err := rpc.Dial("tcp", "YOUR_GOL_ENGINE_IP:8030")
	if err != nil {
		log.Fatal("Failed connecting to Gol Engine:", err)
	}
	defer client.Close()

	request := stubs.EngineRequest{
		World:       world,
		ImageWidth:  p.ImageWidth,
		ImageHeight: p.ImageHeight,
		Turns:       p.Turns,
	}
	response := new(stubs.EngineResponse)

	err = client.Call(stubs.Process, request, response)
	if err != nil {
		log.Fatal("Error calling Gol Engine:", err)
	}

	ticker := time.NewTicker(2 * time.Second)
	done := make(chan bool)
	quit := false

	go func() {
		for {
			select {
			case key := <-keyPresses:
				if key == 'q' {
					// 서버에 중지 요청
					stopRequest := stubs.StopRequest{}
					stopResponse := new(stubs.StopResponse)
					err := client.Call(stubs.StopProcessing, stopRequest, stopResponse)
					if err != nil {
						log.Println("Error calling StopProcessing:", err)
					}
					quit = true
					done <- true
					return
				}
			}
		}
	}()

	turn := 0

	go func() {
		for {
			if !quit {
				select {
				case <-done:
					return
				case <-ticker.C:
					// Get alive cells from server
					countRequest := stubs.AliveCellsCountRequest{}
					countResponse := new(stubs.AliveCellsCountResponse)
					err := client.Call(stubs.GetAliveCells, countRequest, countResponse)
					if err != nil {
						log.Println("Error calling GetAliveCells:", err)
					} else {
						aliveReport := AliveCellsCount{
							CompletedTurns: countResponse.CompletedTurns,
							CellsCount:     countResponse.CellsCount,
						}
						c.events <- aliveReport
						turn = countResponse.CompletedTurns
					}
				}
			} else {
				return
			}
		}
	}()

	<-done

	// Get FinalState from server
	finalWorldRequest := stubs.GetWorldRequest{}
	finalWorldResponse := new(stubs.GetWorldResponse)
	err = client.Call(stubs.GetWorld, finalWorldRequest, finalWorldResponse)
	if err != nil {
		log.Println("Error calling GetWorld:", err)
	} else {
		world = finalWorldResponse.World
	}

	handleOutput(p, c, world, turn)

	aliveCells := []util.Cell{}
	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			if world[y][x] == 255 {
				aliveCells = append(aliveCells, util.Cell{X: x, Y: y})
			}
		}
	}
	c.events <- FinalTurnComplete{
		CompletedTurns: turn,
		Alive:          aliveCells,
	}

	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	c.events <- StateChange{CompletedTurns: turn, NewState: Quitting}

	close(c.events)
}
