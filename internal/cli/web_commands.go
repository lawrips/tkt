package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/lawrips/tkt/internal/web"
)

type webState struct {
	PID       int    `json:"pid"`
	Addr      string `json:"addr"`
	URL       string `json:"url"`
	Token     string `json:"token"`
	StartedAt string `json:"started_at"`
}

func runWeb(ctx context, args []string) error {
	return runWebRun(ctx, args)
}

func runWebRun(ctx context, args []string) error {
	fs := flag.NewFlagSet("web run", flag.ContinueOnError)
	fs.SetOutput(ctx.stderr)
	addr := "127.0.0.1:0"
	statePath := ""
	fs.StringVar(&addr, "addr", addr, "")
	fs.StringVar(&statePath, "state", "", "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("usage: tkt web run [--addr=127.0.0.1:0]")
	}

	server, err := web.New(web.Options{
		CWD:             currentWorkingDir(),
		ProjectOverride: ctx.projectOverride,
		Version:         versionString,
	})
	if err != nil {
		return err
	}
	listener, err := web.Listen(addr)
	if err != nil {
		return err
	}
	defer listener.Close()

	url := server.URLFor(listener)
	if statePath != "" {
		state := webState{
			PID:       os.Getpid(),
			Addr:      listener.Addr().String(),
			URL:       url,
			Token:     server.Token(),
			StartedAt: time.Now().UTC().Format(time.RFC3339),
		}
		if err := writeWebState(statePath, state); err != nil {
			return err
		}
	} else {
		_, _ = fmt.Fprintf(ctx.stdout, "tkt web running at %s\n", url)
	}

	return http.Serve(listener, server)
}

func runWebStart(ctx context, args []string) error {
	fs := flag.NewFlagSet("web start", flag.ContinueOnError)
	fs.SetOutput(ctx.stderr)
	addr := "127.0.0.1:0"
	fs.StringVar(&addr, "addr", addr, "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("usage: tkt web start [--addr=127.0.0.1:0]")
	}

	pidPath, err := webPIDPath()
	if err != nil {
		return err
	}
	logPath, err := webLogPath()
	if err != nil {
		return err
	}
	statePath, err := webStatePath()
	if err != nil {
		return err
	}

	if pid, running := serveRunningPID(pidPath); running {
		state, _ := readWebState(statePath)
		if ctx.json {
			return emitJSON(ctx, map[string]any{
				"status": "already_running",
				"pid":    pid,
				"url":    state.URL,
			})
		}
		_, _ = fmt.Fprintf(ctx.stdout, "web already running (pid %d)\n", pid)
		if state.URL != "" {
			_, _ = fmt.Fprintf(ctx.stdout, "url: %s\n", state.URL)
		}
		return nil
	}
	_ = os.Remove(pidPath)
	_ = os.Remove(statePath)

	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot find executable: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0755); err != nil {
		return err
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}

	childArgs := []string{self, "web", "run", "--addr=" + addr, "--state", statePath}
	procAttr := &os.ProcAttr{
		Files: []*os.File{
			nil,
			logFile,
			logFile,
		},
	}
	proc, err := os.StartProcess(self, childArgs, procAttr)
	logFile.Close()
	if err != nil {
		return fmt.Errorf("start web process: %w", err)
	}
	pid := proc.Pid
	_ = proc.Release()
	if err := writePIDFile(pidPath, pid); err != nil {
		return err
	}

	state := waitForWebState(statePath, pid, 2*time.Second)
	if ctx.json {
		return emitJSON(ctx, map[string]any{
			"status":     "started",
			"pid":        pid,
			"pid_file":   pidPath,
			"log_file":   logPath,
			"state_file": statePath,
			"url":        state.URL,
		})
	}

	_, _ = fmt.Fprintf(ctx.stdout, "web started (pid %d, log: %s)\n", pid, logPath)
	if state.URL != "" {
		_, _ = fmt.Fprintf(ctx.stdout, "url: %s\n", state.URL)
	} else {
		_, _ = fmt.Fprintln(ctx.stdout, "url: pending; run `tkt web status`")
	}
	return nil
}

func runWebStop(ctx context, args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("usage: tkt web stop")
	}
	pidPath, err := webPIDPath()
	if err != nil {
		return err
	}
	statePath, err := webStatePath()
	if err != nil {
		return err
	}
	pid, running := serveRunningPID(pidPath)
	if !running {
		_ = os.Remove(pidPath)
		_ = os.Remove(statePath)
		if ctx.json {
			return emitJSON(ctx, map[string]any{"status": "not_running"})
		}
		_, _ = fmt.Fprintln(ctx.stdout, "web is not running")
		return nil
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find process %d: %w", pid, err)
	}
	if err := proc.Signal(os.Interrupt); err != nil {
		return fmt.Errorf("signal process %d: %w", pid, err)
	}
	_ = os.Remove(pidPath)
	_ = os.Remove(statePath)

	if ctx.json {
		return emitJSON(ctx, map[string]any{"status": "stopped", "pid": pid})
	}
	_, _ = fmt.Fprintf(ctx.stdout, "web stopped (pid %d)\n", pid)
	return nil
}

func runWebStatus(ctx context, args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("usage: tkt web status")
	}
	pidPath, err := webPIDPath()
	if err != nil {
		return err
	}
	logPath, err := webLogPath()
	if err != nil {
		return err
	}
	statePath, err := webStatePath()
	if err != nil {
		return err
	}

	pid, running := serveRunningPID(pidPath)
	state, _ := readWebState(statePath)
	if !running {
		_ = os.Remove(pidPath)
		_ = os.Remove(statePath)
	}

	if ctx.json {
		return emitJSON(ctx, map[string]any{
			"running":    running,
			"pid":        pid,
			"pid_file":   pidPath,
			"log_file":   logPath,
			"state_file": statePath,
			"url":        state.URL,
		})
	}

	if running {
		_, _ = fmt.Fprintf(ctx.stdout, "web running (pid %d)\n", pid)
		if state.URL != "" {
			_, _ = fmt.Fprintf(ctx.stdout, "url: %s\n", state.URL)
		}
	} else {
		_, _ = fmt.Fprintln(ctx.stdout, "web is not running")
	}
	_, _ = fmt.Fprintf(ctx.stdout, "log: %s\n", logPath)
	return nil
}

func runWebLogs(ctx context, args []string) error {
	fs := flag.NewFlagSet("web logs", flag.ContinueOnError)
	fs.SetOutput(ctx.stderr)
	lines := 50
	fs.IntVar(&lines, "n", 50, "")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("usage: tkt web logs [-n=50]")
	}

	logPath, err := webLogPath()
	if err != nil {
		return err
	}
	raw, err := os.ReadFile(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			if ctx.json {
				return emitJSON(ctx, map[string]any{
					"log_file": logPath,
					"lines":    []string{},
				})
			}
			_, _ = fmt.Fprintln(ctx.stdout, "(no log file)")
			return nil
		}
		return err
	}
	allLines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	start := 0
	if len(allLines) > lines {
		start = len(allLines) - lines
	}
	tail := allLines[start:]
	if ctx.json {
		return emitJSON(ctx, map[string]any{
			"log_file": logPath,
			"lines":    tail,
		})
	}
	for _, line := range tail {
		_, _ = fmt.Fprintln(ctx.stdout, line)
	}
	return nil
}

func webPIDPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".tkt", "state", "web.pid"), nil
}

func webLogPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".tkt", "state", "web.log"), nil
}

func webStatePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".tkt", "state", "web.json"), nil
}

func writeWebState(path string, state webState) error {
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0600)
}

func readWebState(path string) (webState, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return webState{}, err
	}
	var state webState
	if err := json.Unmarshal(raw, &state); err != nil {
		return webState{}, err
	}
	return state, nil
}

func waitForWebState(path string, pid int, timeout time.Duration) webState {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		state, err := readWebState(path)
		if err == nil && state.PID == pid && state.URL != "" {
			return state
		}
		time.Sleep(50 * time.Millisecond)
	}
	return webState{}
}

func writePIDFile(path string, pid int) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strconv.Itoa(pid)+"\n"), 0644)
}

func currentWorkingDir() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return cwd
}
