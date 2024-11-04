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

type WorkerInfo struct {
	client  *rpc.Client
	addr    string
	alive   bool
	retries int
}

type Broker struct {
	mu         sync.Mutex
	workers    []*WorkerInfo
	world      [][]uint8
	height     int
	width      int
	turn       int
	totalTurns int
	stop       bool
	processing bool
	paused     bool
	shutdown   bool
}

func (b *Broker) connectToWorkers(workerAddrs []string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.workers = make([]*WorkerInfo, len(workerAddrs))

	for i, addr := range workerAddrs {
		client, err := rpc.Dial("tcp", addr)
		if err != nil {
			log.Printf("Failed to connect to worker at %s: %v", addr, err)
			b.workers[i] = &WorkerInfo{
				client:  nil,
				addr:    addr,
				alive:   false,
				retries: 0,
			}
			continue
		}
		b.workers[i] = &WorkerInfo{
			client:  client,
			addr:    addr,
			alive:   true,
			retries: 0,
		}
	}

	return nil
}

func (b *Broker) monitorWorkers() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			b.mu.Lock()
			for _, worker := range b.workers {
				if worker.alive {
					go b.checkWorkerHeartbeat(worker)
				} else {
					// Try to reconnect
					client, err := rpc.Dial("tcp", worker.addr)
					if err == nil {
						worker.client = client
						worker.alive = true
						worker.retries = 0
						log.Printf("Reconnected to worker at %s", worker.addr)
					}
				}
			}
			b.mu.Unlock()
		}
	}
}

func (b *Broker) checkWorkerHeartbeat(worker *WorkerInfo) {
	b.mu.Lock()
	client := worker.client
	b.mu.Unlock()

	request := &stubs.HeartbeatRequest{}
	response := &stubs.HeartbeatResponse{}

	err := client.Call(stubs.Heartbeat, request, response)
	if err != nil {
		b.mu.Lock()
		worker.retries++
		if worker.retries >= 3 {
			log.Printf("Worker at %s failed after %d retries.", worker.addr, worker.retries)
			worker.alive = false
			if worker.client != nil {
				worker.client.Close()
			}
		}
		b.mu.Unlock()
	} else {
		b.mu.Lock()
		worker.retries = 0
		b.mu.Unlock()
	}
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
	// Filter out dead workers
	activeWorkers := []*WorkerInfo{}
	for _, worker := range b.workers {
		if worker.alive {
			activeWorkers = append(activeWorkers, worker)
		}
	}

	if len(activeWorkers) == 0 {
		b.mu.Unlock()
		return fmt.Errorf("no active workers available")
	}

	// Divide world into slices
	numWorkers := len(activeWorkers)
	rowsPerWorker := b.height / numWorkers
	remainder := b.height % numWorkers

	var wg sync.WaitGroup
	newWorld := make([][]uint8, b.height)
	for i := 0; i < b.height; i++ {
		newWorld[i] = make([]uint8, b.width)
	}

	startY := 0
	for i, workerInfo := range activeWorkers {
		extraRow := 0
		if i < remainder {
			extraRow = 1
		}
		endY := startY + rowsPerWorker + extraRow

		// Prepare world slice with ghost rows
		worldSlice := make([][]uint8, endY-startY+2)
		for y := startY - 1; y <= endY; y++ {
			row := make([]uint8, b.width)
			copy(row, b.world[(y+b.height)%b.height])
			worldSlice[y-startY+1] = row
		}

		request := stubs.WorkerRequest{
			StartY:      startY,
			EndY:        endY,
			WorldSlice:  worldSlice,
			ImageWidth:  b.width,
			ImageHeight: b.height,
		}

		worker := workerInfo.client
		wg.Add(1)
		go func(worker *rpc.Client, request stubs.WorkerRequest, workerInfo *WorkerInfo) {
			defer wg.Done()
			response := new(stubs.WorkerResponse)
			err := worker.Call(stubs.CalculateNextState, request, response)
			if err != nil {
				log.Printf("Error calling worker %s: %v", workerInfo.addr, err)
				// Mark worker as dead
				b.mu.Lock()
				workerInfo.alive = false
				if workerInfo.client != nil {
					workerInfo.client.Close()
				}
				b.mu.Unlock()
				// Redistribute work
				b.redistributeWork(request, newWorld)
				return
			}
			// Copy the results back into newWorld
			b.mu.Lock()
			for y := request.StartY; y < request.EndY; y++ {
				copy(newWorld[y], response.WorldSlice[y-request.StartY])
			}
			b.mu.Unlock()
		}(worker, request, workerInfo)

		startY = endY
	}
	b.mu.Unlock()

	wg.Wait()
	b.mu.Lock()
	b.world = newWorld
	b.mu.Unlock()

	return nil
}

func (b *Broker) redistributeWork(failedRequest stubs.WorkerRequest, newWorld [][]uint8) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Find active workers
	activeWorkers := []*WorkerInfo{}
	for _, worker := range b.workers {
		if worker.alive {
			activeWorkers = append(activeWorkers, worker)
		}
	}

	if len(activeWorkers) == 0 {
		log.Println("No active workers available to redistribute work")
		return
	}

	// Split the failed work among active workers
	numWorkers := len(activeWorkers)
	startY := failedRequest.StartY
	endY := failedRequest.EndY
	totalRows := endY - startY
	rowsPerWorker := totalRows / numWorkers
	remainder := totalRows % numWorkers

	var wg sync.WaitGroup
	for i, workerInfo := range activeWorkers {
		extraRow := 0
		if i < remainder {
			extraRow = 1
		}
		workerStartY := startY + i*(rowsPerWorker) + min(i, remainder)
		workerEndY := workerStartY + rowsPerWorker + extraRow

		// Prepare world slice with ghost rows
		worldSlice := make([][]uint8, workerEndY-workerStartY+2)
		for y := workerStartY - 1; y <= workerEndY; y++ {
			row := make([]uint8, b.width)
			copy(row, b.world[(y+b.height)%b.height])
			worldSlice[y-workerStartY+1] = row
		}

		request := stubs.WorkerRequest{
			StartY:      workerStartY,
			EndY:        workerEndY,
			WorldSlice:  worldSlice,
			ImageWidth:  b.width,
			ImageHeight: b.height,
		}

		worker := workerInfo.client
		wg.Add(1)
		go func(worker *rpc.Client, request stubs.WorkerRequest, workerInfo *WorkerInfo) {
			defer wg.Done()
			response := new(stubs.WorkerResponse)
			err := worker.Call(stubs.CalculateNextState, request, response)
			if err != nil {
				log.Printf("Error calling worker %s during redistribution: %v", workerInfo.addr, err)
				// Mark worker as dead
				b.mu.Lock()
				workerInfo.alive = false
				if workerInfo.client != nil {
					workerInfo.client.Close()
				}
				b.mu.Unlock()
				// Recursively redistribute work
				b.redistributeWork(request, newWorld)
				return
			}
			// Copy the results back into the newWorld
			b.mu.Lock()
			for y := request.StartY; y < request.EndY; y++ {
				copy(newWorld[y], response.WorldSlice[y-request.StartY])
			}
			b.mu.Unlock()
		}(worker, request, workerInfo)
	}

	wg.Wait()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
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
		"18.212.136.191:8031",
		"18.234.25.205:8032",
		"3.89.210.9:8033",
		// Add more worker addresses as needed
	}

	broker := new(Broker)
	err := broker.connectToWorkers(workerAddrs)
	if err != nil {
		log.Fatal("Failed to connect to workers:", err)
	}

	// Start monitoring workers
	go broker.monitorWorkers()

	rpc.Register(broker)
	listener, err := net.Listen("tcp", ":8030") // Broker listens on port 8030
	if err != nil {
		log.Fatal("Error starting broker:", err)
	}
	defer listener.Close()
	log.Println("Broker listening on port 8030")
	rpc.Accept(listener)
}
