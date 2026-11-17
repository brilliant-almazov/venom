package runartifacts

import (
	"fmt"
	"strings"
)

// graph is every node the steps of the run touch and every edge between them, in call order.
func (l layout) graph() ([]*node, []*edge) {
	var nodes []*node
	var edges []*edge
	nodeAt, edgeAt := map[string]*node{}, map[string]*edge{}

	for _, r := range l.rows {
		prev := runner

		for _, s := range r.view.Steps {
			name, places := placeOf(s)
			n, ok := nodeAt[name]

			if !ok {
				n = &node{name: name, kind: orNone(s.Type), at: map[string]*node{}, cases: map[string]bool{}}
				nodeAt[name] = n
				nodes = append(nodes, n)
			}

			n.c.add(s.Mark)
			n.cases[r.testcase] = true

			for _, p := range places {
				sub, ok := n.at[p]

				if !ok {
					sub = &node{name: orNone(p)}
					n.at[p] = sub
					n.subs = append(n.subs, sub)
				}

				sub.c.add(s.Mark)
			}

			key := prev + "\x00" + name
			e, ok := edgeAt[key]

			if !ok {
				e = &edge{from: prev, to: name}
				edgeAt[key] = e
				edges = append(edges, e)
			}

			e.c.add(s.Mark)
			prev = name
		}
	}

	return nodes, edges
}

// zoomSystemText is 01-system.md: every node the scenarios touch with its bar, its paths or tables under it, red
// marked, then the nodes and the edges in call order.
func (l layout) zoomSystemText(hist []map[string]count) string {
	base := l.zoomDir()
	nodes, edges := l.graph()
	var b strings.Builder
	b.WriteString("# System of the run\n\n")
	fmt.Fprintf(&b, "- testcases: %s\n- zoom: %s · %s\n- scheme: %s\n\n", countOf(l.rows), link(ZoomRunFile, base, l.zoomRun()),
		link(IndexFile, base, l.zoomIndex()), link(SystemFile, base, l.systemScheme()))

	lines := []string{runner}

	for i, n := range nodes {
		branch, indent := branchMid, indentMid

		if i == len(nodes)-1 {
			branch, indent = branchEnd, indentEnd
		}

		lines = append(lines, strings.TrimSuffix(branch, " ")+"► "+n.name+"  "+bar(n.c)+red(n.c))

		for j, sub := range n.subs {
			leaf := branchMid

			if j == len(n.subs)-1 {
				leaf = branchEnd
			}

			lines = append(lines, indent+leaf+sub.name+"  "+bar(sub.c)+red(sub.c))
		}
	}

	fence(&b, lines)
	drawProcesses(&b, headProcesses, l.rows, hist)
	b.WriteString("## Nodes\n\n| node | executor | steps | verdict | bar | testcases |\n| -- | -- | -- | -- | -- | -- |\n")

	for _, n := range nodes {
		fmt.Fprintf(&b, "| %s | %s | %d | %s | %s | %d |\n", cell(n.name), cell(n.kind), n.c.all, n.c.mark(), progress(n.c),
			len(n.cases))
	}

	b.WriteString("\n## Edges in call order\n\n| # | from | to | steps | verdict | bar |\n| -- | -- | -- | -- | -- | -- |\n")

	for i, e := range edges {
		fmt.Fprintf(&b, "| %d | %s | %s | %d | %s | %s |\n", i+1, cell(e.from), cell(e.to), e.c.all, e.c.mark(), progress(e.c))
	}

	return b.String()
}

// red marks a node with a failed step.
func red(c count) string {
	if c.failed > 0 {
		return "  ◄ red"
	}

	return ""
}
