package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/tgdrive/teldrive/cmd"
)

type flagRow struct {
	Name        string
	Shorthand   string
	Default     string
	Description string
	Required    bool
	Group       string
}

func main() {
	root := cmd.New()
	outDir := filepath.Join("docs", "docs", "cli")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		panic(err)
	}

	commands := root.Commands()
	sort.Slice(commands, func(i, j int) bool { return commands[i].Name() < commands[j].Name() })

	if err := os.WriteFile(filepath.Join(outDir, "index.md"), []byte(buildCLIIndex(commands)), 0o644); err != nil {
		panic(err)
	}

	for _, command := range commands {
		if command.Hidden {
			continue
		}
		path := filepath.Join(outDir, command.Name()+".md")
		if err := os.WriteFile(path, []byte(buildCommandDoc(command)), 0o644); err != nil {
			panic(err)
		}
	}
}

func buildCLIIndex(commands []*cobra.Command) string {
	var b strings.Builder
	b.WriteString("# CLI\n\n")
	b.WriteString("These pages are generated from the Cobra command tree and config-backed flags in this repository.\n\n")
	b.WriteString("## Commands\n\n")
	for _, command := range commands {
		if command.Hidden {
			continue
		}
		b.WriteString(fmt.Sprintf("- [`%s`](/docs/cli/%s) — %s\n", command.Name(), command.Name(), strings.TrimSpace(command.Short)))
	}
	b.WriteString("\n> Re-run `task gen:docs` after changing commands, defaults, or flag descriptions.\n")
	return b.String()
}

func buildCommandDoc(command *cobra.Command) string {
	flags := collectFlags(command)
	var b strings.Builder
	b.WriteString(fmt.Sprintf("# `%s`\n\n", command.CommandPath()))
	if long := strings.TrimSpace(command.Long); long != "" {
		b.WriteString(long + "\n\n")
	} else if short := strings.TrimSpace(command.Short); short != "" {
		b.WriteString(short + "\n\n")
	}
	b.WriteString("## Usage\n\n```sh\n")
	b.WriteString(strings.TrimSpace(command.UseLine()))
	b.WriteString("\n```\n\n")

	if len(flags) == 0 {
		b.WriteString("This command has no flags.\n")
		return b.String()
	}

	groups := make(map[string][]flagRow)
	groupOrder := make([]string, 0)
	for _, flag := range flags {
		if _, ok := groups[flag.Group]; !ok {
			groupOrder = append(groupOrder, flag.Group)
		}
		groups[flag.Group] = append(groups[flag.Group], flag)
	}
	sort.Strings(groupOrder)
	if idx := indexOf(groupOrder, "General"); idx > 0 {
		groupOrder[0], groupOrder[idx] = groupOrder[idx], groupOrder[0]
	}

	b.WriteString("## Flags\n\n")
	for _, group := range groupOrder {
		b.WriteString(fmt.Sprintf("### %s\n\n", group))
		b.WriteString("| Flag | Default | Description |\n")
		b.WriteString("| --- | --- | --- |\n")
		for _, flag := range groups[group] {
			defaultValue := flag.Default
			if defaultValue == "" {
				defaultValue = "—"
			}
			description := escapePipes(flag.Description)
			if flag.Required {
				description += " **Required.**"
			}
			b.WriteString(fmt.Sprintf("| `%s` | `%s` | %s |\n", renderFlag(flag), escapeBackticks(defaultValue), description))
		}
		b.WriteString("\n")
	}

	b.WriteString("> Duration flags accept values like `30s`, `5m`, `1h`, or `7d`. Flags can also be set through the config file or environment-variable mapping where applicable.\n")
	return b.String()
}

func collectFlags(command *cobra.Command) []flagRow {
	rows := make([]flagRow, 0)
	command.NonInheritedFlags().VisitAll(func(flag *pflag.Flag) {
		if flag.Hidden || flag.Deprecated != "" {
			return
		}
		rows = append(rows, flagRow{
			Name:        flag.Name,
			Shorthand:   flag.Shorthand,
			Default:     flag.DefValue,
			Description: flag.Usage,
			Required:    flag.Annotations != nil && len(flag.Annotations[cobra.BashCompOneRequiredFlag]) > 0,
			Group:       flagGroup(flag.Name),
		})
	})
	sort.Slice(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })
	return rows
}

func renderFlag(flag flagRow) string {
	if flag.Shorthand == "" {
		return "--" + flag.Name
	}
	return fmt.Sprintf("-%s, --%s", flag.Shorthand, flag.Name)
}

func flagGroup(name string) string {
	if name == "config" {
		return "General"
	}
	parts := strings.Split(name, "-")
	if len(parts) == 0 {
		return "General"
	}
	return strings.ToUpper(parts[0][:1]) + parts[0][1:]
}

func escapePipes(value string) string {
	value = strings.ReplaceAll(value, "|", "\\|")
	return escapeBackticks(value)
}

func escapeBackticks(value string) string {
	return strings.ReplaceAll(value, "`", "\\`")
}

func indexOf(items []string, needle string) int {
	for i, item := range items {
		if item == needle {
			return i
		}
	}
	return -1
}
