package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"magu/internal/auth"
)

func main() {
	username := flag.String("username", "", "username untuk login")
	password := flag.String("password", "", "password")
	name := flag.String("name", "", "nama lengkap")
	role := flag.String("role", "cashier", "admin atau cashier")
	flag.Parse()

	if *username == "" || *password == "" || *name == "" {
		flag.Usage()
		os.Exit(2)
	}

	ctx := context.Background()
	db, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err == nil {
		defer db.Close()
		err = auth.SaveUser(ctx, db, *username, *password, *name, *role)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("user %q (%s) tersimpan\n", *username, *role)
}
