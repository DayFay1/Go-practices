package main

import "fmt"

func main() {
	fmt.Println("Migrations live in internal/db/migrations.")
	fmt.Println("Use golang-migrate CLI, e.g.:")
	fmt.Println(`migrate -path go-practice3/internal/db/migrations -database "sqlite://./go-practice3/expense.db" up`)
}
