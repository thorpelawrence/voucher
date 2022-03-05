package database

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/rs/xid"
	_ "modernc.org/sqlite"
)

type DB struct {
	d *sql.DB
}

type Voucher struct {
	Code     xid.ID
	Redeemed bool
}

func New(filename string) (*DB, error) {
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s", filename))
	if err != nil {
		return nil, err
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS vouchers (
			code text PRIMARY KEY,
			redeemed bool DEFAULT false NOT NULL
		);
	`)
	if err != nil {
		return nil, err
	}

	return &DB{db}, nil
}

func (db *DB) GetVouchers() ([]Voucher, error) {
	rows, err := db.d.Query("SELECT code, redeemed FROM vouchers ORDER BY code DESC")
	if err != nil {
		return nil, err
	}

	var vouchers []Voucher

	for rows.Next() {
		var v Voucher
		err = rows.Scan(&v.Code, &v.Redeemed)
		if err != nil {
			return nil, err
		}
		vouchers = append(vouchers, v)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return vouchers, nil
}

func (db *DB) CreateVoucher(code string) error {
	_, err := db.d.Exec("INSERT INTO vouchers (code) VALUES (?)", code)
	return err
}

type RedeemStatus int

const (
	RedeemStatusSuccess = iota
	RedeemStatusRedeemed
	RedeemStatusNotExists
	RedeemStatusError
)

func (db *DB) RedeemVoucher(code string) (status RedeemStatus, err error) {
	row := db.d.QueryRow("SELECT redeemed FROM vouchers WHERE code = ?", code)
	if err := row.Err(); err != nil {
		return RedeemStatusError, err
	}

	var redeemed bool
	if err := row.Scan(&redeemed); err != nil {
		if err == sql.ErrNoRows {
			return RedeemStatusNotExists, nil
		}
		return RedeemStatusError, err
	}

	if redeemed {
		return RedeemStatusRedeemed, nil
	}

	_, err = db.d.Exec("UPDATE vouchers SET redeemed = true WHERE code = ?", code)
	if err != nil {
		return RedeemStatusError, err
	}

	return RedeemStatusSuccess, nil
}

func (db *DB) DeleteVouchers(codes ...string) error {
	placeholders := make([]string, 0, len(codes))
	args := make([]interface{}, 0, len(codes))

	for _, code := range codes {
		placeholders = append(placeholders, "?")
		args = append(args, code)
	}

	_, err := db.d.Exec(
		fmt.Sprintf(
			"DELETE FROM vouchers WHERE code IN (%s)",
			strings.Join(placeholders, ","),
		),
		args...,
	)
	return err
}
