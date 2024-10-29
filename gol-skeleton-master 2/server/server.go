package main

import (
	"flag"
	"fmt"
	"net"
	"net/rpc"
	"os"
	"sync"

	"uk.ac.bris.cs/gameoflife/stubs"
)

// 1 : Pass the report from the distributor. (Ticker request (broker -> dis))
// 2 : Connect with the distributor (Register (broker -> dis))
// 3 : Pass the keypress arguments from the distributor to the worker. (Keypress (dis -> broker) (broker -> worker))

// Channels that are used to communicate with broker and worker
var worldChan []chan World
var workers []Worker
var nextId = 0
var topicmx sync.RWMutex
var unit int

// var theWorld World

type World struct {
	world [][]uint8
	turns int
}

type WorkerParams struct {
	StartY int
	EndY   int
	StartX int
	EndX   int
}

type Worker struct {
	id           int
	stateSwitch  int
	worker       *rpc.Client
	address      *string
	params       WorkerParams
	worldChannel chan World
}

// Store the world
var p stubs.Params
var world [][]uint8
var completedTurns int

// Connect the worker in a loop
func subscribe_loop(w Worker, startGame chan bool) {
	fmt.Println("Loooping")
	response := new(stubs.Response)
	workerReq := stubs.WorkerRequest{WorkerId: w.id, StartY: w.params.StartY, EndY: w.params.EndY, StartX: w.params.StartX, EndX: w.params.EndX, World: world, Turns: p.Turns, Params: p}
	<-startGame
	go func() {
		for {
			wt := <-w.worldChannel
			updateResponse := new(stubs.StatusReport)
			updateRequest := stubs.UpdateRequest{World: wt.world, Turns: wt.turns}
			err := w.worker.Call(stubs.UpdateWorker, updateRequest, updateResponse)
			if err != nil {
				fmt.Println("Error calling UpdateWorker")
				//fmt.Println(err)
				fmt.Println("Closing subscriber thread.")
				//Place the unfulfilled job back on the topic channel.
				w.worldChannel <- wt
				break
			}
			fmt.Println("Updated worker:", w.id, "turns:", completedTurns)
		}
	}()
	err := w.worker.Call(stubs.ProcessTurnsHandler, workerReq, response)
	if err != nil {
		fmt.Println("Error calling ProcessTurnsHandler")
		fmt.Println("Closing subscriber thread.")
	}
}

// Initialise connecting worker, and if no error occurs, invoke register_loop.
func subscribe(workerAddress string) (err error) {
	fmt.Println("Subscription request")
	client, err := rpc.Dial("tcp", workerAddress)
	var newWorker Worker
	if nextId != p.Threads-1 {
		newWorker = Worker{
			id:           nextId,
			stateSwitch:  -1,
			worker:       client,
			address:      &workerAddress,
			worldChannel: worldChan[nextId],
			params: WorkerParams{
				StartX: 0,
				StartY: nextId * unit,
				EndX:   p.ImageWidth,
				EndY:   (nextId + 1) * (unit),
			},
		}
	} else {
		newWorker = Worker{
			id:           nextId,
			stateSwitch:  -1,
			worker:       client,
			address:      &workerAddress,
			worldChannel: worldChan[nextId],
			params: WorkerParams{
				StartX: 0,
				StartY: nextId * unit,
				EndX:   p.ImageWidth,
				EndY:   p.ImageHeight,
			},
		}
	}
	workers = append(workers, newWorker)
	nextId++
	startGame := make(chan bool)
	go func() {
		for {
			if p.Threads == len(workers) {
				startGame <- true
			}
		}
	}()
	if err == nil {
		fmt.Println("Looooop")
		go subscribe_loop(newWorker, startGame)
	} else {
		fmt.Println("Error subscribing ", workerAddress)
		fmt.Println(err)
		return err
	}

	return
}

// Make an connection with the Distributor
// And initialise the params.
func registerDistributor(req stubs.Request, res *stubs.StatusReport) (err error) {
	topicmx.RLock()
	defer topicmx.RUnlock()
	world = req.World
	p.Turns = req.Turns
	p.Threads = req.Threads
	p.ImageHeight = req.ImageHeight
	p.ImageWidth = req.ImageWidth
	unit = int(p.ImageHeight / p.Threads)
	completedTurns = 0
	return err
}

func makeChannel(threads int) {
	topicmx.Lock()
	defer topicmx.Unlock()
	worldChan = make([]chan World, threads)

	for i := range worldChan {
		worldChan[i] = make(chan World)

		fmt.Println("Created channel #", i)
	}
}

var incr int = 0

func merge(ubworldSlice [][]uint8, w Worker) {
	for i := range ubworldSlice {
		//fmt.Println("merge slice on:", w.params.StartY+i)
		copy(world[w.params.StartY+i], ubworldSlice[i])
	}
	incr++

	// return
}

func matchWorker(id int) Worker {
	for _, w := range workers {
		if w.id == id {
			return w
		}
	}
	panic("No such worker")
}

var worldChanWhat []chan [][]uint8

func updateBroker(ubturns int, ubworldSlice [][]uint8, workerId int) error {
	topicmx.Lock()
	defer topicmx.Unlock()
	fmt.Println("Call merge func for worker:", workerId)
	merge(ubworldSlice, matchWorker(workerId))

	if incr == p.Threads {
		for _, w := range workers {
			fmt.Println("Sending update to worker #", w.id)
			w.worldChannel <- World{
				world: world,
				turns: ubturns,
			}
			incr--
		}
		completedTurns = ubturns

	}
	return nil
}

func closeBroker() {
	for _, w := range workers {
		w.worker.Close()
	}
	defer os.Exit(0)
	return
}

type Broker struct{}

// func (b *Broker) ReportStatus(req stubs.StateRequest, req *stubs.Response) (err error) {
// 	return err
// }

func (b *Broker) UpdateBroker(req stubs.UpdateRequest, res *stubs.StatusReport) (err error) {
	err = updateBroker(req.Turns, req.World, req.WorkerId)
	return err
}

func (b *Broker) MakeChannel(req stubs.ChannelRequest, res *stubs.StatusReport) (err error) {
	makeChannel(req.Threads)
	return
}

// Calls and connects to the worker (Subscribe)
func (b *Broker) ConnectWorker(req stubs.SubscribeRequest, res *stubs.StatusReport) (err error) {
	err = subscribe(req.WorkerAddress)
	if err != nil {
		fmt.Println(err)
	}
	return
}

func (b *Broker) ConnectDistributor(req stubs.Request, res *stubs.Response) (err error) {
	err = registerDistributor(req, new(stubs.StatusReport))
	// Checks if the connection and the worker is still on
	if len(workers) == p.Threads {
		for _, w := range workers {
			startGame := make(chan bool)
			fmt.Println("Unit = ", unit)
			fmt.Println("wid = ", w.id)
			fmt.Println("widXunit = ", w.id*unit)
			if w.id != p.Threads-1 {
				w.params = WorkerParams{
					StartX: 0,
					StartY: w.id * unit,
					EndX:   p.ImageWidth,
					EndY:   (w.id + 1) * (unit),
				}
			} else {
				w.params = WorkerParams{
					StartX: 0,
					StartY: w.id * unit,
					EndX:   p.ImageWidth,
					EndY:   p.ImageHeight,
				}
			}

			go subscribe_loop(w, startGame)
			go func() {
				startGame <- true
			}()
		}
	} else if len(workers) < p.Threads {
		for _, w := range workers {
			w.params = WorkerParams{
				StartX: 0,
				StartY: w.id * unit,
				EndX:   p.ImageWidth,
				EndY:   (w.id + 1) * (unit),
			}
			startGame := make(chan bool)
			go subscribe_loop(w, startGame)
			go func() {
				startGame <- true
			}()
		}
	}
	for {
		if p.Turns == completedTurns {
			res.World = world
			res.TurnsDone = completedTurns
			return
		}
	}
}

func (b *Broker) Publish(req stubs.TickerRequest, res *stubs.Response) (err error) {
	// loop and condition added to make sure that world variable is updated before publishing
	for {
		if incr == 0 {
			res.World = world
			res.TurnsDone = completedTurns
			break
		}
	}

	//err = publish(stubs.StateRequest{State: req.State})
	return err
}

func (b *Broker) Action(req stubs.StateRequest, res *stubs.StatusReport) (err error) {
	for _, w := range workers {
		w.worker.Call(stubs.ActionHandlerWorker, stubs.StateRequest{State: req.State}, res)
	}
	return nil
}

func (b *Broker) ActionWithReport(req stubs.StateRequest, res *stubs.Response) (err error) {
	for _, w := range workers {
		w.worker.Call(stubs.ActionReportWorker, req, new(stubs.StatusReport))
	}

	res.TurnsDone = completedTurns
	res.World = world
	if req.State == stubs.Kill {
		go closeBroker()
	}

	return nil
}

func main() {
	// Listens to the distributor
	//pAddr := flag.String("port", "8030", "Port to listen on")
	flag.Parse()
	rpc.Register(&Broker{})
	listener, _ := net.Listen("tcp", ":"+"8030")
	defer listener.Close()
	rpc.Accept(listener)
}
