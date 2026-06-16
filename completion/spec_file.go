package completion

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// User spec files let users add completions for commands the shell doesn't
// know, fish's ~/.config/fish/completions equivalent. A file is looked up the
// first time its command is completed and the result (including "no file") is
// cached for the session.
//
// Format (~/.config/ravenshell/completions/<cmd>.json):
//
//	{
//	  "flags": [{"text": "--verbose", "desc": "Verbose output"}],
//	  "subcommands": [
//	    {
//	      "name": "serve", "desc": "Start the server",
//	      "flags": [{"text": "--port", "desc": "Port to listen on"}],
//	      "args": {"static": [{"text": "dev"}], "command": "mytool list-envs", "noFiles": true}
//	    }
//	  ],
//	  "args": {"command": "...", "noFiles": false, "dirsOnly": false}
//	}

type fileItem struct {
	Text string `json:"text"`
	Desc string `json:"desc"`
}

type fileArgs struct {
	Static   []fileItem `json:"static"`
	Command  string     `json:"command"`
	NoFiles  bool       `json:"noFiles"`
	DirsOnly bool       `json:"dirsOnly"`
}

type fileSub struct {
	Name  string     `json:"name"`
	Desc  string     `json:"desc"`
	Flags []fileItem `json:"flags"`
	Args  *fileArgs  `json:"args"`
}

type fileSpec struct {
	Flags       []fileItem `json:"flags"`
	Subcommands []fileSub  `json:"subcommands"`
	Args        *fileArgs  `json:"args"`
}

// userSpec returns the spec loaded from the user's completion directory for
// cmd, or nil if there is none (or it is malformed). Lookups are cached.
func (e *Engine) userSpec(cmd string) *Spec {
	e.mu.Lock()
	defer e.mu.Unlock()

	if spec, ok := e.userSpecs[cmd]; ok {
		return spec
	}

	var spec *Spec
	if e.specDir != "" {
		spec = loadSpecFile(filepath.Join(e.specDir, cmd+".json"))
	}
	e.userSpecs[cmd] = spec
	return spec
}

// loadSpecFile reads and converts one user spec file, returning nil if the
// file is absent or invalid.
func loadSpecFile(path string) *Spec {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var fs fileSpec
	if json.Unmarshal(data, &fs) != nil {
		return nil
	}

	spec := &Spec{
		Flags: itemsToCandidates(fs.Flags),
		Args:  argsToSpec(fs.Args),
	}
	for _, sub := range fs.Subcommands {
		if sub.Name == "" {
			continue
		}
		spec.Subcommands = append(spec.Subcommands, SubSpec{
			Name:  sub.Name,
			Desc:  sub.Desc,
			Flags: itemsToCandidates(sub.Flags),
			Args:  argsToSpec(sub.Args),
		})
	}
	return spec
}

func itemsToCandidates(items []fileItem) []Candidate {
	if len(items) == 0 {
		return nil
	}
	out := make([]Candidate, 0, len(items))
	for _, it := range items {
		if it.Text != "" {
			out = append(out, Candidate{Text: it.Text, Desc: it.Desc})
		}
	}
	return out
}

func argsToSpec(a *fileArgs) *ArgSpec {
	if a == nil {
		return nil
	}
	return &ArgSpec{
		Static:   itemsToCandidates(a.Static),
		Command:  a.Command,
		NoFiles:  a.NoFiles,
		DirsOnly: a.DirsOnly,
	}
}
