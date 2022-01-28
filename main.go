package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/shu-go/gli"
	"golang.org/x/xerrors"
)

// Version is app version
var Version string

func init() {
	if Version == "" {
		Version = "dev-" + time.Now().Format("20060102")
	}
}

type globalCmd struct {
	Export exportCmd `help:"export to stdout" usage:"default secion is 'alias' only\nuse --all if you need"`
	Import importCmd `help:"import from stdin(terminal: enter 2 empty lines to finish)"`

	List listCmd `cli:"list,ls" help:"list items"`

	Git string `default:"git" help:"git command"`
}

type exportCmd struct {
	Sections gli.StrList `cli:"section,s=LIST" default:"alias"`
	All      bool        `help:"export all sections. --section is ignored"`

	System   bool
	Global   bool
	Local    bool
	Worktree bool
}

func (c exportCmd) Run(g globalCmd) error {
	cmd := exec.Command(g.Git, "config", "--list")
	appendLocation(cmd, c.System, c.Global, c.Local, c.Worktree)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return xerrors.Errorf("stdoutpipe: %v", err)
	}

	err = cmd.Start()
	if err != nil {
		return xerrors.Errorf("run: %v", err)
	}

	lines := make([]string, 0, 10)

	buf := bufio.NewReader(stdout)
	for {
		linebyte, _, err := buf.ReadLine()
		if err != nil {
			break
		}

		line := string(linebyte)

		if !c.All {
			ok := false
			for _, s := range c.Sections {
				if strings.HasPrefix(line, s+".") {
					ok = true
				}
			}
			if !ok {
				continue
			}
		}

		//fmt.Println(line)
		lines = append(lines, line)
	}

	sort.Slice(lines, func(i, j int) bool {
		return lines[i] < lines[j]
	})
	for _, l := range lines {
		fmt.Println(l)
	}

	return nil
}

type importCmd struct {
	Alias bool `help:"complete 'alias.' if no section provided"`

	System   bool
	Global   bool
	Local    bool
	Worktree bool
}

func (c importCmd) Run(g globalCmd) error {
	prevEmpty := false

	buf := bufio.NewReader(os.Stdin)
	for {
		linebyte, _, err := buf.ReadLine()
		if err != nil {
			break
		}

		line := strings.TrimSpace(string(linebyte))

		if line == "" {
			if prevEmpty {
				break
			}
			prevEmpty = true
			continue
		}
		prevEmpty = false

		if strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}

		delimpos := strings.Index(line, "=")
		if delimpos == -1 {
			delimpos = strings.Index(line, " ")
			if delimpos == -1 {
				fmt.Fprint(os.Stderr, "ERROR: no delimiter between key and value\n")
				continue
			}
			fmt.Fprint(os.Stderr, "WARNING: no =, you put Space instead?\n")
		}
		key := line[:delimpos]
		value := line[delimpos+1:]

		dotpos := strings.Index(key, ".")
		if dotpos <= 0 {
			if c.Alias {
				key = "alias." + key
			} else {
				fmt.Fprint(os.Stderr, "ERROR: no section\n")
				continue
			}
		}
		if dotpos == len(key)-1 {
			fmt.Fprint(os.Stderr, "ERROR: no variable\n")
			continue
		}

		if value == "" {
			// unset
			cmd := exec.Command(g.Git, "config")
			appendLocation(cmd, c.System, c.Global, c.Local, c.Worktree)
			cmd.Args = append(cmd.Args, "--unset")
			cmd.Args = append(cmd.Args, key)
			cmd.Stderr = os.Stderr
			err = cmd.Run()
			if err != nil {
				fmt.Fprintf(os.Stderr, "ERROR: git: %v\n", err)
				continue
			}
		} else {
			// unset and add
			cmd := exec.Command(g.Git, "config")
			appendLocation(cmd, c.System, c.Global, c.Local, c.Worktree)
			cmd.Args = append(cmd.Args, "--unset")
			cmd.Args = append(cmd.Args, key)
			cmd.Stderr = os.Stderr
			_ /*IGNORE*/ = cmd.Run()

			cmd = exec.Command(g.Git, "config")
			appendLocation(cmd, c.System, c.Global, c.Local, c.Worktree)
			cmd.Args = append(cmd.Args, "--add")
			cmd.Args = append(cmd.Args, key)
			cmd.Args = append(cmd.Args, value)
			cmd.Stderr = os.Stderr
			err = cmd.Run()
			if err != nil {
				fmt.Fprintf(os.Stderr, "ERROR: git: %v\n", err)
				continue
			}
		}
	}

	return nil
}

type listCmd struct {
	Diff bool `help:"output only those items that have different values for each scope"`

	Sections gli.StrList `cli:"section,s=LIST --all is ignored"`
	All      bool        `help:"export all sections" default:"true"`
}

type scopedValue struct {
	scope, value string
}

func (c listCmd) Run(g globalCmd, args []string) error {
	scopes := []string{
		"worktree",
		"local",
		"global",
		"system",
	}

	values := make(map[string]([]scopedValue))

	for _, scope := range scopes {
		cmd := exec.Command(g.Git, "config", "--list")
		cmd.Args = append(cmd.Args, "--"+scope)

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			return xerrors.Errorf("stdoutpipe: %v", err)
		}

		err = cmd.Start()
		if err != nil {
			return xerrors.Errorf("run: %v", err)
		}

		buf := bufio.NewReader(stdout)
		for {
			linebyte, _, err := buf.ReadLine()
			if err != nil {
				break
			}

			line := string(linebyte)

			ok := false
			if len(c.Sections) > 0 {
				for _, s := range c.Sections {
					if strings.HasPrefix(line, s+".") {
						ok = true
					}
				}
			} else if c.All {
				ok = true
			}
			if !ok {
				continue
			}

			kv := strings.Split(line, "=")
			if len(kv) < 2 {
				continue
			}
			if _, found := values[kv[0]]; found {
				values[kv[0]] = append(values[kv[0]], scopedValue{
					scope: scope,
					value: kv[1],
				})
			} else {
				values[kv[0]] = []scopedValue{
					{
						scope: scope,
						value: kv[1],
					},
				}
			}
		}
	}

	var keys []string
	for k, _ := range values {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return keys[i] < keys[j]
	})

	diffColor := color.New(color.FgRed)
	for _, k := range keys {
		diff := differs(values[k])
		if c.Diff && !diff {
			continue
		}

		fmt.Println(k)

		first := values[k][0].value
		for _, sv := range values[k] {
			if sv.value == first {
				fmt.Printf("\t%s\t%s\n", sv.value, sv.scope)
			} else {
				diffColor.Printf("\t%s\t%s\n", sv.value, sv.scope)
			}
		}
	}

	return nil
}

func differs(values []scopedValue) bool {
	diff := false

	if len(values) == 0 {
		return diff
	}

	first := values[0].value
	for _, sv := range values {
		if sv.value != first {
			diff = true
			break
		}
	}

	return diff
}

func appendLocation(cmd *exec.Cmd, system, global, local, worktree bool) {
	if system {
		cmd.Args = append(cmd.Args, "--system")
	}
	if global {
		cmd.Args = append(cmd.Args, "--global")
	}
	if local {
		cmd.Args = append(cmd.Args, "--local")
	}
	if worktree {
		cmd.Args = append(cmd.Args, "--worktree")
	}
}

func main() {
	app := gli.NewWith(&globalCmd{})
	app.Name = "git-konfig"
	app.Desc = "export/import gitconfig"
	app.Version = Version
	app.Usage = `git konfig export
git konfig import < myconfig.txt
location options (--sytem, --global, --local and --worktree) are same as git config's`
	app.Copyright = "(C) 2021 Shuhei Kubota"
	app.Run(os.Args)
}
