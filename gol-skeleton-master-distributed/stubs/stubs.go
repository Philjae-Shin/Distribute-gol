package stubs

import "uk.ac.bris.cs/gameoflife/util"

const (
	Process            = "Broker.Process"
	GetAliveCells      = "Broker.GetAliveCells"
	StopProcessing     = "Broker.StopProcessing"
	GetWorld           = "Broker.GetWorld"
	Pause              = "Broker.Pause"
	Resume             = "Broker.Resume"
	Shutdown           = "Broker.Shutdown"
	CalculateNextState = "GolWorker.CalculateNextState"
	Heartbeat          = "GolWorker.Heartbeat" // Added for heartbeat mechanism

	// Live SDL
	ReportCellFlipped  = "Distributor.ReportCellFlipped"
	ReportCellsFlipped = "Distributor.ReportCellsFlipped"
	ReportTurnComplete = "Distributor.ReportTurnComplete"
	ReportStateChange  = "Distributor.ReportStateChange"
)

type EngineRequest struct {
	World          [][]uint8
	ImageWidth     int
	ImageHeight    int
	Turns          int
	ControllerAddr string
}

type EngineResponse struct {
	World          [][]uint8
	CompletedTurns int
}

type AliveCellsCountRequest struct{}

type AliveCellsCountResponse struct {
	CompletedTurns int
	CellsCount     int
}

type StopRequest struct{}

type StopResponse struct{}

type GetWorldRequest struct{}

type GetWorldResponse struct {
	World          [][]uint8
	CompletedTurns int
	Processing     bool
}

type PauseRequest struct{}

type PauseResponse struct {
	Turn int
}

type ResumeRequest struct{}

type ResumeResponse struct{}

type ShutdownRequest struct{}

type ShutdownResponse struct{}

type WorkerRequest struct {
	StartY      int
	EndY        int
	WorldSlice  [][]uint8
	ImageWidth  int
	ImageHeight int
}

type WorkerResponse struct {
	WorldSlice   [][]uint8
	FlippedCells []util.Cell
}

type HeartbeatRequest struct{} // Added for heartbeat mechanism

type HeartbeatResponse struct{}

// Types for reporting events back to the controller
type ReportCellFlippedRequest struct {
	CompletedTurns int
	Cell           util.Cell
}

type ReportCellFlippedResponse struct{}

type ReportCellsFlippedRequest struct {
	CompletedTurns int
	Cells          []util.Cell
}

type ReportCellsFlippedResponse struct{}

type ReportTurnCompleteRequest struct {
	CompletedTurns int
}

type ReportTurnCompleteResponse struct{}

type ReportStateChangeRequest struct {
	CompletedTurns int
	NewState       State
}

type ReportStateChangeResponse struct{}
