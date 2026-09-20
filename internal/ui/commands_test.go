package ui

import "testing"

// 每个命令名与别名都能查到，且都带 Run：这是"候选与分发同源"的守门测试。
func TestCommandTableIsResolvable(t *testing.T) {
	if len(commands) == 0 {
		t.Fatal("commands table is empty")
	}
	for _, spec := range commands {
		if spec.Name == "" || spec.Desc == "" || spec.Run == nil {
			t.Errorf("incomplete spec: %+v", spec)
		}
		names := append([]string{spec.Name}, spec.Aliases...)
		for _, name := range names {
			got, ok := commandLookup(name)
			if !ok {
				t.Fatalf("commandLookup(%q) not found", name)
			}
			if got.Name != spec.Name {
				t.Errorf("commandLookup(%q).Name = %q, want %q", name, got.Name, spec.Name)
			}
		}
	}
}

// 未知命令只写错误，绝不调用任何 Run（否则一个拼错的 ":w" 会写盘、":q" 会退出）。
func TestUnknownCommandDoesNotRun(t *testing.T) {
	m := setupModel()
	setCommand(m, "nope")
	executeCommand(m)
	if got := m.Error.Message; got != "Unknown command: nope" {
		t.Errorf("error = %q, want %q", got, "Unknown command: nope")
	}
}

// hint 是候选行右侧的说明：参数提示在前，一句话说明在后。
func TestCommandHint(t *testing.T) {
	if got := commandLookup0(t, "p").hint(); got != "Toggle play/pause" {
		t.Errorf("p hint = %q", got)
	}
	if got := commandLookup0(t, "open").hint(); got != "<path>  Load an audio file" {
		t.Errorf("open hint = %q", got)
	}
}

func commandLookup0(t *testing.T, name string) commandSpec {
	t.Helper()
	spec, ok := commandLookup(name)
	if !ok {
		t.Fatalf("commandLookup(%q) not found", name)
	}
	return spec
}

// parseInvocation 必须保留第一个 token 之后的原文（内部连续空格不能被折叠）。
func TestParseInvocationKeepsRawRest(t *testing.T) {
	inv, ok := parseInvocation("open /a/b  c.mp3")
	if !ok {
		t.Fatal("parseInvocation returned ok = false")
	}
	if inv.Name != "open" {
		t.Errorf("Name = %q", inv.Name)
	}
	if inv.Rest != "/a/b  c.mp3" {
		t.Errorf("Rest = %q, want %q", inv.Rest, "/a/b  c.mp3")
	}
	if len(inv.Parts) != 3 {
		t.Errorf("Parts = %v, want 3 fields", inv.Parts)
	}
	if _, ok := parseInvocation("   "); ok {
		t.Error("blank input should not parse")
	}
}

// lrcSubcommands must cover dispatch completely: every entry resolves through
// lrcSubcommandLookup and the full subcommand set is present, so a candidate can
// never describe a subcommand the dispatcher would reject.
func TestLrcSubcommandTable(t *testing.T) {
	if len(lrcSubcommands) != 7 {
		t.Fatalf("lrcSubcommands has %d entries, want 7", len(lrcSubcommands))
	}
	seen := map[string]bool{}
	for _, sub := range lrcSubcommands {
		if sub.Name == "" || sub.Desc == "" || sub.Run == nil {
			t.Errorf("incomplete sub spec: %+v", sub)
		}
		if seen[sub.Name] {
			t.Errorf("duplicate subcommand %q", sub.Name)
		}
		seen[sub.Name] = true
		if _, ok := lrcSubcommandLookup(sub.Name); !ok {
			t.Errorf("lrcSubcommandLookup(%q) not found", sub.Name)
		}
	}
	for _, want := range []string{"on", "off", "switch", "refresh", "agent", "desktop", "panel"} {
		if !seen[want] {
			t.Errorf("subcommand %q missing from the table", want)
		}
	}
}

// parseInvocation.Rest must share strings.Fields' notion of whitespace
// (unicode.IsSpace, not just " \t"), otherwise a leading NBSP or newline
// misaligns the slice and Rest keeps the whitespace.
func TestParseInvocationRestUsesFieldsWhitespace(t *testing.T) {
	inv, ok := parseInvocation("open\u00a0/a  b.mp3")
	if !ok {
		t.Fatal("parseInvocation returned ok = false")
	}
	if inv.Rest != "/a  b.mp3" {
		t.Errorf("Rest = %q, want %q", inv.Rest, "/a  b.mp3")
	}
	if inv, _ := parseInvocation("open  /a.mp3"); inv.Rest != "/a.mp3" {
		t.Errorf("Rest = %q, want %q", inv.Rest, "/a.mp3")
	}
	if inv, _ := parseInvocation("open"); inv.Rest != "" {
		t.Errorf("Rest = %q, want empty", inv.Rest)
	}
}
