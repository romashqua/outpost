package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// Command represents a REPL command handler.
type Command struct {
	Name        string
	Description string
	Run         func(args []string) error
}

// REPL is an interactive command-line session.
type REPL struct {
	appName  string
	commands map[string]Command
	ordered  []Command // preserve order for help
	aliases  map[string]string
}

// NewREPL creates a new interactive REPL.
func NewREPL(appName string) *REPL {
	return &REPL{
		appName:  appName,
		commands: make(map[string]Command),
		aliases:  make(map[string]string),
	}
}

// Alias registers a single-key shortcut for a command.
func (r *REPL) Alias(short, full string) {
	r.aliases[short] = full
}

// Register adds a command to the REPL.
func (r *REPL) Register(cmd Command) {
	r.commands[cmd.Name] = cmd
	r.ordered = append(r.ordered, cmd)
}

const (
	// Dark background for input line — slightly lighter than #0a0a0f.
	bgInput = "\033[48;2;24;24;32m"
	// Full-width line separator.
	separatorChar = "─"
)

// inputLine renders the highlighted input prompt spanning the full terminal width.
// Background fills the entire line so the cursor sits on a dark "bar".
func inputLine() {
	tw := termWidth()
	// Separator line above input.
	fmt.Printf("%s%s%s\n", Muted, strings.Repeat(separatorChar, tw), Reset)
	// Highlighted input row: dark bg across full width, green prompt.
	// \033[K = erase to end of line (extends bg color).
	fmt.Printf("%s%s%s❯%s%s \033[K", bgInput, Bold, Accent, Reset, bgInput)
}

// statusBar renders the bottom hint bar spanning the full terminal width.
func statusBar() {
	tw := termWidth()
	left := fmt.Sprintf(" %s?%s for shortcuts", Accent, Reset)
	// Pad to full width so bg extends.
	visLeft := visualLen(left)
	pad := ""
	if tw > visLeft {
		pad = strings.Repeat(" ", tw-visLeft)
	}
	fmt.Printf("%s%s%s%s\n", Dim, left, pad, Reset)
}

// Run starts the interactive REPL loop.
func (r *REPL) Run() {
	// Built-in commands.
	r.commands["help"] = Command{
		Name:        "help",
		Description: "Show available commands",
		Run: func(_ []string) error {
			r.printHelp()
			return nil
		},
	}
	r.commands["quit"] = Command{
		Name:        "quit",
		Description: "Exit the session",
		Run:         nil, // handled specially
	}
	r.commands["exit"] = Command{
		Name:        "exit",
		Description: "Exit the session",
		Run:         nil,
	}
	r.commands["clear"] = Command{
		Name:        "clear",
		Description: "Clear the screen",
		Run: func(_ []string) error {
			fmt.Print("\033[2J\033[H")
			return nil
		},
	}

	// Welcome hint.
	fmt.Printf("\n  %sType a command or %s?%s%s for help, %sq%s%s to quit%s\n",
		Muted, Accent, Reset, Muted, Accent, Reset, Muted, Reset)
	fmt.Printf("  %sShell completion: %seval $(%s completion bash)%s\n",
		Muted, Dim, r.appName, Reset)

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Println()
		inputLine()

		if !scanner.Scan() {
			// Reset bg and print newline on EOF.
			fmt.Printf("%s\n", Reset)
			break
		}

		// Reset background after user presses Enter.
		fmt.Print(Reset)

		line := strings.TrimSpace(scanner.Text())

		// Status bar below input.
		statusBar()

		if line == "" {
			continue
		}

		parts := strings.Fields(line)
		cmdName := parts[0]
		args := parts[1:]

		// Resolve aliases.
		if resolved, ok := r.aliases[cmdName]; ok {
			cmdName = resolved
		}

		// Exit commands.
		if cmdName == "quit" || cmdName == "exit" {
			fmt.Println(SuccessMsg("Goodbye"))
			return
		}

		cmd, ok := r.commands[cmdName]
		if !ok {
			suggestion := r.suggest(cmdName)
			if suggestion != "" {
				fmt.Printf("  %sUnknown command:%s %s. Did you mean %s%s%s?\n",
					Muted, Reset, cmdName, Accent, suggestion, Reset)
			} else {
				fmt.Printf("  %sUnknown command:%s %s. Type %shelp%s or %s?%s for commands.\n",
					Muted, Reset, cmdName, Bold+White, Reset, Accent, Reset)
			}
			continue
		}

		if cmd.Run == nil {
			continue
		}

		fmt.Println()
		if err := cmd.Run(args); err != nil {
			fmt.Fprintf(os.Stderr, "  %s\n", ErrorMsg(err.Error()))
		}
	}
}

func (r *REPL) printHelp() {
	// Build reverse alias map: command → shortcuts.
	shortcuts := make(map[string][]string)
	for alias, cmd := range r.aliases {
		shortcuts[cmd] = append(shortcuts[cmd], alias)
	}

	fmt.Printf("\n%s%s> Commands%s\n\n", Accent, Bold, Reset)

	maxLen := 12
	for _, cmd := range r.ordered {
		l := len(cmd.Name)
		if keys, ok := shortcuts[cmd.Name]; ok {
			l += 2 + len(strings.Join(keys, ", "))
		}
		if l > maxLen {
			maxLen = l
		}
	}

	for _, cmd := range r.ordered {
		label := cmd.Name
		if keys, ok := shortcuts[cmd.Name]; ok {
			label += fmt.Sprintf(" %s(%s)%s", Dim, strings.Join(keys, ", "), Reset+Accent)
		}
		fmt.Printf("  %s%-*s%s  %s%s%s\n", Accent, maxLen+2, label, Reset, Muted, cmd.Description, Reset)
	}

	fmt.Printf("\n%s%s> Built-in%s\n\n", Accent, Bold, Reset)
	builtins := []struct{ name, desc string }{
		{"help, h, ?", "Show this help"},
		{"clear", "Clear the screen"},
		{"quit, q", "Exit the session"},
	}
	for _, b := range builtins {
		fmt.Printf("  %s%-*s%s  %s%s%s\n", Accent, maxLen+2, b.name, Reset, Muted, b.desc, Reset)
	}
}

// suggest finds the closest command name by prefix match.
func (r *REPL) suggest(input string) string {
	input = strings.ToLower(input)
	for name := range r.commands {
		if strings.HasPrefix(name, input) {
			return name
		}
	}
	for name := range r.commands {
		if strings.HasPrefix(input, name[:min(len(input), len(name))]) {
			return name
		}
	}
	return ""
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
