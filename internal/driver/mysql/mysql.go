package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/yourname/mcp-x/internal/driver"
)

type MySQLDriver struct {
	db *sql.DB
}

func (d *MySQLDriver) Name() string            { return "mysql" }
func (d *MySQLDriver) Type() driver.DriverType { return driver.DriverTypeSQL }

func (d *MySQLDriver) Connect(ctx context.Context, cfg driver.ConnConfig) error {
	db, err := sql.Open("mysql", cfg.DSN)
	if err != nil {
		return fmt.Errorf("mysql open: %w", err)
	}
	if cfg.MaxOpenConns > 0 {
		db.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns > 0 {
		db.SetMaxIdleConns(cfg.MaxIdleConns)
	}
	if cfg.ConnMaxLifetime > 0 {
		db.SetConnMaxLifetime(time.Duration(cfg.ConnMaxLifetime))
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return fmt.Errorf("mysql ping: %w", err)
	}
	d.db = db
	return nil
}

func (d *MySQLDriver) Query(ctx context.Context, sqlStr string, args []any) (*driver.QueryResult, error) {
	rows, err := d.db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("columns: %w", err)
	}

	var resultRows [][]any
	for rows.Next() {
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		for i, v := range values {
			if b, ok := v.([]byte); ok {
				values[i] = string(b)
			}
			if v == nil {
				values[i] = "NULL"
			}
		}
		resultRows = append(resultRows, values)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iter: %w", err)
	}

	return &driver.QueryResult{
		Columns: cols,
		Rows:    resultRows,
	}, nil
}

func (d *MySQLDriver) Execute(ctx context.Context, sqlStr string, args []any) (*driver.ExecResult, error) {
	start := time.Now()
	res, err := d.db.ExecContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("exec: %w", err)
	}
	affected, _ := res.RowsAffected()
	lastID, _ := res.LastInsertId()
	return &driver.ExecResult{
		AffectedRows: affected,
		LastInsertID: lastID,
		Duration:     time.Since(start).Milliseconds(),
	}, nil
}

func (d *MySQLDriver) ListTables(ctx context.Context) ([]driver.TableInfo, error) {
	rows, err := d.db.QueryContext(ctx, "SHOW TABLES")
	if err != nil {
		return nil, fmt.Errorf("show tables: %w", err)
	}
	defer rows.Close()

	var tables []driver.TableInfo
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan table: %w", err)
		}
		tables = append(tables, driver.TableInfo{Name: name})
	}
	return tables, rows.Err()
}

func (d *MySQLDriver) DescribeTable(ctx context.Context, table string) (*driver.TableSchema, error) {
	rows, err := d.db.QueryContext(ctx, "DESCRIBE "+quoteIdent(table))
	if err != nil {
		return nil, fmt.Errorf("describe: %w", err)
	}
	defer rows.Close()

	schema := &driver.TableSchema{Table: table}
	for rows.Next() {
	var field, typ, nullness, keyStr, extra string
	var defStr sql.NullString
	if err := rows.Scan(&field, &typ, &nullness, &keyStr, &defStr, &extra); err != nil {
		return nil, fmt.Errorf("describe scan: %w", err)
	}
	schema.Columns = append(schema.Columns, driver.ColumnInfo{
		Name:     field,
		Type:     typ,
		Nullable: nullness,
		Key:      keyStr,
		Default:  defStr.String,
	})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	idxRows, err := d.db.QueryContext(ctx, fmt.Sprintf("SHOW INDEX FROM %s", quoteIdent(table)))
	if err == nil {
		defer idxRows.Close()
		idxMap := make(map[string]*driver.IndexInfo)
		var idxOrder []string
		for idxRows.Next() {
			var tableNonEq, nonUniqueStr, keyName string
			var seqInIndex int
			var colName sql.NullString
			if err := idxRows.Scan(&tableNonEq, &nonUniqueStr, &keyName, &seqInIndex, &colName, nil, nil, nil, nil, nil, nil, nil, nil); err != nil {
				break
			}
			idx, ok := idxMap[keyName]
			if !ok {
				idx = &driver.IndexInfo{
					Name:   keyName,
					Unique: nonUniqueStr == "0",
				}
				idxMap[keyName] = idx
				idxOrder = append(idxOrder, keyName)
			}
			if colName.Valid {
				idx.Columns = append(idx.Columns, colName.String)
			}
		}
		for _, name := range idxOrder {
			schema.Indexes = append(schema.Indexes, *idxMap[name])
		}
	}

	return schema, nil
}

func (d *MySQLDriver) Ping(ctx context.Context) error {
	return d.db.PingContext(ctx)
}

func (d *MySQLDriver) Close() error {
	if d.db == nil {
		return nil
	}
	return d.db.Close()
}

func quoteIdent(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}

func init() {
	driver.Register("mysql", func() driver.AnyDriver { return &MySQLDriver{} })
}
