package gol

import (
	"strconv"
	"sync"
	"time"

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

func calculateNextState(height, width, startY, endY int, world [][]byte) ([][]byte, []util.Cell) {

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
	c.ioCommand <- ioOutput
	outFilename := strconv.Itoa(p.ImageHeight) + "x" + strconv.Itoa(p.ImageWidth) + "x" + strconv.Itoa(t)
	c.ioFilename <- outFilename
	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			c.ioOutput <- world[y][x]
		}
	}

	// Wait for IO to finish
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle
	c.events <- ImageOutputComplete{
		CompletedTurns: t,
		Filename:       outFilename,
	}
}

func handleInput(p Params, c distributorChannels, world [][]uint8) [][]uint8 {
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
	return world
}

func handleKeyPress(p Params, c distributorChannels, keyPresses <-chan rune, action chan int) {
	paused := false
	for {
		input := <-keyPresses
		switch input {
		case 's':
			action <- Save
		case 'q':
			action <- Quit
			return
		case 'p':
			if paused {
				action <- unPause
				paused = false
			} else {
				action <- Pause
				paused = true
			}
		}
	}
}

func distributor(p Params, c distributorChannels, keyPresses <-chan rune) {
	world := make([][]uint8, p.ImageHeight)
	prevWorld := make([][]uint8, p.ImageHeight)
	for i := range world {
		world[i] = make([]uint8, p.ImageWidth)
		prevWorld[i] = make([]uint8, p.ImageWidth)
	}

	world = handleInput(p, c, world)

	turn := 0
	ticker := time.NewTicker(2 * time.Second)
	done := make(chan bool)
	pause := false
	quit := false
	finished := false

	var mu sync.Mutex

	action := make(chan int)

	go handleKeyPress(p, c, keyPresses, action)

	// Send StateChange event indicating Executing state at the start
	c.events <- StateChange{CompletedTurns: turn, NewState: Executing}

	// Start ticker for AliveCellsCount events
	go func() {
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				mu.Lock()
				snapshot := make([][]uint8, p.ImageHeight)
				for i := range prevWorld {
					snapshot[i] = make([]uint8, p.ImageWidth)
					copy(snapshot[i], prevWorld[i])
				}
				currentTurn := turn
				mu.Unlock()
				aliveCount, _ := calculateAliveCells(p, snapshot)
				c.events <- AliveCellsCount{
					CompletedTurns: currentTurn,
					CellsCount:     aliveCount,
				}
			}
		}
	}()

	for !finished && (turn < p.Turns || pause) {
		if pause {
			select {
			case command := <-action:
				switch command {
				case unPause:
					pause = false
					// Send StateChange event indicating Executing state
					c.events <- StateChange{CompletedTurns: turn, NewState: Executing}
				case Quit:
					quit = true
					finished = true
				case Save:
					mu.Lock()
					snapshot := make([][]uint8, p.ImageHeight)
					for i := range world {
						snapshot[i] = make([]uint8, p.ImageWidth)
						copy(snapshot[i], world[i])
					}
					currentTurn := turn
					mu.Unlock()
					handleOutput(p, c, snapshot, currentTurn)
				}
			default:
				// Sleep briefly to prevent busy waiting
				time.Sleep(100 * time.Millisecond)
			}
			continue
		}

		select {
		case command := <-action:
			switch command {
			case Pause:
				pause = true
				// Send StateChange event indicating Paused state
				c.events <- StateChange{CompletedTurns: turn, NewState: Paused}
			case Quit:
				quit = true
				finished = true
			case Save:
				mu.Lock()
				snapshot := make([][]uint8, p.ImageHeight)
				for i := range world {
					snapshot[i] = make([]uint8, p.ImageWidth)
					copy(snapshot[i], world[i])
				}
				currentTurn := turn
				mu.Unlock()
				handleOutput(p, c, snapshot, currentTurn)
			}
		default:
			if !quit && turn < p.Turns {
				mu.Lock()
				for i := range world {
					copy(prevWorld[i], world[i])
				}
				mu.Unlock()
				var flipFragment []util.Cell
				world, flipFragment = calculateNextState(p.ImageHeight, p.ImageWidth, 0, p.ImageHeight, prevWorld)
				for _, cell := range flipFragment {
					c.events <- CellFlipped{
						CompletedTurns: turn,
						Cell:           cell,
					}
				}
				mu.Lock()
				turn++
				mu.Unlock()
				c.events <- TurnComplete{CompletedTurns: turn}
			} else if quit {
				break
			} else if turn >= p.Turns {
				break
			}
		}
	}

	ticker.Stop()
	done <- true

	_, aliveCells := calculateAliveCells(p, world)

	handleOutput(p, c, world, turn)

	if quit {
		c.events <- StateChange{CompletedTurns: turn, NewState: Quitting}
	}
	c.events <- FinalTurnComplete{
		CompletedTurns: turn,
		Alive:          aliveCells,
	}

	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	close(c.events)
}
