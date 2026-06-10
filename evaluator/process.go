package evaluator

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"ravenshell/ast"
	"sort"
	"strconv"
	"strings"
	"syscall"
)

// job is a background process started with '&'.
type job struct {
	id   int
	pid  int
	name string
	done bool
}

// procInfo is a single entry from the system process list.
type procInfo struct {
	pid  int
	name string
}

// signalNames maps friendly signal names (with or without the SIG prefix) to
// their signal values.
var signalNames = map[string]syscall.Signal{
	"HUP":  syscall.SIGHUP,
	"INT":  syscall.SIGINT,
	"QUIT": syscall.SIGQUIT,
	"KILL": syscall.SIGKILL,
	"TERM": syscall.SIGTERM,
	"STOP": syscall.SIGSTOP,
	"CONT": syscall.SIGCONT,
	"USR1": syscall.SIGUSR1,
	"USR2": syscall.SIGUSR2,
}

// parseSignal resolves a signal name (TERM, SIGKILL, ...) or number (9) to a
// signal. Defaults are handled by the caller.
func parseSignal(s string) (syscall.Signal, error) {
	if n, err := strconv.Atoi(s); err == nil {
		return syscall.Signal(n), nil
	}
	name := strings.ToUpper(strings.TrimPrefix(strings.ToUpper(s), "SIG"))
	if sig, ok := signalNames[name]; ok {
		return sig, nil
	}
	return 0, fmt.Errorf("unknown signal %q", s)
}

// evalBackground runs a command in the background (the '&' suffix). Only
// external programs are backgrounded; anything else runs normally.
func (e *Evaluator) evalBackground(node *ast.BackgroundExpression) (Value, error) {
	cmd, ok := node.Command.(*ast.Command)
	if !ok || cmd.Type != ast.CMD_EXTERNAL {
		// Backgrounding only applies to external programs; run inline otherwise.
		return e.evalExpressionValue(node.Command)
	}

	args, err := e.evalArgs(cmd.Arguments)
	if err != nil {
		return nil, err
	}
	return e.startBackground(cmd.Name, args)
}

// startBackground launches an external program without waiting for it, records
// it as a job, and prints the job id and pid.
func (e *Evaluator) startBackground(name string, args []string) (string, error) {
	path, err := e.lookPath(name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: command not found\n", name)
		e.lastStatus = 127
		return "", nil
	}

	c := exec.Command(path, args...)
	c.Dir = e.cwd
	c.Env = e.buildEnv()
	c.Stdout = e.stdout
	c.Stderr = os.Stderr
	// Background jobs do not read from the terminal.
	c.Stdin = nil

	if err := c.Start(); err != nil {
		if errors.Is(err, syscall.ENOEXEC) {
			// Executable scripts with no shebang line: POSIX shells fall back
			// to interpreting the file with /bin/sh.
			c = exec.Command("/bin/sh", append([]string{path}, args...)...)
			c.Dir = e.cwd
			c.Env = e.buildEnv()
			c.Stdout = e.stdout
			c.Stderr = os.Stderr
			c.Stdin = nil
			err = c.Start()
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", name, err)
			e.lastStatus = 1
			return "", nil
		}
	}

	j := e.addJob(c.Process.Pid, name)
	fmt.Fprintf(e.stdout, "[%d] %d\n", j.id, j.pid)

	// Reap the process so it does not become a zombie, and mark it done.
	go func() {
		c.Wait()
		e.jobsMu.Lock()
		j.done = true
		e.jobsMu.Unlock()
	}()

	e.lastStatus = 0
	return "", nil
}

func (e *Evaluator) addJob(pid int, name string) *job {
	e.jobsMu.Lock()
	defer e.jobsMu.Unlock()
	j := &job{id: e.nextJobID, pid: pid, name: name}
	e.nextJobID++
	e.jobs = append(e.jobs, j)
	return j
}

// execJobs lists background jobs, dropping completed ones after reporting.
func (e *Evaluator) execJobs() (string, error) {
	e.jobsMu.Lock()
	defer e.jobsMu.Unlock()

	var out bytes.Buffer
	var alive []*job
	for _, j := range e.jobs {
		status := "running"
		if j.done {
			status = "done"
		}
		fmt.Fprintf(&out, "[%d]  %-8d %-8s %s\n", j.id, j.pid, status, j.name)
		if !j.done {
			alive = append(alive, j)
		}
	}
	e.jobs = alive

	result := out.String()
	if result == "" {
		result = "no background jobs\n"
	}
	fmt.Fprint(e.stdout, result)
	return result, nil
}

// jobByRef resolves a "%N" job reference to its pid.
func (e *Evaluator) jobByRef(ref string) (int, bool) {
	id, err := strconv.Atoi(strings.TrimPrefix(ref, "%"))
	if err != nil {
		return 0, false
	}
	e.jobsMu.Lock()
	defer e.jobsMu.Unlock()
	for _, j := range e.jobs {
		if j.id == id {
			return j.pid, true
		}
	}
	return 0, false
}

// splitSignalArgs pulls a signal out of the arguments. The signal may be a
// leading flag (-KILL) or the second positional argument (after the target);
// the remaining positionals are returned. Defaults to SIGTERM.
func splitSignalArgs(args []string) (syscall.Signal, []string, error) {
	sig := syscall.SIGTERM
	var positional []string
	for _, a := range args {
		if strings.HasPrefix(a, "-") && len(a) > 1 {
			s, err := parseSignal(strings.TrimPrefix(a, "-"))
			if err != nil {
				return sig, nil, err
			}
			sig = s
			continue
		}
		positional = append(positional, a)
	}
	// A second positional (kill PID SIGNAL) is also accepted as the signal.
	if len(positional) >= 2 {
		if s, err := parseSignal(positional[1]); err == nil {
			sig = s
			positional = positional[:1]
		}
	}
	return sig, positional, nil
}

// execKill sends a signal to a process by pid or "%job", defaulting to SIGTERM.
//
//	kill <pid|%job> [signal]    (signal may also be a -FLAG, e.g. kill -KILL 1234)
func (e *Evaluator) execKill(args []string) (string, error) {
	sig, positional, err := splitSignalArgs(args)
	if err != nil {
		return "", fmt.Errorf("kill: %v", err)
	}
	if len(positional) == 0 {
		return "", fmt.Errorf("kill: usage: kill <pid|%%job> [signal]")
	}

	var pid int
	if strings.HasPrefix(positional[0], "%") {
		p, ok := e.jobByRef(positional[0])
		if !ok {
			return "", fmt.Errorf("kill: no such job %s", positional[0])
		}
		pid = p
	} else {
		p, err := strconv.Atoi(positional[0])
		if err != nil {
			return "", fmt.Errorf("kill: invalid pid %q", positional[0])
		}
		pid = p
	}

	if err := signalPid(pid, sig); err != nil {
		return "", fmt.Errorf("kill: %v", err)
	}
	fmt.Fprintf(e.stdout, "signal %d sent to process %d\n", int(sig), pid)
	return "", nil
}

// execKillall signals all processes whose name contains the given text
// (case-insensitive), defaulting to SIGTERM.
//
//	killall <name> [signal]
func (e *Evaluator) execKillall(args []string) (string, error) {
	sig, positional, err := splitSignalArgs(args)
	if err != nil {
		return "", fmt.Errorf("killall: %v", err)
	}
	if len(positional) == 0 {
		return "", fmt.Errorf("killall: usage: killall <name> [signal]")
	}

	procs, err := listProcesses()
	if err != nil {
		return "", fmt.Errorf("killall: %v", err)
	}

	needle := strings.ToLower(positional[0])
	self := os.Getpid()
	killed := 0
	for _, p := range procs {
		if p.pid == self {
			continue
		}
		if strings.Contains(strings.ToLower(p.name), needle) {
			if err := signalPid(p.pid, sig); err == nil {
				fmt.Fprintf(e.stdout, "killed %d %s\n", p.pid, p.name)
				killed++
			}
		}
	}
	if killed == 0 {
		fmt.Fprintf(e.stdout, "no processes matching %q\n", args[0])
	}
	return "", nil
}

// execPs lists running processes, optionally filtered by a name substring.
//
//	ps [name-filter]
func (e *Evaluator) execPs(args []string) (string, error) {
	procs, err := listProcesses()
	if err != nil {
		return "", fmt.Errorf("ps: %v", err)
	}

	filter := ""
	if len(args) > 0 {
		filter = strings.ToLower(args[0])
	}

	var out bytes.Buffer
	fmt.Fprintf(&out, "%-8s %s\n", "PID", "NAME")
	for _, p := range procs {
		if filter != "" && !strings.Contains(strings.ToLower(p.name), filter) {
			continue
		}
		fmt.Fprintf(&out, "%-8d %s\n", p.pid, p.name)
	}

	result := out.String()
	fmt.Fprint(e.stdout, result)
	return result, nil
}

// signalPid sends sig to the process with the given pid.
func signalPid(pid int, sig syscall.Signal) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return proc.Signal(sig)
}

// listProcesses returns the system process list via `ps` (works on macOS and
// Linux), sorted by pid.
func listProcesses() ([]procInfo, error) {
	out, err := exec.Command("ps", "-axo", "pid=,comm=").Output()
	if err != nil {
		return nil, err
	}

	var procs []procInfo
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// "1234 /path/to/Some Command" -> pid, name (name may contain spaces).
		sp := strings.IndexAny(line, " \t")
		if sp < 0 {
			continue
		}
		pid, err := strconv.Atoi(strings.TrimSpace(line[:sp]))
		if err != nil {
			continue
		}
		name := strings.TrimSpace(line[sp+1:])
		procs = append(procs, procInfo{pid: pid, name: name})
	}

	sort.Slice(procs, func(i, j int) bool { return procs[i].pid < procs[j].pid })
	return procs, nil
}
