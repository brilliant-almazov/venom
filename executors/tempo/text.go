package tempo

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

const sqlWidth = 150

var sqlClause = regexp.MustCompile(`\s+(FROM|WHERE|ORDER BY|GROUP BY|LIMIT|AND|OR|VALUES|JOIN|SET)\b`)

func sqlLines(q string) []string {
	q = strings.Join(strings.Fields(q), " ")
	var lines []string
	for _, clause := range strings.Split(sqlClause.ReplaceAllString(q, "\n$1"), "\n") {
		line := ""
		for _, w := range strings.Fields(clause) {
			if line != "" && len([]rune(line))+1+len([]rune(w)) > sqlWidth {
				lines = append(lines, line)
				line = ""
			}
			if line != "" {
				line += " "
			}
			line += w
		}
		lines = append(lines, line)
	}
	return lines
}

func (v renderValue) pyText() string {
	switch {
	case v.BoolValue != nil:
		if *v.BoolValue {
			return "True"
		}
		return "False"
	case v.ArrayValue != nil:
		items := make([]string, 0, len(v.ArrayValue.Values))
		for _, i := range v.ArrayValue.Values {
			items = append(items, i.pyText())
		}
		return "[" + strings.Join(items, ", ") + "]"
	}
	return v.text()
}

func pyFloat(f float64) string {
	if a := math.Abs(f); a != 0 && (a < 1e-4 || a >= 1e16) {
		return strconv.FormatFloat(f, 'e', -1, 64)
	}
	s := strconv.FormatFloat(f, 'f', -1, 64)
	if !strings.Contains(s, ".") {
		s += ".0"
	}
	return s
}

func ms(start, end int64) string {
	f, _ := strconv.ParseFloat(strconv.FormatFloat(float64(end-start)/1e6, 'f', 2, 64), 64)
	return pyFloat(f)
}
