package cli

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

// ANSI color codes matching the Outpost cyberpunk theme.
const (
	Reset     = "\033[0m"
	Bold      = "\033[1m"
	Dim       = "\033[2m"
	Italic    = "\033[3m"
	Underline = "\033[4m"

	// Outpost accent — #00ff88 green.
	Green   = "\033[38;2;0;255;136m"
	Red     = "\033[38;2;255;68;68m"
	Yellow  = "\033[38;2;255;170;0m"
	Blue    = "\033[38;2;100;149;237m"
	Cyan    = "\033[38;2;0;200;200m"
	Grey    = "\033[38;2;102;102;102m"
	White   = "\033[38;2;200;200;200m"
	BgPanel = "\033[48;2;16;16;24m"

	// Semantic.
	Accent  = Green
	Error   = Red
	Warning = Yellow
	Info    = Blue
	Muted   = Grey
)

// Logo returns the Outpost VPN text logo.
// The ">_ OUTPOST" prompt IS the brand identity (from assets/logo.svg).
func Logo() string {
	return Bold + White + ">_" + Reset + " " + Bold + Accent + "OUTPOST" + Reset + " " + Dim + Green + "VPN" + Reset
}

// _ suppresses unused constant warning.
var _ = BgPanel

// termWidth returns the current terminal width, defaulting to 80.
func termWidth() int {
	w, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || w <= 0 {
		return 80
	}
	return w
}

// Banner renders a clean welcome banner — single column, no overflow.
func Banner(appName, appVersion, gitCommit string, extra []string) string {
	tw := termWidth()
	if tw > 80 {
		tw = 80
	}
	w := tw - 2 // inner width (minus 2 border chars)

	var b strings.Builder
	line := func(content string) {
		b.WriteString(Accent + "|" + Reset)
		b.WriteString(padRight(content, w))
		b.WriteString(Accent + "|" + Reset + "\n")
	}
	border := func() {
		b.WriteString(Accent + "+" + strings.Repeat("-", w) + "+" + Reset + "\n")
	}

	border()
	line("")
	line(centerText(Logo(), w))
	line("")

	// Version.
	ver := fmt.Sprintf("%s%s v%s%s", Dim, appName, appVersion, Reset)
	if gitCommit != "unknown" && gitCommit != "" && len(gitCommit) > 7 {
		ver += fmt.Sprintf(" %s(%s)%s", Dim, gitCommit[:7], Reset)
	}
	line(centerText(ver, w))

	for _, e := range extra {
		line(centerText(e, w))
	}
	line("")
	border()

	return b.String()
}

// StatusLine renders a colored status line.
func StatusLine(ok bool, label, value string) string {
	icon := Green + "[ok]" + Reset
	if !ok {
		icon = Red + "[!!]" + Reset
	}
	return fmt.Sprintf("  %s %s%s%s  %s", icon, White, label, Reset, value)
}

// Section prints a styled section header.
func Section(title string) string {
	return fmt.Sprintf("\n%s%s> %s%s", Accent, Bold, title, Reset)
}

// KeyValue renders a key-value pair with aligned formatting.
func KeyValue(key, value string, width int) string {
	return fmt.Sprintf("  %s%-*s%s %s", Muted, width, key, Reset, value)
}

// PromptSymbol returns the styled input prompt.
func PromptSymbol() string {
	return fmt.Sprintf("%s>%s ", Accent, Reset)
}

// ErrorMsg formats an error message.
func ErrorMsg(msg string) string {
	return fmt.Sprintf("%s%s error:%s %s", Red, Bold, Reset, msg)
}

// SuccessMsg formats a success message.
func SuccessMsg(msg string) string {
	return fmt.Sprintf("%s%s[ok]%s %s", Green, Bold, Reset, msg)
}

// WarnMsg formats a warning message.
func WarnMsg(msg string) string {
	return fmt.Sprintf("%s%s[!!]%s %s", Yellow, Bold, Reset, msg)
}

// centerText centers text within the given visual width.
func centerText(text string, width int) string {
	visLen := visualLen(text)
	if visLen >= width {
		return text
	}
	pad := (width - visLen) / 2
	return strings.Repeat(" ", pad) + text
}

// padRight pads text to the given visual width.
func padRight(text string, width int) string {
	visLen := visualLen(text)
	if visLen >= width {
		return text
	}
	return text + strings.Repeat(" ", width-visLen)
}

// visualLen returns the length of a string excluding ANSI escape sequences.
func visualLen(s string) int {
	n := 0
	inEsc := false
	for _, r := range s {
		if r == '\033' {
			inEsc = true
			continue
		}
		if inEsc {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEsc = false
			}
			continue
		}
		n++
	}
	return n
}

// IsTerminal returns true if stdout is a terminal (not piped).
func IsTerminal() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}
