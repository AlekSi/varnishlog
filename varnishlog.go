package varnishlog

import (
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
)

var lineRE = regexp.MustCompile(`^\-+\s+(\w+)\s*(.*)$`)

// Line is a pair of tag and value.
type Line struct {
	Tag   string
	Value string
}

func (l Line) String() string {
	return l.Tag + " " + l.Value
}

// ParseLine parses varnishlog line.
func ParseLine(s string) (l Line, err error) {
	p := lineRE.FindStringSubmatch(s)
	if p == nil {
		err = fmt.Errorf("ParseLine: failed to parse %q", s)
		return
	}
	l.Tag = p[1]
	l.Value = p[2]
	return
}

// Reference contains information about parent or child transaction (Begin and Links tags).
type Reference struct {
	Type   string
	VXID   uint
	Reason string
}

// ParseReference parses Reference from given string value.
func ParseReference(s string) (ref Reference, err error) {
	p := strings.Split(s, " ")
	if len(p) != 3 {
		err = fmt.Errorf("%q: expected 3 parts, got %d", s, len(p))
		return
	}

	vxid, err := strconv.ParseUint(p[1], 10, 64)
	if err != nil {
		return
	}

	ref.Type = p[0]
	ref.VXID = uint(vxid)
	ref.Reason = p[2]
	return
}

// Transaction is a single unit of Varnish work.
type Transaction struct {
	VXID  uint
	Begin Reference
	Lines []Line
}

// ParseTransaction reads lines for given channel and returns next transaction, or error.
// It returns io.EOF when channel is closed.
func ParseTransaction(lines <-chan string) (t Transaction, err error) {
	for line := range lines {
		line = strings.TrimSpace(line)

		switch {
		case line == "":
			continue

		case strings.HasPrefix(line, "*"):
			// transaction start
			p := strings.Split(line, " ")
			var vxid uint64
			vxid, err = strconv.ParseUint(p[len(p)-1], 10, 64)
			if err != nil {
				return
			}
			t.VXID = uint(vxid)

		default:
			// transaction line
			var l Line
			l, err = ParseLine(line)
			if err != nil {
				return
			}
			t.Lines = append(t.Lines, l)

			switch l.Tag {
			case "Begin":
				var ref Reference
				ref, err = ParseReference(l.Value)
				if err != nil {
					return
				}
				t.Begin = ref

			case "End":
				return
			}
		}
	}

	err = io.EOF
	return
}
