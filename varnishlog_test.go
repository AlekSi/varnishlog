package varnishlog

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func parse(t *testing.T, lines <-chan string, expected Transaction) Transaction {
	actual, err := ParseTransaction(lines)
	require.Nil(t, err)
	expected.Lines = actual.Lines
	assert.Equal(t, expected, actual)
	return actual
}

func assertHasLine(t *testing.T, tr Transaction, expected Line) {
	for _, l := range tr.Lines {
		if assert.ObjectsAreEqual(expected, l) {
			return
		}
	}
	assert.Fail(t, "line not found", expected.String())
}

func testParseTransaction(t *testing.T, filename string) {
	lines := make(chan string)
	f, err := os.Open(filepath.Join("testdata", filename))
	require.Nil(t, err)
	defer f.Close()

	go func() {
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			lines <- scanner.Text()
		}

		err = scanner.Err()
		require.Nil(t, err)
		close(lines)
	}()

	tr := parse(t, lines, Transaction{VXID: 32771, Begin: Reference{Type: "bereq", VXID: 32770, Reason: "fetch"}})
	assertHasLine(t, tr, Line{"BerespStatus", "200"})

	tr = parse(t, lines, Transaction{VXID: 32770, Begin: Reference{Type: "req", VXID: 32769, Reason: "rxreq"}})
	assertHasLine(t, tr, Line{"Link", "bereq 32771 fetch"})

	parse(t, lines, Transaction{VXID: 32773, Begin: Reference{Type: "bereq", VXID: 32772, Reason: "fetch"}})
	parse(t, lines, Transaction{VXID: 32772, Begin: Reference{Type: "req", VXID: 32769, Reason: "rxreq"}})
	parse(t, lines, Transaction{VXID: 32775, Begin: Reference{Type: "bereq", VXID: 32774, Reason: "fetch"}})
	parse(t, lines, Transaction{VXID: 32774, Begin: Reference{Type: "req", VXID: 32769, Reason: "rxreq"}})

	parse(t, lines, Transaction{VXID: 32769, Begin: Reference{Type: "sess", VXID: 0, Reason: "HTTP/1"}})

	tr, err = ParseTransaction(lines)
	assert.Equal(t, io.EOF, err)
	assert.Equal(t, Transaction{}, tr)
}

func TestParseTransaction41(t *testing.T) {
	testParseTransaction(t, "4.1.0.log")
}

func TestParseTransaction41VXID(t *testing.T) {
	testParseTransaction(t, "4.1.0-vxid.log")
}
