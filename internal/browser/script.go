package browser

import (
	"fmt"
	"strings"
)

// ScriptStep represents a single action in a C4A-Script program.
type ScriptStep struct {
	Action   string `json:"action"`
	Selector string `json:"selector,omitempty"`
	Value    string `json:"value,omitempty"`
}

// Script is a parsed C4A-Script program composed of sequential steps.
type Script struct {
	Steps []ScriptStep `json:"steps"`
}

// ParseScript parses C4A-Script DSL source text into a Script.
//
// The DSL is line-based. Supported commands:
//
//	navigate <url>
//	wait <ms>
//	click <selector>
//	type <selector> <text>
//	scroll <direction> <amount>
//	screenshot <filename>
//	extract <selector>
//	assert <selector> <condition>
func ParseScript(source string) (*Script, error) {
	lines := strings.Split(source, "\n")
	var steps []ScriptStep

	for lineNo, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) == 0 {
			continue
		}

		action := strings.ToLower(parts[0])
		step := ScriptStep{Action: action}

		switch action {
		case "navigate":
			if len(parts) < 2 {
				return nil, fmt.Errorf("line %d: navigate requires a URL", lineNo+1)
			}
			step.Value = parts[1]

		case "wait":
			if len(parts) < 2 {
				return nil, fmt.Errorf("line %d: wait requires milliseconds", lineNo+1)
			}
			step.Value = parts[1]

		case "click":
			if len(parts) < 2 {
				return nil, fmt.Errorf("line %d: click requires a selector", lineNo+1)
			}
			step.Selector = strings.Join(parts[1:], " ")

		case "type":
			if len(parts) < 3 {
				return nil, fmt.Errorf("line %d: type requires a selector and text", lineNo+1)
			}
			step.Selector = parts[1]
			step.Value = strings.Join(parts[2:], " ")

		case "scroll":
			if len(parts) < 3 {
				return nil, fmt.Errorf("line %d: scroll requires direction and amount", lineNo+1)
			}
			step.Selector = parts[1] // direction
			step.Value = parts[2]    // amount

		case "screenshot":
			if len(parts) < 2 {
				return nil, fmt.Errorf("line %d: screenshot requires a filename", lineNo+1)
			}
			step.Value = parts[1]

		case "extract":
			if len(parts) < 2 {
				return nil, fmt.Errorf("line %d: extract requires a selector", lineNo+1)
			}
			step.Selector = strings.Join(parts[1:], " ")

		case "assert":
			if len(parts) < 3 {
				return nil, fmt.Errorf("line %d: assert requires a selector and condition", lineNo+1)
			}
			step.Selector = parts[1]
			step.Value = strings.Join(parts[2:], " ")

		default:
			return nil, fmt.Errorf("line %d: unknown action %q", lineNo+1, action)
		}

		steps = append(steps, step)
	}

	return &Script{Steps: steps}, nil
}

// CompileToJS compiles a Script into CDP-compatible JavaScript.
// Each step is translated to a self-contained JS statement.
func CompileToJS(script *Script) string {
	var sb strings.Builder
	sb.WriteString("(async () => {\n")

	for _, step := range script.Steps {
		switch step.Action {
		case "navigate":
			fmt.Fprintf(&sb, "  window.location.href = %s;\n", jsString(step.Value))
			sb.WriteString("  await new Promise(r => setTimeout(r, 1000));\n")

		case "wait":
			fmt.Fprintf(&sb, "  await new Promise(r => setTimeout(r, %s));\n", step.Value)

		case "click":
			fmt.Fprintf(&sb, "  document.querySelector(%s).click();\n", jsString(step.Selector))

		case "type":
			fmt.Fprintf(&sb, "  {\n")
			fmt.Fprintf(&sb, "    const el = document.querySelector(%s);\n", jsString(step.Selector))
			sb.WriteString("    el.focus();\n")
			fmt.Fprintf(&sb, "    el.value = %s;\n", jsString(step.Value))
			sb.WriteString("    el.dispatchEvent(new Event('input', {bubbles: true}));\n")
			sb.WriteString("    el.dispatchEvent(new Event('change', {bubbles: true}));\n")
			fmt.Fprintf(&sb, "  }\n")

		case "scroll":
			direction := step.Selector
			amount := step.Value
			switch direction {
			case "down":
				fmt.Fprintf(&sb, "  window.scrollBy(0, %s);\n", amount)
			case "up":
				fmt.Fprintf(&sb, "  window.scrollBy(0, -%s);\n", amount)
			case "left":
				fmt.Fprintf(&sb, "  window.scrollBy(-%s, 0);\n", amount)
			case "right":
				fmt.Fprintf(&sb, "  window.scrollBy(%s, 0);\n", amount)
			default:
				fmt.Fprintf(&sb, "  window.scrollBy(0, %s);\n", amount)
			}

		case "screenshot":
			sb.WriteString("  // screenshot is handled by the CDP caller\n")
			fmt.Fprintf(&sb, "  window.__c4a_screenshot = %s;\n", jsString(step.Value))

		case "extract":
			fmt.Fprintf(&sb, "  {\n")
			fmt.Fprintf(&sb, "    const el = document.querySelector(%s);\n", jsString(step.Selector))
			sb.WriteString("    window.__c4a_extracted = el ? el.textContent : '';\n")
			fmt.Fprintf(&sb, "  }\n")

		case "assert":
			fmt.Fprintf(&sb, "  {\n")
			fmt.Fprintf(&sb, "    const el = document.querySelector(%s);\n", jsString(step.Selector))
			fmt.Fprintf(&sb, "    if (!el) throw new Error('assert failed: element not found ' + %s);\n", jsString(step.Selector))
			switch step.Value {
			case "visible":
				sb.WriteString("    if (el.offsetParent === null) throw new Error('assert failed: element not visible');\n")
			case "hidden":
				sb.WriteString("    if (el.offsetParent !== null) throw new Error('assert failed: element not hidden');\n")
			case "exists":
				// Already checked above.
			default:
				fmt.Fprintf(&sb, "    // custom condition: %s\n", step.Value)
			}
			fmt.Fprintf(&sb, "  }\n")
		}
	}

	sb.WriteString("})();\n")
	return sb.String()
}

// jsString returns a JavaScript string literal with proper escaping.
func jsString(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "'", "\\'")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	return "'" + s + "'"
}
