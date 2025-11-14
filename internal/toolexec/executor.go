package toolexec

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Request captures a shell execution request exposed to the agent.
type Request struct {
	Command    string
	Args       []string
	Workdir    string
	Background bool
}

// Result captures the outcome of a foreground execution.
type Result struct {
	Output   string
	ExitCode int
}

// JobStatus describes the lifecycle milestone of a background job.
type JobStatus string

const (
	JobRunning JobStatus = "running"
	JobSuccess JobStatus = "success"
	JobFailed  JobStatus = "failed"
)

// Job carries metadata about a background execution.
type Job struct {
	ID        string
	Command   string
	Args      []string
	StartedAt time.Time
	EndedAt   time.Time
	Status    JobStatus
	ExitCode  int
	Output    string
	Error     string
}

// Event is emitted whenever a job changes status.
type Event struct {
	Job Job
}

type foregroundCmd struct {
	cancel context.CancelFunc
}

// Executor coordinates foreground/ background shell execution.
type Executor struct {
	mu         sync.Mutex
	foreground *foregroundCmd
	jobs       map[string]*Job
	cancels    map[string]context.CancelFunc
	events     chan Event
}

// NewExecutor creates an Executor instance.
func NewExecutor() *Executor {
	return &Executor{
		jobs:    make(map[string]*Job),
		cancels: make(map[string]context.CancelFunc),
		events:  make(chan Event, 32),
	}
}

// Events exposes a stream of job updates for UI consumers.
func (e *Executor) Events() <-chan Event {
	return e.events
}

// Run executes the request in either foreground or background mode.
func (e *Executor) Run(ctx context.Context, req Request) (Result, string, error) {
	if req.Command == "" {
		return Result{}, "", errors.New("command is required")
	}
	if req.Background {
		id, err := e.startBackground(req)
		return Result{}, id, err
	}
	res, err := e.runForeground(ctx, req)
	return res, "", err
}

// CancelForeground aborts the active foreground process if any.
func (e *Executor) CancelForeground() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.foreground == nil {
		return false
	}
	e.foreground.cancel()
	e.foreground = nil
	return true
}

// CancelJob requests cancellation of a background job, returning true if a job was canceled.
func (e *Executor) CancelJob(id string) bool {
	e.mu.Lock()
	cancel, ok := e.cancels[id]
	if ok {
		delete(e.cancels, id)
	}
	e.mu.Unlock()
	if ok {
		cancel()
	}
	return ok
}

func (e *Executor) runForeground(ctx context.Context, req Request) (Result, error) {
	ctx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(ctx, req.Command, req.Args...)
	if req.Workdir != "" {
		cmd.Dir = filepath.Clean(req.Workdir)
	}
	var buffer bytes.Buffer
	cmd.Stdout = &buffer
	cmd.Stderr = &buffer

	e.mu.Lock()
	e.foreground = &foregroundCmd{cancel: cancel}
	e.mu.Unlock()

	err := cmd.Run()

	e.mu.Lock()
	e.foreground = nil
	e.mu.Unlock()

	exitCode := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		} else {
			exitCode = -1
		}
	}

	return Result{Output: buffer.String(), ExitCode: exitCode}, err
}

func (e *Executor) startBackground(req Request) (string, error) {
	id := uuid.NewString()
	job := &Job{
		ID:        id,
		Command:   req.Command,
		Args:      req.Args,
		StartedAt: time.Now(),
		Status:    JobRunning,
	}

	bgCtx, cancel := context.WithCancel(context.Background())

	e.mu.Lock()
	e.jobs[id] = job
	e.cancels[id] = cancel
	e.mu.Unlock()

	e.emit(job)

	go func() {
		cmd := exec.CommandContext(bgCtx, req.Command, req.Args...)
		if req.Workdir != "" {
			cmd.Dir = filepath.Clean(req.Workdir)
		}
		var buffer bytes.Buffer
		cmd.Stdout = &buffer
		cmd.Stderr = &buffer
		err := cmd.Run()
		e.mu.Lock()
		job.Output = buffer.String()
		job.EndedAt = time.Now()
		if err != nil {
			job.Status = JobFailed
			job.Error = err.Error()
			job.ExitCode = -1
			if ee, ok := err.(*exec.ExitError); ok {
				job.ExitCode = ee.ExitCode()
			}
		} else {
			job.Status = JobSuccess
			job.ExitCode = 0
		}
		delete(e.cancels, id)
		e.mu.Unlock()
		e.emit(job)
	}()

	return id, nil
}

func (e *Executor) emit(job *Job) {
	select {
	case e.events <- Event{Job: *job}:
	default:
	}
}

// ActiveJobs returns a snapshot of current jobs.
func (e *Executor) ActiveJobs() []Job {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]Job, 0, len(e.jobs))
	for _, job := range e.jobs {
		out = append(out, *job)
	}
	return out
}
