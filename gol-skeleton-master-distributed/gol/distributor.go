package gol

import (
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

	client, err := rpc.Dial("tcp", "34.204.100.63:8030")
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

	// 시뮬레이션 시작
	err = client.Call(stubs.Process, request, response)
	if err != nil {
		log.Fatal("Error calling Gol Engine:", err)
	}

	ticker := time.NewTicker(2 * time.Second)
	done := make(chan bool)
	processingDone := make(chan bool)

	// 키 입력 처리 고루틴
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
					done <- true
					return
				}
			}
		}
	}()

	// 2초마다 살아있는 셀 수 가져오기
	go func() {
		for {
			select {
			case <-ticker.C:
				// 서버에서 살아있는 셀 수 가져오기
				countRequest := stubs.AliveCellsCountRequest{}
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

	// 시뮬레이션 완료 대기
	go func() {
		// 시뮬레이션이 완료될 때까지 대기
		for {
			time.Sleep(1 * time.Second)
			getWorldRequest := stubs.GetWorldRequest{}
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

	// 시뮬레이션 종료 또는 완료 대기
	select {
	case <-done:
	case <-processingDone:
	}

	// 최종 세계 상태 가져오기
	finalWorldRequest := stubs.GetWorldRequest{}
	finalWorldResponse := new(stubs.GetWorldResponse)
	err = client.Call(stubs.GetWorld, finalWorldRequest, finalWorldResponse)
	if err != nil {
		log.Println("Error calling GetWorld:", err)
	} else {
		world = finalWorldResponse.World
		turn := finalWorldResponse.CompletedTurns

		// 최종 결과 처리
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

		c.ioCommand <- ioCheckIdle
		<-c.ioIdle

		c.events <- StateChange{CompletedTurns: turn, NewState: Quitting}
	}

	close(c.events)
}
