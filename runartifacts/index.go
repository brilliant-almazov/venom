package runartifacts

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// indexHeader is the table of protocols/INDEX.md, one row per testcase.
const indexHeader = "| suite | testcase | verdict | trace | link |\n| -- | -- | -- | -- | -- |\n"

// noTree is the trace cell of a call whose trace id has no span tree in Tempo.
const noTree = "trace not saved"

// protocolLink is the text of a link to a protocol.
const protocolLink = "protocol"

// sortRows puts the testcases in testsuite file order, then in their order in the testsuite.
func sortRows(rows []row) {
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].suiteFile != rows[j].suiteFile {
			return rows[i].suiteFile < rows[j].suiteFile
		}

		return rows[i].index < rows[j].index
	})
}

func (w *Writer) writeIndex() error {
	base := filepath.Join(w.dir, ProtocolsDir)
	var b strings.Builder
	b.WriteString("# Protocols\n\n" + indexHeader)

	for _, r := range w.rows {
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s |\n", cell(r.suite), cell(r.testcase), verdict(r.view.Mark, r.view.Code),
			traceCell(base, r), link(protocolLink, base, r.protocol))
	}

	return writeFile(filepath.Join(base, IndexFile), []byte(b.String()))
}

// verdict is the mark with the answer code, when the call had one.
func verdict(mark, code string) string {
	if code == "" {
		return mark
	}

	return mark + " " + code
}

// traceCell is the link to the span tree of the call, «trace not saved <trace_id>» when Tempo has none, «—» for
// a call with no trace id (the monolith).
func traceCell(from string, r row) string {
	switch {
	case r.trace != "":
		return link(short(r.view.TraceID), from, r.trace)

	case r.view.TraceID != "":
		return noTree + " `" + r.view.TraceID + "`"

	default:
		return "—"
	}
}

// short is the first 8 characters of a trace id.
func short(id string) string {
	if len(id) > 8 {
		return id[:8]
	}

	return id
}

// link is a markdown link to file relative to the folder from.
func link(text, from, file string) string {
	rel, err := filepath.Rel(from, file)
	if err != nil {
		rel = file
	}

	return "[" + text + "](" + strings.ReplaceAll(filepath.ToSlash(rel), " ", "%20") + ")"
}

func cell(s string) string {
	return strings.ReplaceAll(s, "|", "\\|")
}
