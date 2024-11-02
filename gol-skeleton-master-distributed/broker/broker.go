package main

import (
	"fmt"
	"log"
	"net"
	"net/rpc"
	"sync"
	"time"

	"uk.ac.bris.cs/gameoflife/stubs"
)

type Broker struct {
	mu          sync.Mutex
	workers     []*rpc.Client
	workerAddrs []string
	world       [][]uint8
	height      int
	width       int
	turn        int
	totalTurns  int
	stop        bool
	processing  bool
	paused      bool
	shutdown    bool
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
		// Previous simulation is running; stop it
		b.stop = true
		// Wait for it to finish
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
			// Wait until resumed
			b.mu.Unlock()
			time.Sleep(100 * time.Millisecond)
			b.mu.Lock()
		}
		b.mu.Unlock()

		// Distribute work to workers
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
	// Divide world into slices
	numWorkers := len(b.workers)
	rowsPerWorker := b.height / numWorkers
	remainder := b.height % numWorkers

	var wg sync.WaitGroup
	wg.Add(numWorkers)
	newWorld := make([][]uint8, b.height)
	for i := 0; i < b.height; i++ {
		newWorld[i] = make([]uint8, b.width)
	}

	for i := 0; i < numWorkers; i++ {
		startY := i * rowsPerWorker
		endY := startY + rowsPerWorker
		if i == numWorkers-1 {
			endY += remainder
		}
		workerWorld := make([][]uint8, endY-startY+2) // Include ghost rows
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
			// Copy the results back into newWorld
			for y := request.StartY; y < request.EndY; y++ {
				copy(newWorld[y], response.WorldSlice[y-request.StartY])
			}
		}(worker, request, i)
	}
	b.mu.Unlock()

	wg.Wait()
	b.mu.Lock()
	b.world = newWorld
	b.mu.Unlock()

	return nil
}

func (b *Broker) waitForProcessingToFinish() {
	// Wait for processing to finish
	b.mu.Lock()
	for b.processing {
		b.mu.Unlock()
		time.Sleep(100 * time.Millisecond)
		b.mu.Lock()
	}
	b.mu.Unlock()
}

// Implement other methods: GetWorld, Pause, Resume, Shutdown, GetAliveCells, StopProcessing

func (b *Broker) GetWorld(req *stubs.GetWorldRequest, res *stubs.GetWorldResponse) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	res.World = b.world
	res.CompletedTurns = b.turn
	res.Processing = b.processing
	return nil
}

func (b *Broker) Pause(req *stubs.PauseRequest, res *stubs.PauseResponse) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.processing || b.paused {
		return nil
	}
	b.paused = true
	res.Turn = b.turn
	return nil
}

func (b *Broker) Resume(req *stubs.ResumeRequest, res *stubs.ResumeResponse) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.processing || !b.paused {
		return nil
	}
	b.paused = false
	return nil
}

func (b *Broker) Shutdown(req *stubs.ShutdownRequest, res *stubs.ShutdownResponse) error {
	b.mu.Lock()
	b.shutdown = true
	b.stop = true
	b.processing = false
	b.paused = false
	b.mu.Unlock()
	return nil
}

func (b *Broker) GetAliveCells(req *stubs.AliveCellsCountRequest, res *stubs.AliveCellsCountResponse) error {
	b.mu.Lock()
	count := 0
	for y := 0; y < b.height; y++ {
		for x := 0; x < b.width; x++ {
			if b.world[y][x] == 255 {
				count++
			}
		}
	}
	res.CellsCount = count
	res.CompletedTurns = b.turn
	b.mu.Unlock()
	return nil
}

func (b *Broker) StopProcessing(req *stubs.StopRequest, res *stubs.StopResponse) error {
	b.mu.Lock()
	b.stop = true
	b.processing = false
	b.mu.Unlock()
	return nil
}

func main() {
	workerAddrs := []string{
		"54.221.83.157:8031",
		"18.232.81.24:8032",
		"54.86.97.98:8033",
		// Add more worker addresses as needed
	}

	broker := new(Broker)
	err := broker.connectToWorkers(workerAddrs)
	if err != nil {
		log.Fatal("Failed to connect to workers:", err)
	}

	rpc.Register(broker)
	listener, err := net.Listen("tcp", ":8030") // Broker listens on port 8030
	if err != nil {
		log.Fatal("Error starting broker:", err)
	}
	defer listener.Close()
	log.Println("Broker listening on port 8030")
	rpc.Accept(listener)
}
