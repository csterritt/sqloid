package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/chris/sqloid/internal/history"
)

func TestDbgEvict5(t *testing.T) {
	execs := &countingExecutors{}
	m := browseModel(t, execs, nil)
	full := history.NewResultStore()
	var fullIDs []history.EntryID
	for i := 0; i < 20; i++ {
		retained, _ := full.AppendFinalized(browseEntry(uint64(300+i), 1000+i, 6))
		fullIDs = append(fullIDs, retained.ID)
	}
	m.ResultHistory = full
	m.enterResultHistoryMode()
	m.resultHistoryCursorID = fullIDs[0]
	m.projectSelectedHistoryEntry()
	v := m.View()
	lines := strings.Split(v, "\n")
	fmt.Printf("%q\n", lines[3])
	trimmed := strings.Trim(lines[3], "\u2502 ")
	fmt.Printf("trimmed: %q\n", trimmed)
	fields := strings.FieldsFunc(trimmed, func(r rune) bool { return r == '|' || r == ' ' })
	fmt.Printf("fields: %q\n", fields)
}
