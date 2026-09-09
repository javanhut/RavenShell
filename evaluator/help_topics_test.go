package evaluator

import (
	"strings"
	"testing"
)

// Every language topic resolves by name and by each alias, renders its body,
// and does not collide with a built-in command name.
func TestHelpTopicsResolve(t *testing.T) {
	commands := BuiltinSummaries()
	for _, topic := range helpTopics {
		names := append([]string{topic.name}, topic.aliases...)
		for _, name := range names {
			if _, clash := commands[name]; clash {
				t.Errorf("topic name %q collides with a built-in command", name)
			}
			_, out := run(t, "raven-help "+name)
			if !strings.HasPrefix(stripANSI(out), topic.name+" — ") {
				t.Errorf("raven-help %s: output does not start with the topic title:\n%s", name, out)
			}
			if !strings.Contains(out, topic.summary) {
				t.Errorf("raven-help %s: missing summary", name)
			}
		}
		if strings.TrimSpace(topic.body) == "" {
			t.Errorf("topic %s has an empty body", topic.name)
		}
	}
}

// The overview lists the topics after the commands, and an unknown name
// still fails with a message that mentions topics.
func TestHelpOverviewListsTopics(t *testing.T) {
	_, out := run(t, "raven-help")
	if !strings.Contains(out, "Language topics") {
		t.Fatalf("overview lacks the Language topics section:\n%s", out)
	}
	for _, name := range topicNameList() {
		if !strings.Contains(out, name) {
			t.Errorf("overview does not list topic %q", name)
		}
	}
	e := New()
	_, err := e.execRavenHelp([]string{"nope"})
	if err == nil || !strings.Contains(err.Error(), "language topic") {
		t.Errorf("unknown name: error = %v, want one mentioning language topics", err)
	}
}

// Completion after raven-help offers topics as well as commands.
func TestHelpSummariesIncludeTopics(t *testing.T) {
	m := HelpSummaries()
	for _, want := range []string{"syntax", "strings", "indexing", "ls", "read"} {
		if _, ok := m[want]; !ok {
			t.Errorf("HelpSummaries lacks %q", want)
		}
	}
}

// The examples the topics make claims about behave as the topics say.
func TestHelpTopicExamplesHold(t *testing.T) {
	cases := []struct{ src, want string }{
		{"count = 5\nprint count + 10\nprint $count + 10\n", "5 + 10\n15\n"},
		{"count = 5\nprint (count + 1) * 2\n", "12\n"},
		{"half = (10 + 5) / 2\nprint $half\n", "7\n"},
		{"items = []\nappend(items, \"a\")\nitems = append(items, \"b\")\nprint items\n", "a b\n"},
		{"grid = [[1, 2], [3, 4]]\nprint grid[1][0]\n", "3\n"},
		{"for i in range(1, 4) { print i }\n", "1\n2\n3\n"},
		{"x = 7\nif x > 10 { print big } else if x > 5 { print medium } else { print small }\n", "medium\n"},
		{"fn add(a, b) { return a + b }\ntotal = add(1, 2)\nprint $total\n", "3\n"},
		{"print {1..9..2}\n", "1 3 5 7 9\n"},
		{"print repeat_str(\"ab\", 2)\n", "abab\n"},
	}
	for _, c := range cases {
		_, out := run(t, c.src)
		if out != c.want {
			t.Errorf("%q: output %q, want %q", c.src, out, c.want)
		}
	}
}
