// PTY service for the in-app terminal. Mirrors the tauri-plugin-pty command
// surface used by the `tauri-pty` npm package (spawn/resize/write/kill plus
// data + exit streams), backed by github.com/aymanbagabas/go-pty (ConPTY on
// Windows, /dev/ptmx elsewhere). Output and exit are streamed to the
// frontend via Wails events (`pty:data` / `pty:exit`), so the JS shim only
// needs one-shot start/write/resize/kill calls.
package app

import (
	"encoding/base64"
	"fmt"
	"os/exec"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"droidgear-wails/internal/core"

	ptylib "github.com/aymanbagabas/go-pty"
	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	// ptyFlushInterval bounds output latency while coalescing reads; the
	// design doc for the terminal called for ~16ms aggregation (§3.9).
	ptyFlushInterval = 16 * time.Millisecond
	// ptyFlushThreshold forces an immediate flush for large bursts so the
	// flush interval can never grow into visible stutter.
	ptyFlushThreshold = 32 * 1024
)

type ptySession struct {
	pty      ptylib.Pty
	cmd      *ptylib.Cmd
	exitOnce sync.Once
	exitCode atomic.Int32
	done     chan struct{}
	// readDone is closed by readLoop once its final output batch has been
	// emitted, so waitLoop never races `pty:exit` ahead of trailing data.
	readDone chan struct{}
}

type ptyManager struct {
	mu       sync.Mutex
	sessions map[int]*ptySession
	nextID   atomic.Int64
}

var ptys = &ptyManager{sessions: map[int]*ptySession{}}

// emitPtyEvent broadcasts a PTY event when a Wails app instance exists;
// outside one (unit tests, early startup) it falls back to the test sink
// when set, otherwise it is a no-op.
func emitPtyEvent(name string, payload map[string]any) {
	if app := application.Get(); app != nil {
		app.Event.Emit(name, payload)
		return
	}
	if ptyEventSink != nil {
		ptyEventSink(name, payload)
	}
}

// ptyEventSink lets tests observe the event stream without a running app.
var ptyEventSink func(name string, payload map[string]any)

// PtyStartOptions mirrors the tauri-pty spawn options (nullable fields match
// the JS call site, which passes null for "unset").
type PtyStartOptions struct {
	File string            `json:"file"`
	Args []string          `json:"args"`
	Name *string           `json:"name"`
	Cols *int              `json:"cols"`
	Rows *int              `json:"rows"`
	Cwd  *string           `json:"cwd"`
	Env  map[string]string `json:"env"`
}

func (m *ptyManager) get(id int) *ptySession {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sessions[id]
}

// Start spawns a process attached to a new PTY and returns its session id.
func (m *ptyManager) Start(opts PtyStartOptions) (int, error) {
	cols := 80
	rows := 24
	if opts.Cols != nil && *opts.Cols > 0 {
		cols = *opts.Cols
	}
	if opts.Rows != nil && *opts.Rows > 0 {
		rows = *opts.Rows
	}

	handle, err := ptylib.New()
	if err != nil {
		return 0, fmt.Errorf("Failed to allocate PTY: %w", err)
	}
	if err := handle.Resize(cols, rows); err != nil {
		_ = handle.Close()
		return 0, fmt.Errorf("Failed to size PTY: %w", err)
	}

	cmd := handle.Command(opts.File, opts.Args...)
	if opts.Cwd != nil && *opts.Cwd != "" {
		cmd.Dir = *opts.Cwd
	}
	if len(opts.Env) > 0 {
		env := make([]string, 0, len(opts.Env))
		for key, value := range opts.Env {
			env = append(env, key+"="+value)
		}
		cmd.Env = env
	}
	// Note: no CREATE_NO_WINDOW/HideWindow here — the ConPTY pseudo-console
	// already keeps the child off any visible console, and the flag would
	// make CreateProcess attach a hidden real console instead, detaching the
	// child from the PTY input pipe.
	if err := cmd.Start(); err != nil {
		_ = handle.Close()
		return 0, fmt.Errorf("Failed to start PTY process: %w", err)
	}

	id := int(m.nextID.Add(1)) // #nosec G115 -- monotonically increasing within a session
	session := &ptySession{
		pty:      handle,
		cmd:      cmd,
		done:     make(chan struct{}),
		readDone: make(chan struct{}),
	}
	m.mu.Lock()
	m.sessions[id] = session
	m.mu.Unlock()

	go m.readLoop(id, session)
	go m.waitLoop(id, session)
	return id, nil
}

// readLoop pumps PTY output to the frontend as base64-encoded chunks; the JS
// shim reassembles them with a streaming TextDecoder, so multibyte sequences
// split across reads need no server-side handling.
//
// Reads are coalesced before emitting: bursts (npm install, cargo build)
// batch for up to ptyFlushInterval or ptyFlushThreshold bytes, so heavy
// output cannot turn into an IPC event storm against the webview
// (guidelines §3.9). Latency for interactive output stays ≤ 16ms.
func (m *ptyManager) readLoop(id int, session *ptySession) {
	defer func() {
		close(session.readDone)
		m.mu.Lock()
		delete(m.sessions, id)
		m.mu.Unlock()
	}()

	// Reader goroutine: turns the blocking PTY Read into a channel so the
	// main loop can select between output and the flush timer.
	chunks := make(chan []byte)
	go func() {
		defer close(chunks)
		buf := make([]byte, 8192)
		for {
			n, err := session.pty.Read(buf)
			if n > 0 {
				chunk := make([]byte, n)
				copy(chunk, buf[:n])
				chunks <- chunk
			}
			if err != nil {
				return
			}
		}
	}()

	flushTimer := time.NewTimer(time.Hour)
	if !flushTimer.Stop() {
		<-flushTimer.C
	}
	timerArmed := false
	pending := make([]byte, 0, ptyFlushThreshold)
	flush := func() {
		if len(pending) == 0 {
			return
		}
		emitPtyEvent("pty:data", map[string]any{
			"id":   id,
			"data": base64.StdEncoding.EncodeToString(pending),
		})
		pending = pending[:0]
	}

	for {
		var timerC <-chan time.Time
		if timerArmed {
			timerC = flushTimer.C
		}
		select {
		case chunk, ok := <-chunks:
			if !ok {
				flush() // trailing batch must land before pty:exit
				return
			}
			pending = append(pending, chunk...)
			if len(pending) >= ptyFlushThreshold {
				if timerArmed {
					flushTimer.Stop()
					timerArmed = false
				}
				flush()
			} else if !timerArmed {
				flushTimer.Reset(ptyFlushInterval)
				timerArmed = true
			}
		case <-timerC:
			timerArmed = false
			flush()
		}
	}
}

func (m *ptyManager) waitLoop(id int, session *ptySession) {
	exitCode := int32(0)
	if err := session.cmd.Wait(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = int32(exitErr.ExitCode()) // #nosec G115 -- process exit codes fit int32
		} else {
			// Non-exit errors (spawn failures surfacing at Wait) mirror the
			// plugin's convention of reporting an unknown (-1) status.
			exitCode = -1
		}
	}
	// Drain trailing output first: `pty:exit` must never overtake the last
	// `pty:data` batch (the shim stops decoding after exit). Bounded so a
	// stuck PTY cannot block shutdown forever.
	select {
	case <-session.readDone:
	case <-time.After(2 * time.Second):
	}
	session.exitOnce.Do(func() {
		session.exitCode.Store(exitCode)
		close(session.done)
		emitPtyEvent("pty:exit", map[string]any{
			"id":       id,
			"exitCode": exitCode,
		})
	})
}

// Write sends user input to the PTY.
func (m *ptyManager) Write(id int, data string) error {
	session := m.get(id)
	if session == nil {
		return errString("EOF")
	}
	if _, err := session.pty.Write([]byte(data)); err != nil {
		return fmt.Errorf("Failed to write to PTY: %w", err)
	}
	return nil
}

// Resize updates the PTY dimensions.
func (m *ptyManager) Resize(id int, cols int, rows int) error {
	session := m.get(id)
	if session == nil {
		return errString("EOF")
	}
	if cols <= 0 || rows <= 0 {
		return errString("Invalid PTY dimensions")
	}
	return session.pty.Resize(cols, rows)
}

// Kill terminates the session's process tree.
func (m *ptyManager) Kill(id int) error {
	session := m.get(id)
	if session == nil {
		return nil
	}
	// Closing the PTY master hangs up the child; also kill the process tree
	// so shell children (node, etc.) do not linger on Windows.
	_ = session.pty.Close()
	if runtime.GOOS == "windows" && session.cmd.Process != nil {
		taskkill := exec.Command("taskkill", "/PID", fmt.Sprint(session.cmd.Process.Pid), "/T", "/F") // #nosec G204 -- pid from our own process
		core.HideConsoleWindow(taskkill)
		if err := taskkill.Run(); err != nil && session.cmd.Process != nil {
			_ = session.cmd.Process.Kill()
		}
		return nil
	}
	if session.cmd.Process != nil {
		_ = session.cmd.Process.Kill()
	}
	return nil
}

// PtyStart spawns a shell/PTY session (tauri-pty `spawn` parity).
func (a *App) PtyStart(opts PtyStartOptions) (int, error) {
	if opts.File == "" {
		return 0, errString("Missing executable")
	}
	return ptys.Start(opts)
}

// PtyWrite sends input to a PTY session.
func (a *App) PtyWrite(id int, data string) error {
	return ptys.Write(id, data)
}

// PtyResize resizes a PTY session.
func (a *App) PtyResize(id int, cols int, rows int) error {
	return ptys.Resize(id, cols, rows)
}

// PtyKill terminates a PTY session.
func (a *App) PtyKill(id int) error {
	return ptys.Kill(id)
}

// killAll terminates every live session; used by the shutdown cleanup so
// no child shell outlives the GUI process.
func (m *ptyManager) killAll() {
	m.mu.Lock()
	ids := make([]int, 0, len(m.sessions))
	for id := range m.sessions {
		ids = append(ids, id)
	}
	m.mu.Unlock()
	for _, id := range ids {
		_ = m.Kill(id)
	}
}
