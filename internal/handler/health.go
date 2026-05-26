package handler

import (
	"database/sql"
	"net/http"
)

func HealthCheck(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := db.Ping(); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("DB Error"))
			return
		}
		w.Write([]byte("OK"))
	}
}
