// Utils

package toroid

import (
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	tsize "github.com/kopoli/go-terminal-size"
)

const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorCyan   = "\033[36m"
	colorGray   = "\033[90m"
)

var (
	width    int
	logWidth int
)

// "YY/MM/DD HH:MM:SS [LEVL]  " = 26 chars
const logPrefix = 26

func init() {
	size, _ := tsize.GetSize()
	width = size.Width
	logWidth = width - logPrefix
}

// takes the string and adds new lines in places that would exceed the terminal width
func wrapInLogWidth(x string) string {
	indent := strings.Repeat(" ", logPrefix)
	var b strings.Builder
	lines := strings.Split(
		strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(x, "\r", ""), "\n", "\\n")),
		"\n",
	)
	for i, line := range lines {
		if i == len(lines)-1 && line == "" {
			break
		}
		if i > 0 {
			b.WriteString(indent)
		}
		if len(line) > logWidth {
			for j := 0; j < len(line); j += logWidth {
				chunk := line[j:min(j+logWidth, len(line))]
				if j > 0 {
					b.WriteString(indent)
				}
				b.WriteString(chunk)
				b.WriteByte('\n')
			}
		} else {
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func logLine(level, color, msg string) {
	ts := time.Now().Format("06/01/02 15:04:05")
	fmt.Fprintf(os.Stdout, "%s %s[%s]%s  %s", ts, color, level, colorReset, wrapInLogWidth(msg))
}

func LogInfo(msg string, args ...any) {
	logLine("INFO", colorCyan, fmt.Sprintf(msg, args...))
}

func LogError(msg string, args ...any) {
	logLine("ERRO", colorRed, fmt.Sprintf(msg, args...))
}

func LogDebug(msg string, args ...any) {
	logLine("DBUG", colorGray, fmt.Sprintf(msg, args...))
}

func PrettyPrintHistory(kernel *Kernel) {
	LogInfo("Printing History: %d (tokens: %d)", len(kernel.history), kernel.currentTokens)
	var b strings.Builder
	indent := strings.Repeat(" ", logPrefix)
	for _, msg := range kernel.history {
		message := indent + colorGray + string(msg.Role) + colorReset + ": " + strings.ReplaceAll(fmt.Sprintf("%v", msg.Content), "\n", "\\n")
		if len(message) > logWidth {
			message = message[:logWidth-3] + "..."
		}
		b.WriteString(fmt.Sprintf("%s\n", message))
	}
	fmt.Fprintf(os.Stdout, b.String())
}

func ApplyDefaults(cfg any) {
	v := reflect.ValueOf(cfg).Elem()
	t := v.Type()

	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		fieldType := t.Field(i)
		defaultTag := fieldType.Tag.Get("default")

		// skip if no default tag
		if defaultTag == "" {
			continue
		}

		// check if field is zero
		isZero := false
		switch field.Kind() {
		case reflect.String:
			isZero = field.String() == ""
		case reflect.Int, reflect.Int64:
			isZero = field.Int() == 0
		case reflect.Bool:
			// bool zero is false, but we can't easily distinguish "false" from "not set"
			// unless it's a pointer.
			isZero = !field.Bool()
		case reflect.Ptr:
			isZero = field.IsNil()
		}

		if isZero {
			switch field.Kind() {
			case reflect.String:
				field.SetString(defaultTag)
			case reflect.Int, reflect.Int64:
				val, _ := strconv.ParseInt(defaultTag, 10, 64)
				field.SetInt(val)
			case reflect.Bool:
				val, _ := strconv.ParseBool(defaultTag)
				field.SetBool(val)
			case reflect.Ptr:
				if field.Type().Elem().Kind() == reflect.Bool {
					val, _ := strconv.ParseBool(defaultTag)
					field.Set(reflect.ValueOf(&val))
				}
			}
		}
	}
}
