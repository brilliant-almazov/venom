package sql

import (
	"context"
	stdsql "database/sql"
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"

	// MySQL drivers
	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"

	// Postgres driver
	_ "github.com/lib/pq"

	// Oracle
	_ "github.com/sijms/go-ora"

	// Sqlite
	_ "modernc.org/sqlite"

	"github.com/ovh/venom"
)

// Name of the executor.
const Name = "sql"

// New returns a new executor that can execute SQL queries
func New() venom.Executor {
	return &Executor{}
}

// Executor is a venom executor can execute SQL queries
type Executor struct {
	File     string   `json:"file,omitempty" yaml:"file,omitempty"`
	Commands []string `json:"commands,omitempty" yaml:"commands,omitempty"`
	Driver   string   `json:"driver" yaml:"driver"`
	DSN      string   `json:"dsn" yaml:"dsn"`
	// ReadOnly runs every command in one read-only transaction.
	ReadOnly bool `json:"read_only,omitempty" yaml:"read_only,omitempty" mapstructure:"read_only"`
}

// Rows represents an array of Row
type Rows []Row

// Row represents a row return by a SQL query.
type Row map[string]interface{}

// QueryResult represents a rows return by a SQL query execution.
type QueryResult struct {
	Rows Rows `json:"rows,omitempty" yaml:"rows,omitempty"`
	// Out is the rows as psql -At -F '|' prints them: columns in select order joined by "|", rows by newline,
	// NULL empty, booleans t/f.
	Out string `json:"out" yaml:"out"`
}

// Result represents a step result.
type Result struct {
	Queries []QueryResult `json:"queries,omitempty" yaml:"queries,omitempty"`
	// Out is the Out of the last query.
	Out string `json:"out" yaml:"out"`
}

// Run implements the venom.Executor interface for Executor.
func (e Executor) Run(ctx context.Context, step venom.TestStep) (interface{}, error) {
	// Transform step to Executor instance.
	if err := mapstructure.WeakDecode(step, &e); err != nil {
		return nil, err
	}
	// Connect to the database and ping it.
	venom.Debug(ctx, "connecting to database %s, %s\n", e.Driver, e.DSN)
	db, err := sqlx.Connect(e.Driver, e.DSN)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to connect to database")
	}
	defer db.Close()

	var q queryer = db
	if e.ReadOnly {
		tx, err := db.BeginTxx(ctx, &stdsql.TxOptions{ReadOnly: true})
		if err != nil {
			return nil, errors.Wrapf(err, "failed to begin a read-only transaction")
		}
		defer tx.Rollback() //nolint:errcheck
		q = tx
	}

	results := []QueryResult{}
	// Execute commands on database
	// if the argument is specified.
	if len(e.Commands) != 0 {
		for i, s := range e.Commands {
			venom.Debug(ctx, "Executing command number %d\n", i)
			rows, err := q.Queryx(s)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to exec command number %d", i)
			}
			r, out, err := handleRows(rows)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to parse SQL rows for command number %d", i)
			}
			results = append(results, QueryResult{Rows: r, Out: out})
		}
	} else if e.File != "" {
		workdir := venom.StringVarFromCtx(ctx, "venom.testsuite.workdir")
		file := path.Join(workdir, e.File)
		venom.Debug(ctx, "loading SQL file from %s\n", file)
		sbytes, errs := os.ReadFile(file)
		if errs != nil {
			return nil, errs
		}
		rows, err := q.Queryx(string(sbytes))
		if err != nil {
			return nil, errors.Wrapf(err, "failed to exec SQL file %q", file)
		}
		r, out, err := handleRows(rows)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse SQL rows for SQL file %q", file)
		}
		results = append(results, QueryResult{Rows: r, Out: out})
	}
	r := Result{Queries: results}
	if len(results) > 0 {
		r.Out = results[len(results)-1].Out
	}
	return r, nil
}

// ZeroValueResult return an empty implementation of this executor result
func (Executor) ZeroValueResult() interface{} {
	return Result{}
}

// GetDefaultAssertions return the default assertions of the executor.
func (e Executor) GetDefaultAssertions() venom.StepAssertions {
	return venom.StepAssertions{Assertions: []venom.Assertion{}}
}

type queryer interface {
	Queryx(query string, args ...interface{}) (*sqlx.Rows, error)
}

// handleRows iter on each SQL rows result sets and serialize it into a []Row and the psql-like text of the rows.
func handleRows(rows *sqlx.Rows) ([]Row, string, error) {
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, "", err
	}
	res := []Row{}
	lines := []string{}
	for rows.Next() {
		vals, err := rows.SliceScan()
		if err != nil {
			return nil, "", err
		}
		row := make(Row, len(cols))
		cells := make([]string, len(cols))
		for i, c := range cols {
			if b, ok := vals[i].([]byte); ok {
				vals[i] = string(b)
			}
			row[c] = vals[i]
			cells[i] = cell(vals[i])
		}
		res = append(res, row)
		lines = append(lines, strings.Join(cells, "|"))
	}
	if err := rows.Err(); err != nil {
		return res, strings.Join(lines, "\n"), err
	}
	return res, strings.Join(lines, "\n"), nil
}

// cell prints one value as psql -At does.
func cell(v interface{}) string {
	switch x := v.(type) {
	case nil:
		return ""
	case bool:
		if x {
			return "t"
		}
		return "f"
	case time.Time:
		return x.Format("2006-01-02 15:04:05.999999")
	case string:
		return x
	default:
		return fmt.Sprint(x)
	}
}
