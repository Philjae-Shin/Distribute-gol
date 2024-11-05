// broker.go
package main

import (
	"fmt"
	"log"
	"net"
	"net/rpc"
	"sync"
	"time"

	"uk.ac.bris.cs/gameoflife/stubs"
	"uk.ac.bris.cs/gameoflife/util"
)

type Broker struct {
	mu           sync.Mutex
	workers      []*rpc.Client
	workerAddrs  []string
	world        [][]uint8
	height       int
	width        int
	turn         int
	totalTurns   int
	stop         bool
	processing   bool
	paused       bool
	shutdown     bool
	cellsFlipped []util.Cell
}

func (b *Broker) connectToWorkers(workerAddrs []string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.workerAddrs = workerAddrs
	b.workers = make([]*rpc.Client, len(workerAddrs))

	for i, addr := range workerAddrs {
		client, err := rpc.Dial("tcp", addr)
		if err != nil {
			return fmt.Errorf("failed to connect to worker at %s: %v", addr, err)
		}
		b.workers[i] = client
	}

	return nil
}

func (b *Broker) Process(req *stubs.EngineRequest, res *stubs.EngineResponse) error {
	b.mu.Lock()
	if b.processing {
		b.stop = true
		b.mu.Unlock()
		b.waitForProcessingToFinish()
		b.mu.Lock()
	}
	b.world = req.World
	b.height = req.ImageHeight
	b.width = req.ImageWidth
	b.turn = 0
	b.totalTurns = req.Turns
	b.stop = false
	b.processing = true
	b.paused = false
	b.shutdown = false
	b.mu.Unlock()

	go b.runSimulation()

	res.World = nil
	res.CompletedTurns = 0
	return nil
}

func (b *Broker) runSimulation() {
	for t := 0; t < b.totalTurns; t++ {
		b.mu.Lock()
		if b.stop || b.shutdown {
			b.processing = false
			b.mu.Unlock()
			break
		}
		for b.paused {
			b.mu.Unlock()
			time.Sleep(100 * time.Millisecond)
			b.mu.Lock()
		}
		b.mu.Unlock()

		err := b.distributeWork()
		if err != nil {
			log.Println("Error distributing work:", err)
			return
		}

		b.mu.Lock()
		b.turn = t + 1
		b.mu.Unlock()
	}

	b.mu.Lock()
	b.processing = false
	b.mu.Unlock()
}

func (b *Broker) distributeWork() error {
	b.mu.Lock()
	numWorkers := len(b.workers)
	rowsPerWorker := b.height / numWorkers
	remainder := b.height % numWorkers

	var wg sync.WaitGroup
	wg.Add(numWorkers)
	newWorld := make([][]uint8, b.height)
	for i := 0; i < b.height; i++ {
		newWorld[i] = make([]uint8, b.width)
	}

	b.cellsFlipped = []util.Cell{}

	for i := 0; i < numWorkers; i++ {
		startY := i * rowsPerWorker
		endY := startY + rowsPerWorker
		if i == numWorkers-1 {
			endY += remainder
		}
		workerWorld := make([][]uint8, endY-startY+2)
		for y := startY - 1; y <= endY; y++ {
			row := make([]uint8, b.width)
			copy(row, b.world[(y+b.height)%b.height])
			workerWorld[y-startY+1] = row
		}

		request := stubs.WorkerRequest{
			StartY:      startY,
			EndY:        endY,
			WorldSlice:  workerWorld,
			ImageWidth:  b.width,
			ImageHeight: b.height,
		}

		worker := b.workers[i]
		go func(worker *rpc.Client, request stubs.WorkerRequest, index int) {
			defer wg.Done()
			response := new(stubs.WorkerResponse)
			err := worker.Call(stubs.CalculateNextState, request, response)
			if err != nil {
				log.Printf("Error calling worker %d: %v", index, err)
				return
			}
			for y := request.StartY; y < request.EndY; y++ {
				copy(newWorld[y], response.WorldSlice[y-request.StartY])
			}
			b.mu.Lock()
			b.cellsFlipped = append(b.cellsFlipped, response.CellsFlipped...)
			b.mu.Unlock()
		}(worker, request, i)
	}
	b.mu.Unlock()

	wg.Wait()
	b.mu.Lock()
	b.world = newWorld
	b.mu.Unlock()

	return nil
}

func (b *Broker) GetTurnUpdates(req *stubs.TurnUpdatesRequest, res *stubs.TurnUpdatesResponse) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	res.CellsFlipped = b.cellsFlipped
	res.CompletedTurns = b.turn
	// Reset cellsFlipped for the next turn
	b.cellsFlipped = []util.Cell{}
	return nil
}

func (b *Broker) waitForProcessingToFinish() {
	b.mu.Lock()
	for b.processing {
		b.mu.Unlock()
		time.Sleep(100 * time.Millisecond)
		b.mu.Lock()
	}
	b.mu.Unlock()
}

func (b *Broker) GetWorld(req *stubs.GetWorldRequest, res *stubs.GetWorldResponse) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	res.World = b.world
	res.CompletedTurns = b.turn
	res.Processing = b.processing
	return nil
}

func main() {
	workerAddrs := []string{
		"3.89.244.38:8031",
		"100.27.232.29:8032",
		"3.88.218.20:8033",
		// Add more worker addresses as needed
	}

	broker := new(Broker)
	err := broker.connectToWorkers(workerAddrs)
	if err != nil {
		log.Fatal("Failed to connect to workers:", err)
	}

	rpc.Register(broker)
	listener, err := net.Listen("tcp", ":8030")
	if err != nil {
		log.Fatal("Error starting broker:", err)
	}
	defer listener.Close()
	log.Println("Broker listening on port 8030")
	rpc.Accept(listener)
}
