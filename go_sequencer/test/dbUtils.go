package test

import (
	dbUtils "tokamak-sybil-resistance/database"

	"github.com/jmoiron/sqlx"
)

// WipeDB redo all the migrations of the SQL DB (HistoryDB and L2DB),
// efectively recreating the original state
func WipeDB(db *sqlx.DB) {
	if err := dbUtils.MigrationsDown(db.DB, 0); err != nil {
		panic(err)
	}
	if err := dbUtils.MigrationsUp(db.DB); err != nil {
		panic(err)
	}
}
