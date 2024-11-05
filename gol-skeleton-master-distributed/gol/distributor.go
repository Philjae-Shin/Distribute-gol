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

	// 이벤트 전송
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
	client, err := rpc.Dial("tcp", "54.226.73.38:8030") // Connect to Broker
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

	// 시뮬레이션 시작
	err = client.Call(stubs.Process, request, response)
	if err != nil {
		log.Fatal("Error calling Process:", err)
	}

	ticker := time.NewTicker(2 * time.Second)
	done := make(chan bool)
	processingDone := make(chan bool)
	paused := false

	// 키 입력 처리 고루틴
	go func() {
		for {
			select {
			case key := <-keyPresses:
				switch key {
				case 's':
					// 현재 상태를 가져와서 저장
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
					// 프로그램 종료
					done <- true
					return
				case 'k':
					// 서버 종료 요청 및 프로그램 종료
					shutdownRequest := &stubs.ShutdownRequest{}
					shutdownResponse := new(stubs.ShutdownResponse)
					err := client.Call(stubs.Shutdown, shutdownRequest, shutdownResponse)
					if err != nil {
						log.Println("Error calling Shutdown:", err)
					}
					// 현재 상태 저장
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
						// 일시 중지 요청
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
						// 재개 요청
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

	// 2초마다 살아있는 셀 수 가져오기
	go func() {
		for {
			select {
			case <-ticker.C:
				// 서버에서 살아있는 셀 수 가져오기
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
			time.Sleep(100 * time.Millisecond)
			getFlippedCellsRequest := &stubs.GetFlippedCellsRequest{}
			getFlippedCellsResponse := new(stubs.GetFlippedCellsResponse)
			err := client.Call(stubs.GetFlippedCells, getFlippedCellsRequest, getFlippedCellsResponse)
			if err != nil {
				log.Println("Error calling GetFlippedCells:", err)
			} else {
				if len(getFlippedCellsResponse.FlippedCells) > 0 {
					c.events <- CellsFlipped{
						CompletedTurns: getFlippedCellsResponse.CompletedTurns,
						Cells:          getFlippedCellsResponse.FlippedCells,
					}
					c.events <- TurnComplete{
						CompletedTurns: getFlippedCellsResponse.CompletedTurns,
					}
				}
				getWorldRequest := &stubs.GetWorldRequest{}
				getWorldResponse := new(stubs.GetWorldResponse)
				err := client.Call(stubs.GetWorld, getWorldRequest, getWorldResponse)
				if err == nil {
					if !getWorldResponse.Processing {
						processingDone <- true
						return
					}
				}
			}
		}
	}()

	// 시뮬레이션 완료 대기
	go func() {
		// 시뮬레이션이 완료될 때까지 대기
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

	// 시뮬레이션 종료 또는 완료 대기
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

	// 최종 세계 상태 가져오기
	finalWorldRequest := &stubs.GetWorldRequest{}
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
				num := <-c.ioInput
				world[y][x] = num
				if world[y][x] == 255 {
					aliveCells = append(aliveCells, util.Cell{X: x, Y: y})
				}
			}
		}

		// Send CellsFlipped event for initial alive cells
		if len(aliveCells) > 0 {
			c.events <- CellsFlipped{
				CompletedTurns: 0,
				Cells:          aliveCells,
			}
		}
		c.events <- TurnComplete{CompletedTurns: 0}

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
