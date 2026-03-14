package utils

import (
	"context"
	"database/sql"

	"github.com/uptrace/bun"
)

// Scan 遍历查询结果并应用回调函数
func Scan[T any](db *bun.DB, rows *sql.Rows, fn func(dst *T)) (err error) {
	ctx := context.Background()
	var t T
	for rows.Next() {
		err = db.ScanRow(ctx, rows, &t)
		if err != nil {
			return
		}
		fn(&t)
	}
	err = rows.Err()
	return
}
