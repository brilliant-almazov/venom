package tempo

import "strings"

// tablePoints are the tables of an SQL text of a span, read by SELECT, written by any other statement; a name right
// before «(» is a function, not a table.
func tablePoints(s renderSpan, query string) []Point {
	op := "written"
	if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(query)), "SELECT") {
		op = "read"
	}
	var points []Point
	for _, m := range queryTable.FindAllStringSubmatchIndex(query, -1) {
		if strings.HasPrefix(query[m[3]:], "(") {
			continue
		}
		name := strings.ToLower(strings.Trim(query[m[2]:m[3]], `"`))
		points = append(points, Point{Service: s.service, Dir: PointOut, Name: "table " + name + " " + op})
	}
	return points
}

// hostOf is the host an http client span calls: server.address, else the host of url.full.
func hostOf(s renderSpan) string {
	if h, ok := s.attr("server.address"); ok {
		return h
	}
	u, _ := s.attr("url.full")
	if i := strings.Index(u, "://"); i >= 0 {
		u = u[i+len("://"):]
	}
	return strings.SplitN(u, "/", 2)[0]
}
